package main

import (
	"fmt"
	"os"

	"github.com/schachte/claudecode-opencode-proxy/cmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	parsePort := func() int {
		port := 8787
		for i := 0; i < len(args); i++ {
			if (args[i] == "-p" || args[i] == "--port") && i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &port)
			}
		}
		return port
	}

	switch command {
	case "run":
		cmd.Claude(args)

	case "serve":
		port := 8787
		bindAddr := "127.0.0.1"
		verbose := false
		foreground := false
		quiet := false
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "-p", "--port":
				if i+1 < len(args) {
					fmt.Sscanf(args[i+1], "%d", &port)
					i++
				}
			case "-b", "--bind":
				if i+1 < len(args) {
					bindAddr = args[i+1]
					i++
				}
			case "-v", "--verbose":
				verbose = true
			case "-f", "--foreground":
				foreground = true
			case "-q", "--quiet":
				quiet = true
			}
		}
		if foreground {
			cmd.ProxyForeground(port, bindAddr, verbose, quiet)
		} else {
			cmd.ProxyBackground(port, bindAddr, verbose, quiet)
		}

	case "stop", "kill":
		cmd.ProxyStop()

	case "logs", "tail":
		cmd.ProxyLogs()

	case "enable":
		cmd.Enable(parsePort())

	case "disable":
		cmd.Disable()

	case "login":
		cmd.Login(args)

	case "status":
		cmd.Status()

	case "config":
		cmd.Config(args)

	case "env":
		cmd.Env(parsePort())

	case "models":
		cmd.Models(args)

	case "-h", "--help", "help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	usage := `Usage: claude-opencode-proxy <command> [options]

Commands:
  run        Launch claude with proxy status banner
  serve      Start the proxy server (background by default)
  stop       Stop the background proxy server (alias: kill)
  logs       Tail the proxy logs (alias: tail)
  enable     Configure Claude Code to use proxy
  disable    Restore Claude Code to default auth
  login      Authenticate with OpenCode
  status     Show current configuration
  config     View or modify proxy configuration
  env        Print environment variables
  models     List available models from connected source

Options for 'run':
  -o, --opencode          Use OpenCode proxy (skip prompt)
  -a, --anthropic         Use Anthropic Console (skip prompt)
  [args]                  Arguments passed to claude

Options for 'serve':
  -p, --port <port>       Port to listen on (default: 8787)
  -b, --bind <addr>       Bind address (default: 127.0.0.1, use 0.0.0.0 for Docker)
  -f, --foreground        Run in foreground (default: background)
  -v, --verbose           Enable verbose logging
  -q, --quiet             Suppress all log output

Options for 'enable', 'env':
  -p, --port <port>       Port for ANTHROPIC_BASE_URL (default: 8787)

Options for 'login':
  --target <url>          Auth target URL (default: from config)

Options for 'models':
  -j, --json              Output as JSON
  -s, --source <url>      Query specific source (default: configured target)
                          Use "anthropic" for direct Anthropic API

Options for 'config':
  --target <url>          Upstream API URL
  --auth-type <type>      Auth type: opencode, apikey
  --auth-file <path>      Path to auth file or API key
  --auth-key <key>        Key in auth JSON file
  --cf-access             Enable Cloudflare Access headers
  --no-cf-access          Disable Cloudflare Access headers
  --cf-client-id <id>     CF Access service token client ID
  --cf-client-secret <s>  CF Access service token secret
  --proxy <url>           HTTP/HTTPS proxy URL (e.g., http://proxy:8080)
  --ca-cert <path>        Path to custom CA certificate (PEM format)
  --insecure-skip-verify  Skip TLS certificate verification (not recommended)
  --no-insecure-skip-verify  Enable TLS certificate verification
  --reset                 Reset to defaults
`
	fmt.Print(usage)
}
