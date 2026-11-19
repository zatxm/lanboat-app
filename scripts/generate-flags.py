"""Rasterize Noto flag emoji into a small atlas for Fego's desktop renderer."""
import json
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

root = Path(__file__).resolve().parents[1]
font = ImageFont.truetype("/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf", 109)
atlas = Image.new("RGBA", (26 * 32, 26 * 24))
for code in json.loads((root / "internal/ruledata/assets/regions.json").read_text()):
    if len(code) != 2:
        continue
    x, y = (ord(c) - ord("a") for c in code)
    text = chr(0x1F1E6 + x) + chr(0x1F1E6 + y)
    glyph = Image.new("RGBA", (160, 160))
    ImageDraw.Draw(glyph).text((0, 0), text, font=font, embedded_color=True)
    if bounds := glyph.getbbox():
        glyph = glyph.crop(bounds)
        glyph.thumbnail((32, 24), Image.Resampling.LANCZOS)
        atlas.paste(glyph, (32*x+(32-glyph.width)//2, 24*y+(24-glyph.height)//2))
atlas.save(root / "internal/native/flags.png", optimize=True)
