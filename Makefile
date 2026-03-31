BINARY=catan
SRC=main.go
GO_BUILD=go build
PORT=8080

##@ Misc stuff

.PHONY: help
help: ## this
	@echo "+---------------------------------------------------------------+"
	@echo "|    ____      _                                                |"
	@echo "|   / ___|__ _| |_ __ _ _ __                                    |"
	@echo "|  | |   / _' | __/ _' | '_ \                                   |"
	@echo "|  | |__| (_| | || (_| | | | |                                  | "
	@echo "|   \____\__,_|\__\__,_|_| |_|                                  |"
	@echo "|                                                               |"
	@echo "|  makefile targets                                             |"
	@echo "+---------------------------------------------------------------+"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build and Run

.PHONY: build
build: ## Build the catan binary
	$(GO_BUILD) -o $(BINARY) $(SRC)

.PHONY: run
run: build ## Build and run the catan TUI
	./$(BINARY)

.PHONY: clean
clean: ## Remove binaries and generated frames
	rm -f $(BINARY)
	rm -rf frames/ vector_frames/
	rm -f catan_preview.gif catan_replay.mp4 vector_preview.gif vector_replay.mp4

##@ DM Actions

.PHONY: playback
playback: build ## Render TUI frames from existing game log
	./$(BINARY) dm playback

.PHONY: simulate
simulate: build ## Simulate a full game and render TUI frames
	./$(BINARY) dm simulate

.PHONY: vector-playback
vector-playback: build ## Render vector frames from existing game log
	./$(BINARY) dm vector-playback

.PHONY: vector-simulate
vector-simulate: build ## Simulate a full game and render vector frames
	./$(BINARY) dm vector-simulate

##@ Asset Generation

.PHONY: gif
gif: build ## Generate MP4 and GIF assets (hybrid TUI/Vector)
	./generate_gif.sh playback

.PHONY: vector-gif
vector-gif: build ## Generate MP4 and GIF assets (Vector only)
	./generate_gif.sh vector

##@ Server

.PHONY: server-start
server-start: ## Start a local python server to view assets (port 8080)
	@echo "Starting server on http://localhost:$(PORT)..."
	@python3 -m http.server $(PORT) > /dev/null 2>&1 & echo $$! > .server.pid
	@echo "Server started with PID $$(cat .server.pid)"

.PHONY: server-stop
server-stop: ## Stop the local python server
	@if [ -f .server.pid ]; then \
		echo "Stopping server with PID $$(cat .server.pid)..."; \
		kill $$(cat .server.pid) && rm .server.pid; \
		echo "Server stopped."; \
	else \
		echo "No server PID file found."; \
		pkill -f "python3 -m http.server $(PORT)" || true; \
	fi
