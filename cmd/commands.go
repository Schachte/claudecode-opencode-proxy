package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/schachte/claudecode-opencode-proxy/claude"
	"github.com/schachte/claudecode-opencode-proxy/config"
	"github.com/schachte/claudecode-opencode-proxy/proxy"
)

var lastAuthChoiceFile = filepath.Join(config.ConfigDir, "last-auth-choice")

func Config(args []string) {
	cfg := config.LoadConfig()

	if len(args) == 0 {
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 < len(args) {
				cfg.Target = args[i+1]
				i++
			}
		case "--auth-type":
			if i+1 < len(args) {
				cfg.AuthType = args[i+1]
				i++
			}
		case "--api-key":
			if i+1 < len(args) {
				cfg.APIKey = args[i+1]
				i++
			}
		case "--login-url":
			if i+1 < len(args) {
				cfg.LoginURL = args[i+1]
				i++
			}
		case "--cf-access":
			cfg.CfAccess = true
		case "--no-cf-access":
			cfg.CfAccess = false
		case "--cf-client-id":
			if i+1 < len(args) {
				cfg.CfClientID = args[i+1]
				i++
			}
		case "--cf-client-secret":
			if i+1 < len(args) {
				cfg.CfClientSecret = args[i+1]
				i++
			}
		case "--proxy":
			if i+1 < len(args) {
				cfg.Proxy = args[i+1]
				i++
			}
		case "--ca-cert":
			if i+1 < len(args) {
				cfg.CACert = args[i+1]
				i++
			}
		case "--insecure-skip-verify":
			cfg.InsecureSkip = true
		case "--no-insecure-skip-verify":
			cfg.InsecureSkip = false
		case "--reset":
			cfg = config.DefaultConfig()
		}
	}

	if err := config.SaveConfig(cfg); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}
	fmt.Println("Config saved:")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
}

func Enable(port int) {
	cfg := config.LoadConfig()

	cmd := exec.Command("claude", "/logout")
	cmd.Run()
	fmt.Println("Logged out of Claude native auth")

	if _, err := os.Stat(claude.ClaudeSettings); err == nil {
		if _, err := os.Stat(claude.BackupSettings); os.IsNotExist(err) {
			data, _ := os.ReadFile(claude.ClaudeSettings)
			os.WriteFile(claude.BackupSettings, data, 0644)
			fmt.Printf("Backed up: %s\n", claude.BackupSettings)
		}
	}

	settings, err := claude.LoadSettings()
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}

	settings["apiKeyHelper"] = config.APIKeyHelper(cfg)

	if err := claude.SaveSettings(settings); err != nil {
		log.Fatalf("Failed to save settings: %v", err)
	}
	fmt.Printf("Updated: %s\n", claude.ClaudeSettings)

	claude.WriteEnvFile(port)
	claude.UpdateShellRC(port, true)

	fmt.Println()
	fmt.Println("Enabled proxy mode")
	fmt.Println("Restart your shell or run:")
	fmt.Printf("  source %s\n", config.EnvFile)
}

func Disable() {
	if _, err := os.Stat(claude.BackupSettings); err == nil {
		data, err := os.ReadFile(claude.BackupSettings)
		if err != nil {
			log.Fatalf("Failed to read backup: %v", err)
		}
		if err := os.WriteFile(claude.ClaudeSettings, data, 0644); err != nil {
			log.Fatalf("Failed to restore settings: %v", err)
		}
		fmt.Printf("Restored: %s\n", claude.ClaudeSettings)
	} else {
		settings, err := claude.LoadSettings()
		if err != nil {
			log.Fatalf("Failed to load settings: %v", err)
		}
		delete(settings, "apiKeyHelper")
		if err := claude.SaveSettings(settings); err != nil {
			log.Fatalf("Failed to save settings: %v", err)
		}
		fmt.Printf("Updated: %s\n", claude.ClaudeSettings)
	}

	os.WriteFile(config.EnvFile, []byte("unset ANTHROPIC_BASE_URL\n"), 0644)
	fmt.Printf("Updated: %s\n", config.EnvFile)

	claude.UpdateShellRC(0, false)

	fmt.Println()
	fmt.Println("Disabled proxy mode")
	fmt.Println("Restart your shell or run:")
	fmt.Printf("  source %s\n", config.EnvFile)
	fmt.Println()
	fmt.Println("To restore Claude native auth:")
	fmt.Println("  claude /login")
}

