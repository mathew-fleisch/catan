# Catan Game Replay System

This project now supports headlessly rendering game replays into animated GIFs.

## Components

1.  **`catan-go dm playback`**: Replays the `game.yaml` log and saves each step as an ANSI text frame in `frames/`.
2.  **`render_frames.py`**: Uses the Python `rich` library to convert ANSI text frames into high-quality SVG images in `output/`.
3.  **`generate_gif.sh`**: Orchestrates the entire process and uses `ffmpeg` (with `librsvg`) to stitch SVGs into `catan_game.gif`.

## Requirements

-   **Go**: To build and run the `catan-go` binary.
-   **Python 3**: With the `rich` library installed (`pip install rich`).
-   **FFmpeg**: Compiled with `librsvg` support (standard in most modern distributions like Ubuntu 24.04).

## How to use

1.  Ensure you have a `game.yaml` with a populated `log` section.
2.  Run the generation script:
    ```bash
    cd catan-go
    ./generate_gif.sh
    ```
3.  The resulting `catan_game.gif` will be in the `catan-go` directory.

## GitHub Actions Integration

The `Dungeon Master` pipeline can now be updated to run `./generate_gif.sh` after each valid move and upload the resulting GIF to the PR or host it on GitHub Pages.
