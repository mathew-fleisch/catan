# Catan Engine Development Guide

## Terminal User Interface (Bubbletea MVU)

The Catan Engine's TUI is built using the [Bubbletea](https://github.com/charmbracelet/bubbletea) framework, which follows the **Model-View-Update** (MVU) pattern.

### 1. Model (`model` struct)

The `model` struct in `main.go` stores all state necessary to render the TUI. Key fields include:
- `state`: The core `GameState` (board, players, log).
- `viewMode`: Current active tab (0: Board, 1: Trading, 2: Chat).
- `isPlaying`: Controls whether the game playback is active.
- `historyIdx`: Index in the replay log when in playback mode.

### 2. Update (`Update` function)

The `Update` function handles all incoming messages:
- **`tea.KeyMsg`**: Processes keyboard inputs (e.g., `1`, `2`, `3` for tabs, `/` for chat).
- **`tickMsg`**: Triggered by a timer, used for advancing the game playback.
- **`gitUserCheckMsg`**: Asynchronous response from the GitHub API when a new player joins.
- **`simulationMsg`**: Triggered when a full game simulation is requested.

### 3. View (`View` function)

The `View` function is responsible for rendering the terminal output. It uses:
- **`renderBoard()`**: Dynamically draws the hex board using `board.txt` as a template.
- **`renderInventory()`**: Shows player resources, cards, and victory points.
- **`renderTrade()`**: Displays current trade offers and history.
- **`renderChat()`**: Displays the "PR Chat" log.

## Customizing Visual Themes (`themes.yaml`)

The engine supports dynamic theming via `themes.yaml`.

### Structure

- **`active`**: Specifies which theme to use (`text`, `emoji`, or `classic`).
- **`characters`**: Global mapping of resource names to Unicode characters or Emojis.
- **`themes`**: Contains individual theme definitions.
    - **`resources`**: Map of resource types (`wood`, `brick`, etc.) to character keys.
    - **`colors`**: HEX color codes for each resource.
    - **`board`**: Icons for game board elements (`settlement`, `city`, `robber`).
    - **`ui`**: Symbols for UI elements like the player cursor and playback status.
    - **`widths`**: Explicit character widths for proper layout alignment (crucial for Emojis).

### Example: Adding a New Theme

To create a custom theme, add a new entry under `themes`:

```yaml
themes:
  neon:
    resources:
      wood: "🌳"
      # ...
    colors:
      wood: "#39FF14"
      # ...
    widths:
      "🌳": 2
      # ...
```

## Board Topology and Rendering

The visual board in the TUI is a combination of a static template (`board.txt`) and a dynamic graph (`topology.yaml`).

### `topology.yaml`

This file defines the mathematical representation of the Catan board:
- **Vertices**: Points where settlements and cities are built. Each vertex has `x, y` coordinates and lists of adjacent edges and hexes.
- **Edges**: Paths between vertices where roads are built.
- **Hexes**: Resource-producing tiles. Each hex specifies its resource type, dice number, and the vertices that surround it.
- **Coordinates**: The `x, y` values in `topology.yaml` are mapped to the character grid in `board.txt`. Typically, `x` maps to the column (multiplied by a factor) and `y` maps to the line number.

### `board.txt`

This is a UTF-8 text file containing the ASCII/Unicode art for the board.
- The engine uses the coordinates from `topology.yaml` to "overlay" game elements (settlements, roads, robbers) onto the text template.
- Symbols like `●` (vertex), `╱`, `╲`, `|` (edges) are used as placeholders that the engine can colorize or replace based on the current game state.

## Rule Engine

The engine currently supports the following standard Catan rules:

### 1. Resource Distribution
- On each turn, two dice (2-12) are rolled.
- Players with settlements or cities adjacent to hexes matching the roll receive the corresponding resource (1 for settlement, 2 for city).
- A roll of "7" activates the robber.

### 2. Building
- **Roads**: Cost 1 Wood + 1 Brick. Must be connected to an existing road or settlement.
- **Settlements**: Cost 1 Wood + 1 Brick + 1 Sheep + 1 Wheat. Must be at least two edges away from any other settlement (the "distance rule").
- **Cities**: Cost 3 Ore + 2 Wheat. Replaces an existing settlement.

### 3. Development Cards
- Cost 1 Sheep + 1 Wheat + 1 Ore.
- Includes Knight, Victory Point, Road Building, Year of Plenty, and Monopoly cards.

### 4. Trading
- **Domestic Trade**: Trading resources between players at negotiated rates.
- **Maritime Trade**: Trading 4:1 with the bank, 3:1 with a general port, or 2:1 with a resource-specific port.

### 5. Special Bonuses
- **Longest Road**: Awarded to the player with a continuous road of at least 5 segments.
- **Largest Army**: Awarded to the player who has played at least 3 Knight cards.

## Rule Variations and Limitations
- The "Friendly Robber" rule is NOT implemented (the robber can block anyone).
- Combined trading/building phase is supported (Catan 5th Edition rules).

## GitHub Integration (PR Chat)

The engine features a "PR Chat" system that allows players to communicate via GitHub Pull Request comments directly from the TUI.

### How it Works
1.  **Fetching Comments**: The TUI periodically polls the GitHub Issues API (`/repos/{repo}/issues/{pr}/comments`) to retrieve the latest comments from the active Pull Request.
2.  **Rendering**: These comments are parsed and displayed in the "3: CHAT" tab of the dashboard.
3.  **Posting Comments**: When a player types a message in the TUI (using the `/` key), the engine sends a `POST` request to the same GitHub API endpoint, effectively adding a new comment to the PR.

### Configuration
The following environment variables must be set for the integration to function:
- `GIT_TOKEN`: A GitHub Personal Access Token with `repo` scope.
- `GITHUB_REPOSITORY`: The full name of the repository (e.g., `user/settlers-of-catan`).
- `GITHUB_PR_NUMBER`: The ID of the active Pull Request.

If these variables are not set, the engine operates in **Local Chat Mode**, where messages are only stored in memory for the current session.

## Adding New Rules

Rules are implemented in `main.go` within the `GameState` logic. When adding a new rule:
1.  Update the `GameState` struct to accommodate new state.
2.  Add a corresponding method to check for rule compliance.
3.  Implement the rule's side effects (e.g., resource transfers) in the state transition logic.
4.  Ensure the `log` in `game.yaml` captures the new action for playback compatibility.
