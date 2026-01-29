
BINARY := claude-opencode-proxy
PORT := 8787

TARGET ?=
LOGIN_URL ?=

.PHONY: all build install config login enable run serve stop status clean help

build:
	@echo "Building $(BINARY)..."
	go build -o $(BINARY) .
	@echo "Done: ./$(BINARY)"

install: build
	@echo "Installing $(BINARY) to /usr/local/bin..."
	sudo mv $(BINARY) /usr/local/bin/
	@echo "Done: $(BINARY) is now available in PATH"

config:
ifndef TARGET
	$(error TARGET is required. Usage: make config TARGET=https://your-server/anthropic LOGIN_URL=https://your-server)
endif
ifndef LOGIN_URL
	$(error LOGIN_URL is required. Usage: make config TARGET=https://your-server/anthropic LOGIN_URL=https://your-server)
endif
	@echo "Configuring proxy..."
	./$(BINARY) config --target $(TARGET) --login-url $(LOGIN_URL)

config-apikey:
ifndef TARGET
	$(error TARGET is required. Usage: make config-apikey TARGET=https://api.anthropic.com/v1 API_KEY=sk-ant-xxx)
endif
ifndef API_KEY
	$(error API_KEY is required. Usage: make config-apikey TARGET=https://api.anthropic.com/v1 API_KEY=sk-ant-xxx)
endif
	@echo "Configuring proxy with API key..."
	./$(BINARY) config --target $(TARGET) --auth-type apikey --api-key $(API_KEY)

login:
	@echo "Logging in to OpenCode..."
	./$(BINARY) login

enable:
	@echo "Enabling proxy..."
	./$(BINARY) enable

serve:
	@echo "Starting proxy server on port $(PORT)..."
	./$(BINARY) serve -p $(PORT)

run:
	./$(BINARY) run

stop:
	./$(BINARY) stop

status:
	./$(BINARY) status

disable:
	./$(BINARY) disable

logs:
	./$(BINARY) logs

clean:
	rm -f $(BINARY)

setup: build config login enable serve
	@echo ""
	@echo "Setup complete! Proxy is running."
	@echo "Run 'make run' or './$(BINARY) run' to start Claude."

setup-apikey: build config-apikey enable serve
	@echo ""
	@echo "Setup complete! Proxy is running."
	@echo "Run 'make run' or './$(BINARY) run' to start Claude."

start: setup run

help:
	@echo "Claude OpenCode Proxy Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build                                    Build the binary"
	@echo "  make install                                  Build and install to /usr/local/bin"
	@echo "  make config TARGET=<url> LOGIN_URL=<url>     Configure for OpenCode auth"
	@echo "  make config-apikey TARGET=<url> API_KEY=<key> Configure for API key auth"
	@echo "  make login                                    Login to OpenCode"
	@echo "  make enable                                   Enable proxy in Claude settings"
	@echo "  make serve                                    Start proxy server"
	@echo "  make run                                      Run Claude with proxy"
	@echo "  make stop                                     Stop proxy server"
	@echo "  make status                                   Show current status"
	@echo "  make disable                                  Disable proxy mode"
	@echo "  make logs                                     View proxy logs"
	@echo "  make clean                                    Remove build artifacts"
	@echo ""
	@echo "All-in-one commands:"
	@echo "  make setup TARGET=<url> LOGIN_URL=<url>      Build + config + login + enable + serve"
	@echo "  make setup-apikey TARGET=<url> API_KEY=<key> Build + config + enable + serve (API key)"
	@echo "  make start TARGET=<url> LOGIN_URL=<url>      Full setup + run Claude"
	@echo ""
	@echo "Examples:"
	@echo "  make setup TARGET=https://myserver.com/anthropic LOGIN_URL=https://myserver.com"
	@echo "  make setup-apikey TARGET=https://api.anthropic.com/v1 API_KEY=sk-ant-xxx"
	@echo ""
	@echo "Variables:"
	@echo "  PORT       Proxy port (default: 8787)"
	@echo "  TARGET     Upstream API URL"
	@echo "  LOGIN_URL  OpenCode login URL"
	@echo "  API_KEY    Anthropic API key"
