package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var (
	ConfigDir  = filepath.Join(os.Getenv("HOME"), ".config/claude-opencode-proxy")
	ConfigFile = filepath.Join(ConfigDir, "config.json")
	EnvFile    = filepath.Join(ConfigDir, "env")
	LogFile    = filepath.Join(ConfigDir, "proxy.log")
	PidFile    = filepath.Join(ConfigDir, "proxy.pid")
)

type Config struct {
	Target         string `json:"target"`
	AuthType       string `json:"auth_type"`
	APIKey         string `json:"api_key"`
	LoginURL       string `json:"login_url"`
	CfAccess       bool   `json:"cf_access"`
	CfClientID     string `json:"cf_client_id"`
	CfClientSecret string `json:"cf_client_secret"`
	Proxy          string `json:"proxy,omitempty"`
	CACert         string `json:"ca_cert,omitempty"`
	InsecureSkip   bool   `json:"insecure_skip_verify,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Target:   "https://opencode.custom.dev/anthropic",
		AuthType: "opencode",
		APIKey:   filepath.Join(os.Getenv("HOME"), ".local/share/opencode/auth.json"),
		LoginURL: "https://opencode.custom.dev",
		CfAccess: true,
	}
}

func LoadConfig() Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	return cfg
}

func SaveConfig(cfg Config) error {
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, data, 0644)
}

func CreateHTTPClient(cfg Config) (*http.Client, error) {
	transport := &http.Transport{}

	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	if cfg.CACert != "" || cfg.InsecureSkip {
		tlsConfig := &tls.Config{}

		if cfg.InsecureSkip {
			tlsConfig.InsecureSkipVerify = true
		}

		if cfg.CACert != "" {
			caCert, err := os.ReadFile(cfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA cert: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA cert")
			}
			tlsConfig.RootCAs = caCertPool
		}

		transport.TLSClientConfig = tlsConfig
	}

	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Minute,
	}, nil
}

func GetToken(cfg Config) (string, string, error) {
	if cfg.AuthType == "apikey" {
		return cfg.APIKey, "apikey", nil
	}

	data, err := os.ReadFile(cfg.APIKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to read auth file: %w", err)
	}

	var authMap map[string]struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &authMap); err != nil {
		return "", "", fmt.Errorf("failed to parse auth file: %w", err)
	}

	auth, ok := authMap[cfg.LoginURL]
	if !ok {
		return "", "", fmt.Errorf("no token found for %s", cfg.LoginURL)
	}

	return auth.Token, "opencode", nil
}

func APIKeyHelper(cfg Config) string {
	if cfg.AuthType == "apikey" {
		return fmt.Sprintf("echo '%s'", cfg.APIKey)
	}
	return fmt.Sprintf(`jq -r '."%s".token' %s`, cfg.LoginURL, cfg.APIKey)
}
