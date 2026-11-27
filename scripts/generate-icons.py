"""Render the platform icons from packaging/lanboat.svg (Pillow and GdkPixbuf with SVG support)."""

import os
from pathlib import Path
import subprocess

import gi
from PIL import Image

gi.require_version("GdkPixbuf", "2.0")
from gi.repository import GdkPixbuf

ROOT = Path(__file__).resolve().parents[1]
PACKAGING = ROOT / "packaging"
DESKTOP_SIZES = (16, 32, 48, 64, 128, 256)


def main():
    source = GdkPixbuf.Pixbuf.new_from_file_at_scale(str(PACKAGING / "lanboat.svg"), 1024, 1024, True)
    master = Image.frombytes("RGBA", (1024, 1024), source.get_pixels(), "raw", "RGBA", source.get_rowstride())
    images = {size: master.resize((size, size), Image.Resampling.LANCZOS)
              for size in (*DESKTOP_SIZES, 512, 1024)}
    for size in DESKTOP_SIZES:
        path = PACKAGING / "icons" / f"{size}x{size}" / "lanboat.png"
        path.parent.mkdir(parents=True, exist_ok=True)
        images[size].save(path, optimize=True)

    ico = PACKAGING / "windows" / "lanboat.ico"
    images[256].save(ico, sizes=[(size, size) for size in DESKTOP_SIZES],
                     append_images=list(images.values()))
    images[1024].save(PACKAGING / "macos" / "Lanboat.icns",
                      append_images=list(images.values()))
    for arch in ("amd64", "arm64"):
        subprocess.run(
            ["go", "run", "github.com/akavel/rsrc@v0.10.2", "-arch", arch,
             "-ico", str(ico), "-o", f"icon_windows_{arch}.syso"],
            cwd=ROOT, env=dict(os.environ, GOWORK="off"), check=True,
        )


if __name__ == "__main__":
    main()
