#!/bin/bash
set -e

FFMPEG="/usr/bin/ffmpeg"
MODE=${1:-playback}

# 1. Render PNG frames directly from Go binary
if [ "$MODE" == "vector" ]; then
    echo "Rendering VECTOR frames..."
    ./catan dm vector-playback
    FRAME_DIR="vector_frames"
    OUT_VIDEO="vector_replay.mp4"
    OUT_GIF="vector_preview.gif"
    FPS=5
elif [ "$MODE" == "simulate" ]; then
    echo "Simulating full game and rendering PNG frames (both types)..."
    ./catan dm simulate
    ./catan dm vector-playback
    FRAME_DIR="frames"
    OUT_VIDEO="catan_replay.mp4"
    OUT_GIF="catan_preview.gif"
    FPS=5
elif [ "$MODE" == "vector-simulate" ]; then
    echo "Simulating full game and rendering VECTOR frames..."
    ./catan dm vector-simulate
    FRAME_DIR="vector_frames"
    OUT_VIDEO="vector_replay.mp4"
    OUT_GIF="vector_preview.gif"
    FPS=5
else
    echo "Rendering PNG frames from existing game log..."
    ./catan dm playback
    FRAME_DIR="frames"
    OUT_VIDEO="catan_replay.mp4"
    OUT_GIF="catan_preview.gif"
    FPS=2
fi

# 2. Freeze the last frame (Duplicate it 15 times for pause at end)
echo "Freezing last frame in $FRAME_DIR..."
LAST_FRAME=$(ls $FRAME_DIR/frame_*.png | sort | tail -n 1)
LAST_NUM=$(echo $(basename $LAST_FRAME) | grep -oP '\d+' | head -n 1)
LAST_VAL=$((10#$LAST_NUM))

for i in {1..15}; do
    NEW_VAL=$((LAST_VAL + i))
    NEW_NUM=$(printf "%04d" $NEW_VAL)
    cp "$LAST_FRAME" "$FRAME_DIR/frame_$NEW_NUM.png"
done

# 3. Generate MP4
echo "Generating High-Quality MP4: $OUT_VIDEO..."
$FFMPEG -y -framerate $FPS -i $FRAME_DIR/frame_%04d.png \
    -c:v libx264 -pix_fmt yuv420p \
    $OUT_VIDEO

# 4. Generate Lightweight Preview GIF
echo "Generating Lightweight Preview GIF: $OUT_GIF..."
$FFMPEG -y -framerate $FPS -i $FRAME_DIR/frame_%04d.png \
    -vf "scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64[p];[s1][p]paletteuse" \
    $OUT_GIF

echo "Done!"
ls -lh $OUT_VIDEO $OUT_GIF
