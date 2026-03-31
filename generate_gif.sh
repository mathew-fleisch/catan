#!/bin/bash
set -e

FFMPEG="/usr/bin/ffmpeg"
MODE=${1:-playback}

# Default settings for TUI
FPS=5

# Function to freeze last frame
freeze_frames() {
    local dir=$1
    if [ -d "$dir" ] && [ -n "$(ls -A $dir 2>/dev/null)" ]; then
        echo "Freezing last frame in $dir..."
        local last_frame=$(ls $dir/frame_*.png | sort | tail -n 1)
        local last_num=$(echo $(basename $last_frame) | grep -oP '\d+' | head -n 1)
        local last_val=$((10#$last_num))

        for i in {1..15}; do
            local new_val=$((last_val + i))
            local new_num=$(printf "%04d" $new_val)
            cp "$last_frame" "$dir/frame_$new_num.png"
        done
    else
        echo "Warning: Directory $dir is empty or does not exist. Skipping freeze."
    fi
}

render_video() {
    local input_dir=$1
    local output_file=$2
    local rate=$3
    echo "Generating MP4: $output_file from $input_dir..."
    $FFMPEG -y -framerate $rate -i $input_dir/frame_%04d.png \
        -c:v libx264 -pix_fmt yuv420p \
        $output_file
}

render_gif() {
    local input_dir=$1
    local output_file=$2
    local rate=$3
    echo "Generating GIF: $output_file from $input_dir..."
    $FFMPEG -y -framerate $rate -i $input_dir/frame_%04d.png \
        -vf "scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64[p];[s1][p]paletteuse" \
        $output_file
}

if [ "$MODE" == "mp4" ]; then
    echo "Rendering TUI MP4 only..."
    ./catan dm playback
    freeze_frames "frames"
    render_video "frames" "catan_replay.mp4" $FPS

elif [ "$MODE" == "gif" ]; then
    echo "Rendering TUI GIF only..."
    ./catan dm playback
    freeze_frames "frames"
    render_gif "frames" "catan_preview.gif" $FPS

elif [ "$MODE" == "simulate" ]; then
    echo "Simulating and rendering TUI assets..."
    ./catan dm simulate
    freeze_frames "frames"
    render_video "frames" "catan_replay.mp4" $FPS
    render_gif "frames" "catan_preview.gif" $FPS

else
    echo "Rendering TUI playback (MP4 and GIF)..."
    ./catan dm playback
    freeze_frames "frames"
    render_video "frames" "catan_replay.mp4" $FPS
    render_gif "frames" "catan_preview.gif" $FPS
fi

echo "Done!"
ls -lh catan_replay.mp4 catan_preview.gif 2>/dev/null || true
