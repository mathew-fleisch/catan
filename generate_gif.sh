#!/bin/bash
set -e

FFMPEG="/usr/bin/ffmpeg"
MODE=${1:-playback}

# Default settings
FPS=5
FRAME_DIR="frames"
OUT_VIDEO="catan_replay.mp4"
OUT_GIF="catan_preview.gif"

if [ "$MODE" == "vector" ]; then
    echo "Rendering VECTOR frames..."
    ./catan dm vector-playback
    FRAME_DIR="vector_frames"
    OUT_VIDEO="vector_replay.mp4"
    OUT_GIF="vector_preview.gif"
elif [ "$MODE" == "simulate" ]; then
    echo "Simulating full game and rendering BOTH TUI and VECTOR frames..."
    ./catan dm simulate
    ./catan dm vector-playback
    # Default playback behavior for simulate: MP4 from vector, GIF from TUI
    # We'll handle the freezing below manually to ensure both are covered
elif [ "$MODE" == "vector-simulate" ]; then
    echo "Simulating full game and rendering VECTOR frames..."
    ./catan dm vector-simulate
    FRAME_DIR="vector_frames"
    OUT_VIDEO="vector_replay.mp4"
    OUT_GIF="vector_preview.gif"
else
    echo "Rendering BOTH TUI and VECTOR frames for hybrid output..."
    ./catan dm playback
    ./catan dm vector-playback
fi

# Function to freeze last frame
freeze_frames() {
    local dir=$1
    echo "Freezing last frame in $dir..."
    local last_frame=$(ls $dir/frame_*.png | sort | tail -n 1)
    local last_num=$(echo $(basename $last_frame) | grep -oP '\d+' | head -n 1)
    local last_val=$((10#$last_num))

    for i in {1..15}; do
        local new_val=$((last_val + i))
        local new_num=$(printf "%04d" $new_val)
        cp "$last_frame" "$dir/frame_$new_num.png"
    done
}

# Special case for playback and simulate: produce hybrid assets
if [ "$MODE" == "playback" ] || [ "$MODE" == "simulate" ]; then
    freeze_frames "frames"
    freeze_frames "vector_frames"

    echo "Generating High-Quality MP4 from Vector: catan_replay.mp4..."
    $FFMPEG -y -framerate $FPS -i vector_frames/frame_%04d.png \
        -c:v libx264 -pix_fmt yuv420p \
        catan_replay.mp4

    echo "Generating Lightweight Preview GIF from TUI: catan_preview.gif..."
    $FFMPEG -y -framerate 2 -i frames/frame_%04d.png \
        -vf "scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64[p];[s1][p]paletteuse" \
        catan_preview.gif
else
    # Standard single-source generation
    freeze_frames "$FRAME_DIR"

    echo "Generating High-Quality MP4: $OUT_VIDEO..."
    $FFMPEG -y -framerate $FPS -i $FRAME_DIR/frame_%04d.png \
        -c:v libx264 -pix_fmt yuv420p \
        $OUT_VIDEO

    echo "Generating Preview GIF: $OUT_GIF..."
    $FFMPEG -y -framerate $FPS -i $FRAME_DIR/frame_%04d.png \
        -vf "scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64[p];[s1][p]paletteuse" \
        $OUT_GIF
fi

echo "Done!"
ls -lh catan_replay.mp4 catan_preview.gif vector_replay.mp4 vector_preview.gif 2>/dev/null || true
