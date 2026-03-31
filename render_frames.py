import os
from rich.console import Console
from rich.text import Text
import glob

def render():
    os.makedirs("output", exist_ok=True)
    frames = sorted(glob.glob("frames/frame_*.txt"))
    
    for frame_path in frames:
        with open(frame_path, "r") as f:
            content = f.read()
        
        # Create a fresh console for each frame to get a clean SVG
        console = Console(width=120, height=45, record=True, force_terminal=True)
        text = Text.from_ansi(content)
        console.print(text)
        
        frame_num = os.path.basename(frame_path).split("_")[1].split(".")[0]
        svg_path = f"output/frame_{frame_num}.svg"
        # We must explicitly set width/height or FFmpeg might treat it as 100x100
        # The viewBox is ~1482x1270, so let's set a fixed pixel size for the rasterizer
        console.save_svg(svg_path, title=f"Catan Step {frame_num}")
        
        # Seditiously edit the SVG to add width/height if missing (Rich sometimes omits them)
        with open(svg_path, "r") as f:
            svg_content = f.read()
        if 'width="' not in svg_content[:200]:
            svg_content = svg_content.replace('<svg ', '<svg width="1482" height="1270" ', 1)
            with open(svg_path, "w") as f:
                f.write(svg_content)
        
        print(f"Rendered {svg_path}")

if __name__ == "__main__":
    render()
