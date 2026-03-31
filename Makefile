BINARY=catan
SRC=main.go
GO_BUILD=go build

##@ Misc stuff

.PHONY: help
help: ## this
	@echo "+---------------------------------------------------------------+"
	@echo "|    ____      _                   _____ _   _ _____            |"
	@echo "|   / ___|__ _| |_ __ _ _ __      |_   _| | | |_   _|           |"
	@echo "|  | |   / _' | __/ _' | '_ \       | | | | | | | |             |"
	@echo "|  | |__| (_| | || (_| | | | |      | | | |_| | | |             |"
	@echo "|   \____\__,_|\__\__,_|_| |_|      |_|  \___/  |_|             |"
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