func ProxyBackground(port int, bindAddr string, verbose bool, quiet bool) {
	cfg := config.LoadConfig()

	// Warn if using default placeholder URL
	if strings.Contains(cfg.Target, "opencode.custom.dev") {
		fmt.Println()
		fmt.Println("\033[33m⚠ Warning: Using default placeholder URL\033[0m")
		fmt.Println()
		fmt.Println("Configure your provider first:")
		fmt.Println()
		fmt.Println("  Anthropic API:")
		fmt.Println("    ./claude-opencode-proxy config --target https://api.anthropic.com/v1 --api-key sk-ant-xxx")
		fmt.Println()
		fmt.Println("  OpenCode Server:")
		fmt.Println("    ./claude-opencode-proxy config --target https://YOUR-SERVER/anthropic --login-url https://YOUR-SERVER")
		fmt.Println("    ./claude-opencode-proxy login")
		fmt.Println()
		return
	}

	os.MkdirAll(config.ConfigDir, 0755)

	args := []string{"serve", "-p", strconv.Itoa(port), "-b", bindAddr, "-f"}
	if verbose {
		args = append(args, "-v")
	}
	if quiet {
		args = append(args, "-q")
	}

	logF, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}

	os.WriteFile(config.PidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	fmt.Printf("Proxy started (PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("Logs: %s\n", config.LogFile)
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  tail -f %s\n", config.LogFile)
	fmt.Println("  claude-opencode-proxy stop")
}

func ProxyStop() {
	data, err := os.ReadFile(config.PidFile)
	if err != nil {
		fmt.Println("Proxy not running (no PID file)")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("Invalid PID file")
		os.Remove(config.PidFile)
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("Process not found")
		os.Remove(config.PidFile)
		return
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Failed to stop process: %v\n", err)
		return
	}

	os.Remove(config.PidFile)
	fmt.Printf("Proxy stopped (PID: %d)\n", pid)
}

func ProxyLogs() {
	cmd := exec.Command("tail", "-f", config.LogFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func ProxyForeground(port int, bindAddr string, verbose bool, quiet bool) {
	proxy.Run(port, bindAddr, verbose, quiet)
}

func Login(args []string) {
	cfg := config.LoadConfig()

	target := cfg.LoginURL
	for i := 0; i < len(args); i++ {
		if args[i] == "--target" && i+1 < len(args) {
			target = args[i+1]
		}
	}

	// Warn if using default placeholder URL
	if strings.Contains(target, "opencode.custom.dev") {
		fmt.Println()
		fmt.Println("\033[33m⚠ Warning: Using default placeholder URL\033[0m")
		fmt.Println()
		fmt.Println("Configure your actual OpenCode server URL:")
		fmt.Println("  ./claude-opencode-proxy config --target https://YOUR-SERVER/anthropic --login-url https://YOUR-SERVER")
		fmt.Println()
		return
	}

	cmd := exec.Command("opencode", "auth", "login", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		log.Fatalf("Login failed: %v", err)
	}
}

func Status() {
	cfg := config.LoadConfig()

	fmt.Println("=== Proxy Config ===")
	fmt.Printf("Config: %s\n", config.ConfigFile)
	fmt.Printf("Target: %s\n", cfg.Target)
	fmt.Printf("Auth type: %s\n", cfg.AuthType)
	fmt.Printf("API key: %s\n", cfg.APIKey)
	fmt.Printf("Login URL: %s\n", cfg.LoginURL)
	fmt.Printf("CF-Access: %v\n", cfg.CfAccess)
	if cfg.Proxy != "" {
		fmt.Printf("Proxy: %s\n", cfg.Proxy)
	}
	if cfg.CACert != "" {
		fmt.Printf("CA Cert: %s\n", cfg.CACert)
	}
	if cfg.InsecureSkip {
		fmt.Printf("Insecure Skip Verify: %v\n", cfg.InsecureSkip)
	}

	fmt.Println()
	fmt.Println("=== Auth Status ===")
	if token, authType, err := config.GetToken(cfg); err != nil {
		fmt.Printf("Token: error (%v)\n", err)
	} else {
		fmt.Printf("Token: %d chars (%s)\n", len(token), authType)
	}

	fmt.Println()
	fmt.Println("=== Claude Settings ===")
	fmt.Printf("Settings: %s\n", claude.ClaudeSettings)

	settings, err := claude.LoadSettings()
	if err != nil {
		fmt.Printf("Status: error (%v)\n", err)
		return
	}

	if helper, ok := settings["apiKeyHelper"].(string); ok {
		fmt.Printf("apiKeyHelper: %s\n", helper)
	} else {
		fmt.Println("Mode: default (direct Anthropic)")
	}

	if _, err := os.Stat(claude.BackupSettings); err == nil {
		fmt.Printf("Backup: %s\n", claude.BackupSettings)
	}
}

func Env(port int) {
	fmt.Printf("export ANTHROPIC_BASE_URL=http://127.0.0.1:%d\n", port)
}

func isProxyRunning() bool {
	// Check if PID file exists
	data, err := os.ReadFile(config.PidFile)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func getProxyPort() int {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		return 8080 // default port
	}
	// Parse port from URL like http://127.0.0.1:8080
	parts := strings.Split(baseURL, ":")
	if len(parts) >= 3 {
		port, err := strconv.Atoi(parts[2])
		if err == nil {
			return port
		}
	}
	return 8080
}

func hasAuthConflict() bool {
	// Check if Claude Console credentials exist
	credentialsFile := filepath.Join(os.Getenv("HOME"), ".claude", ".credentials.json")
	hasConsoleAuth := false
	if data, err := os.ReadFile(credentialsFile); err == nil && len(data) > 2 {
		hasConsoleAuth = true
	}

	// Check if apiKeyHelper is configured
	settings, err := claude.LoadSettings()
	if err != nil {
		return false
	}
	_, hasApiKeyHelper := settings["apiKeyHelper"]

	return hasConsoleAuth && hasApiKeyHelper
}

func getLastAuthChoice() string {
	data, err := os.ReadFile(lastAuthChoiceFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveLastAuthChoice(choice string) {
	os.MkdirAll(config.ConfigDir, 0755)
	os.WriteFile(lastAuthChoiceFile, []byte(choice), 0644)
}

func promptAuthChoice() string {
	last := getLastAuthChoice()

	fmt.Println()
	fmt.Println("  \033[33m⚠ Auth conflict detected\033[0m")
	fmt.Println()
	fmt.Println("  Select auth mode:")
	fmt.Println()
	fmt.Println("    1) OpenCode proxy")
	fmt.Println("    2) Anthropic Console")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	if last != "" {
		fmt.Printf("  Choice [%s]: ", last)
	} else {
		fmt.Print("  Choice: ")
	}

	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(input)

	if choice == "" && last != "" {
		choice = last
	}

	switch choice {
	case "1", "opencode", "o":
		saveLastAuthChoice("1")
		return "opencode"
	case "2", "anthropic", "a":
		saveLastAuthChoice("2")
		return "anthropic"
	default:
		if choice == "" {
			saveLastAuthChoice("1")
			return "opencode"
		}
		fmt.Println("Invalid choice, using OpenCode")
		saveLastAuthChoice("1")
		return "opencode"
	}
}

func Claude(args []string) {
	cfg := config.LoadConfig()

	// Check for --opencode, --anthropic, and --model flags
	authMode := ""
	model := ""
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--opencode", "-o":
			authMode = "opencode"
		case "--anthropic", "-a":
			authMode = "anthropic"
		case "--model", "-m":
			if i+1 < len(args) {
				model = args[i+1]
				filteredArgs = append(filteredArgs, args[i], args[i+1])
				i++
			}
		default:
			filteredArgs = append(filteredArgs, args[i])
		}
	}
	args = filteredArgs

	// Handle auth conflict
	if authMode == "" && hasAuthConflict() {
		authMode = promptAuthChoice()
	}

	// Auto-start proxy if not running (unless using anthropic mode)
	if authMode != "anthropic" && !isProxyRunning() {
		port := getProxyPort()
		fmt.Printf("Proxy not running, starting on port %d...\n", port)
		ProxyBackground(port, "127.0.0.1", false, false)
		// Give the proxy a moment to start
		time.Sleep(500 * time.Millisecond)
	}

	settings, _ := claude.LoadSettings()
	originalHelper, hadHelper := settings["apiKeyHelper"]

	// Temporarily adjust settings based on auth mode
	if authMode == "anthropic" {
		// Remove apiKeyHelper to use Console auth
		delete(settings, "apiKeyHelper")
		claude.SaveSettings(settings)
	}

	mode := "direct"
	if authMode == "anthropic" {
		mode = "anthropic-console"
	} else if strings.Contains(cfg.Target, "opencode") {
		mode = "opencode"
	} else if strings.Contains(cfg.Target, "gateway.ai.cloudflare.com") {
		mode = "ai-gateway"
	} else if strings.Contains(cfg.Target, "anthropic.com") {
		mode = "anthropic"
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "not set (direct)"
	}

	fmt.Println()
	fmt.Println("\033[90m┌─────────────────────────────────────────────────────┐\033[0m")
	fmt.Printf("\033[90m│\033[0m \033[1mclaude-opencode-proxy\033[0m                               \033[90m│\033[0m\n")
	fmt.Printf("\033[90m│\033[0m Mode: %-45s \033[90m│\033[0m\n", mode)
	if model != "" {
		fmt.Printf("\033[90m│\033[0m Model: %-44s \033[90m│\033[0m\n", Truncate(model, 44))
	}
	if authMode != "anthropic" {
		fmt.Printf("\033[90m│\033[0m Base: %-45s \033[90m│\033[0m\n", Truncate(baseURL, 45))
		fmt.Printf("\033[90m│\033[0m Target: %-43s \033[90m│\033[0m\n", Truncate(cfg.Target, 43))
	}
	fmt.Println("\033[90m└─────────────────────────────────────────────────────┘\033[0m")
	fmt.Println()

	cmd := exec.Command("claude", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	cmd.Run()

	// Restore original settings after claude exits
	if authMode == "anthropic" && hadHelper {
		settings["apiKeyHelper"] = originalHelper
		claude.SaveSettings(settings)
	}
}

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ModelInfo represents a model from the API response
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	Type        string `json:"type"`
}

// ModelsResponse represents the API response for listing models
type ModelsResponse struct {
	Data    []ModelInfo `json:"data"`
	HasMore bool        `json:"has_more"`
	FirstID string      `json:"first_id"`
	LastID  string      `json:"last_id"`
}

func Models(args []string) {
	cfg := config.LoadConfig()

	// Parse args
	jsonOutput := false
	source := "" // empty means use config target
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json", "-j":
			jsonOutput = true
		case "--source", "-s":
			if i+1 < len(args) {
				source = args[i+1]
				i++
			}
		}
	}

	client, err := config.CreateHTTPClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create HTTP client: %v", err)
	}

	// Build the models endpoint URL
	var modelsURL string
	var useApiKey bool

	switch source {
	case "anthropic", "direct":
		modelsURL = "https://api.anthropic.com/v1/models"
		useApiKey = true
	case "":
		// Try the configured target first
		modelsURL = strings.TrimSuffix(cfg.Target, "/") + "/v1/models"
		useApiKey = false
	default:
		modelsURL = strings.TrimSuffix(source, "/") + "/v1/models"
		useApiKey = false
	}

	req, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Get auth token and set appropriate headers
	token, authType, err := config.GetToken(cfg)
	if err != nil {
		log.Fatalf("Failed to get auth token: %v", err)
	}

	if useApiKey && authType == "apikey" {
		req.Header.Set("x-api-key", token)
	} else if cfg.CfAccess && !useApiKey {
		req.Header.Set("cf-access-token", token)
		if cfg.CfClientID != "" && cfg.CfClientSecret != "" {
			req.Header.Set("CF-Access-Client-Id", cfg.CfClientID)
			req.Header.Set("CF-Access-Client-Secret", cfg.CfClientSecret)
		}
	} else if authType == "apikey" {
		req.Header.Set("x-api-key", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to fetch models: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// If the configured target doesn't support /v1/models, show helpful message
		if source == "" && resp.StatusCode == 404 {
			fmt.Println("The configured target doesn't support the /v1/models endpoint.")
			fmt.Println()
			fmt.Println("To list models from Anthropic directly (requires API key):")
			fmt.Println("  claude-opencode-proxy models --source anthropic")
			fmt.Println()
			fmt.Println("Or configure an API key:")
			fmt.Println("  claude-opencode-proxy config --auth-type apikey --api-key YOUR_KEY")
			fmt.Println()
			fmt.Println("Common Claude models:")
			printStaticModelList()
			return
		}
		log.Fatalf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var modelsResp ModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(modelsResp, "", "  ")
		fmt.Println(string(output))
		return
	}

	// Sort models by ID for consistent output
	models := modelsResp.Data
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	fmt.Printf("Available models from %s:\n\n", cfg.Target)

	// Group models by family
	families := make(map[string][]ModelInfo)
	for _, m := range models {
		family := getModelFamily(m.ID)
		families[family] = append(families[family], m)
	}

	// Print models grouped by family
	familyOrder := []string{"claude-3-5", "claude-3", "claude-2", "other"}
	for _, family := range familyOrder {
		if modelList, ok := families[family]; ok && len(modelList) > 0 {
			fmt.Printf("  %s:\n", familyDisplayName(family))
			for _, m := range modelList {
				displayName := m.ID
				if m.DisplayName != "" {
					displayName = m.DisplayName
				}
				created := ""
				if m.CreatedAt != "" {
					if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
						created = fmt.Sprintf(" (%s)", t.Format("Jan 2006"))
					}
				}
				fmt.Printf("    • %s%s\n", displayName, created)
			}
			fmt.Println()
		}
	}

	fmt.Printf("Total: %d models\n", len(models))
}

