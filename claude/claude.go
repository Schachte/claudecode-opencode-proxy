package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/schachte/claudecode-opencode-proxy/config"
)

var (
	ClaudeSettings = filepath.Join(os.Getenv("HOME"), ".claude/settings.json")
	BackupSettings = filepath.Join(os.Getenv("HOME"), ".claude/settings.json.backup")
	ShellMarker    = "# claude-opencode-proxy"
)

func LoadSettings() (map[string]interface{}, error) {
	data, err := os.ReadFile(ClaudeSettings)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func SaveSettings(settings map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(ClaudeSettings), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ClaudeSettings, data, 0644)
}

func GetShellRC() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(os.Getenv("HOME"), ".zshrc")
	}
	return filepath.Join(os.Getenv("HOME"), ".bashrc")
}

func UpdateShellRC(port int, enable bool) {
	rcFile := GetShellRC()
	data, _ := os.ReadFile(rcFile)
	lines := strings.Split(string(data), "\n")

	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, ShellMarker) {
			newLines = append(newLines, line)
		}
	}

	if enable {
		newLines = append(newLines, fmt.Sprintf("export ANTHROPIC_BASE_URL=http://127.0.0.1:%d %s", port, ShellMarker))
		newLines = append(newLines, fmt.Sprintf("source %s 2>/dev/null %s", config.EnvFile, ShellMarker))
	}

	os.WriteFile(rcFile, []byte(strings.Join(newLines, "\n")), 0644)
	fmt.Printf("Updated: %s\n", rcFile)
}

func WriteEnvFile(port int) {
	os.MkdirAll(config.ConfigDir, 0755)
	content := fmt.Sprintf("export ANTHROPIC_BASE_URL=http://127.0.0.1:%d\n", port)
	os.WriteFile(config.EnvFile, []byte(content), 0644)
	fmt.Printf("Updated: %s\n", config.EnvFile)
}
