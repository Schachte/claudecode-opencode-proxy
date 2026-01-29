package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/schachte/claude-opencode-proxy/config"
)

func Run(port int, bindAddr string, verbose bool, quiet bool) {
	cfg := config.LoadConfig()
	var lastModel string
	var requestCount int

	client, err := config.CreateHTTPClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create HTTP client: %v", err)
	}

	timestamp := func() string {
		return time.Now().Format("15:04:05")
	}

	logInfo := func(format string, args ...interface{}) {
		if !quiet {
			log.Printf("[%s] %s", timestamp(), fmt.Sprintf(format, args...))
		}
	}

	logDebug := func(format string, args ...interface{}) {
		if verbose && !quiet {
			log.Printf("[%s] [DEBUG] %s", timestamp(), fmt.Sprintf(format, args...))
		}
	}

	handleProxy := func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		requestCount++
		reqID := requestCount

		logDebug("REQ #%d %s %s", reqID, r.Method, r.URL.Path)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var reqData map[string]interface{}
		isStreaming := false
		model := ""
		if len(body) > 0 {
			if err := json.Unmarshal(body, &reqData); err == nil {
				delete(reqData, "context_management")
				delete(reqData, "mcp_servers")
				if stream, ok := reqData["stream"].(bool); ok {
					isStreaming = stream
				}
				if m, ok := reqData["model"].(string); ok {
					model = m
				}
				body, _ = json.Marshal(reqData)
			}
		}

		if model != "" && model != lastModel {
			logInfo("MODEL  %s", model)
			lastModel = model
		}

		streamType := "sync"
		if isStreaming {
			streamType = "stream"
		}
		logInfo("START  #%d [%s]", reqID, streamType)

		logDebug("REQ #%d model=%s stream=%v", reqID, model, isStreaming)

		token, authType, err := config.GetToken(cfg)
		if err != nil {
			logInfo("ERROR  #%d auth failed: %v", reqID, err)
			http.Error(w, "Failed to get auth token", http.StatusInternalServerError)
			return
		}

		upstreamURL := cfg.Target + r.URL.Path
		logDebug("PROXY  #%d -> %s", reqID, upstreamURL)

		upstreamReq, err := http.NewRequest(r.Method, upstreamURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "Failed to create upstream request", http.StatusInternalServerError)
			return
		}

		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("anthropic-version", "2023-06-01")

		if cfg.CfAccess {
			upstreamReq.Header.Set("cf-access-token", token)
			if cfg.CfClientID != "" && cfg.CfClientSecret != "" {
				upstreamReq.Header.Set("CF-Access-Client-Id", cfg.CfClientID)
				upstreamReq.Header.Set("CF-Access-Client-Secret", cfg.CfClientSecret)
			}
		} else if authType == "apikey" {
			upstreamReq.Header.Set("x-api-key", token)
		} else {
			upstreamReq.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(upstreamReq)
		if err != nil {
			logInfo("ERROR  #%d upstream failed: %v", reqID, err)
			http.Error(w, fmt.Sprintf("Upstream request failed: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		logDebug("RES    #%d status=%d", reqID, resp.StatusCode)

		if isStreaming && resp.StatusCode == http.StatusOK {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				logInfo("ERROR  #%d flusher not supported", reqID)
				return
			}

			reader := bufio.NewReader(resp.Body)
			totalBytes := 0

			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err != io.EOF {
						logDebug("STREAM #%d read error: %v", reqID, err)
					}
					break
				}
				totalBytes += len(line)
				if _, writeErr := w.Write(line); writeErr != nil {
					logDebug("STREAM #%d write error: %v", reqID, writeErr)
					break
				}
				flusher.Flush()
			}
			logInfo("DONE   #%d [%s] %dB %v", reqID, streamType, totalBytes, time.Since(startTime).Round(time.Millisecond))
		} else {
			for key, values := range resp.Header {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(resp.StatusCode)
			written, _ := io.Copy(w, resp.Body)
			logInfo("DONE   #%d [%s] %dB %v", reqID, streamType, written, time.Since(startTime).Round(time.Millisecond))
		}
	}

	handleHealth := func(w http.ResponseWriter, r *http.Request) {
		token, _, err := config.GetToken(cfg)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"target":    cfg.Target,
			"auth_type": cfg.AuthType,
			"cf_access": cfg.CfAccess,
			"has_token": err == nil && token != "",
		})
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/", handleProxy)

	addr := fmt.Sprintf("%s:%d", bindAddr, port)
	fmt.Printf("Proxy: http://%s -> %s\n", addr, cfg.Target)
	fmt.Printf("Auth: %s, CF-Access: %v\n", cfg.AuthType, cfg.CfAccess)
	if verbose {
		fmt.Println("Verbose: on")
	}
	fmt.Println()

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