func getModelFamily(modelID string) string {
	if strings.Contains(modelID, "claude-3-5") || strings.Contains(modelID, "claude-3.5") {
		return "claude-3-5"
	}
	if strings.Contains(modelID, "claude-3") {
		return "claude-3"
	}
	if strings.Contains(modelID, "claude-2") {
		return "claude-2"
	}
	return "other"
}

func familyDisplayName(family string) string {
	switch family {
	case "claude-3-5":
		return "Claude 3.5"
	case "claude-3":
		return "Claude 3"
	case "claude-2":
		return "Claude 2"
	default:
		return "Other"
	}
}

func printStaticModelList() {
	fmt.Println("  Claude 3.5:")
	fmt.Println("    • claude-3-5-sonnet-20241022 (latest)")
	fmt.Println("    • claude-3-5-sonnet-20240620")
	fmt.Println("    • claude-3-5-haiku-20241022")
	fmt.Println()
	fmt.Println("  Claude 3:")
	fmt.Println("    • claude-3-opus-20240229")
	fmt.Println("    • claude-3-sonnet-20240229")
	fmt.Println("    • claude-3-haiku-20240307")
	fmt.Println()
	fmt.Println("  Claude 4:")
	fmt.Println("    • claude-sonnet-4-20250514")
	fmt.Println("    • claude-opus-4-20250514")
	fmt.Println()
	fmt.Println("Note: Actual availability depends on your access level.")
}
