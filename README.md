# Catan Engine

A high-performance, terminal-based Settlers of Catan engine and simulation environment written in Go.

## Overview

The Catan Engine provides a robust implementation of the Settlers of Catan rules, featuring a rich Terminal User Interface (TUI) and a "Dungeon Master" (DM) mode for automated game management and replay generation.

### Key Features

*   **TUI Dashboard**: A multi-tabbed terminal interface built with [Bubbletea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).
*   **Rule Engine**: Full implementation of base game rules, including resource distribution, building, trading, and special cards.
*   **Dungeon Master (DM) Mode**: Command-line interface for running simulations, replaying game logs, and processing state updates.
*   **Theming System**: Highly customizable visual themes via `themes.yaml`, supporting plain text, Unicode, and Emoji-based rendering.
*   **Replay System**: Headless rendering of game logs into ANSI frames, which can be converted to MP4 or GIF.

## Architecture

The project follows the Model-View-Update (MVU) architecture provided by Bubbletea.

*   **Model**: Maintains the full game state (`GameState`), including the board topology, player inventories, and game log.
*   **Update**: Handles messages (user input, timer ticks, DM commands) and transitions the state.
*   **View**: Renders the game state into a string for the terminal, utilizing Lipgloss for sophisticated layouts and styling.

### DM Interface

The `dm` subcommand is used for headless operations:

*   `catan dm playback`: Reads `game.yaml` and renders each turn as an ANSI frame in the `frames/` directory.
*   `catan dm simulate`: Runs a full automated simulation of a game.

## Getting Started

### Prerequisites

*   Go 1.21+
*   Python 3 (for asset generation)
*   FFmpeg (for GIF/MP4 conversion)

### Installation

```bash
make build
```

### Running the TUI

```bash
make run
```

## Configuration

*   **`topology.yaml`**: Defines the board structure (vertices, edges, hexes) and their spatial relationships.
*   **`board.txt`**: A template used for rendering the visual board.
*   **`themes.yaml`**: Configures colors and icons used in the TUI.
*   **`game.yaml`**: Stores the current game state and turn log.

## License

MIT
