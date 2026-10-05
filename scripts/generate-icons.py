#!/usr/bin/env python3
"""
SqlFox Brand Mark & Standard Icon Generator
Generates canonical multi-platform icon assets from geometric SVG master:
- web/dist/assets/logo.svg & docs/assets/logo.svg (Master Vector)
- web/dist/favicon.svg (Optimized browser favicon)
- web/dist/favicon.ico (Multi-res 16, 32, 48, 256 Windows/Browser ICO)
- web/dist/apple-touch-icon.png (180x180 iOS/iPad bookmark)
- web/dist/icon-192.png & web/dist/icon-512.png (PWA Manifest)
- web/dist/manifest.webmanifest (PWA configuration)
- assets/icon.ico & assets/icon.png (Desktop package archives)
- cmd/sqlfox/rsrc_windows_amd64.syso & arm64.syso (Windows PE embedded resource)
"""

import os
import sys
from PIL import Image, ImageDraw

# Canonical Brand Colors
ORANGE_LIGHT  = "#fb923c"  # Amber 400
ORANGE_MAIN   = "#f97316"  # Amber 500 (Primary Brand)
ORANGE_DEEP   = "#ea580c"  # Orange 600
ORANGE_DARK   = "#c2410c"  # Orange 700
CYAN_ACCENT   = "#38bdf8"  # Sky 400 (SqlFox Cyan)
CYAN_GLOW     = "#7dd3fc"  # Sky 300
CYAN_DARK     = "#0284c7"  # Sky 600
WHITE_PURE    = "#ffffff"  # Pure White
WHITE_SOFT    = "#f1f5f9"  # Slate 100
SLATE_LIGHT   = "#cbd5e1"  # Slate 300
SLATE_DARK    = "#1e293b"  # Slate 800
NOSE_COLOR    = "#090d16"  # Background Deep

# Low-Poly Geometric Fox Head Facets (viewBox: 0 0 512 512, Symmetrical along x=256)
FACETS = [
    # --- Ears (Outer & Inner) ---
    # Left Ear Outer
    ([(256, 140), (200, 140), (108, 48), (170, 180)], ORANGE_DARK),
    # Right Ear Outer
    ([(256, 140), (312, 140), (404, 48), (342, 180)], ORANGE_DARK),
    # Left Ear Outer Flank
    ([(108, 48), (76, 210), (170, 180)], ORANGE_DEEP),
    # Right Ear Outer Flank
    ([(404, 48), (436, 210), (342, 180)], ORANGE_DEEP),
    # Left Ear Inner (Shadow depth)
    ([(170, 180), (116, 68), (196, 150)], SLATE_DARK),
    # Right Ear Inner (Shadow depth)
    ([(342, 180), (396, 68), (316, 150)], SLATE_DARK),
    # Left Ear Inner Highlight
    ([(170, 180), (196, 150), (216, 190)], ORANGE_MAIN),
    # Right Ear Inner Highlight
    ([(342, 180), (316, 150), (296, 190)], ORANGE_MAIN),

    # --- Forehead & Crown ---
    # Forehead Upper Diamond Top
    ([(256, 140), (216, 190), (256, 224)], ORANGE_LIGHT),
    ([(256, 140), (296, 190), (256, 224)], ORANGE_MAIN),
    # Forehead Temple Left
    ([(200, 140), (170, 180), (216, 190)], ORANGE_DEEP),
    # Forehead Temple Right
    ([(312, 140), (342, 180), (296, 190)], ORANGE_DEEP),
    # Forehead Flank to Brow Left
    ([(170, 180), (150, 240), (216, 190)], ORANGE_MAIN),
    # Forehead Flank to Brow Right
    ([(342, 180), (362, 240), (296, 190)], ORANGE_MAIN),
    # Forehead Center Lower Diamond (Database / Core Emblem)
    ([(256, 224), (216, 190), (216, 260), (256, 290)], ORANGE_MAIN),
    ([(256, 224), (296, 190), (296, 260), (256, 290)], ORANGE_LIGHT),
    # Brow to Eye Top Left
    ([(216, 190), (150, 240), (216, 260)], ORANGE_DEEP),
    # Brow to Eye Top Right
    ([(296, 190), (362, 240), (296, 260)], ORANGE_DEEP),

    # --- Cheek Wings (Wide Fox Cheeks) ---
    # Left Upper Cheek Outer
    ([(76, 210), (52, 280), (150, 240)], ORANGE_MAIN),
    # Right Upper Cheek Outer
    ([(436, 210), (460, 280), (362, 240)], ORANGE_MAIN),
    # Left Cheek Wing Outer Spike
    ([(52, 280), (36, 330), (130, 310)], ORANGE_LIGHT),
    # Right Cheek Wing Outer Spike
    ([(460, 280), (476, 330), (382, 310)], ORANGE_LIGHT),
    # Left Mid Cheek
    ([(150, 240), (52, 280), (130, 310)], ORANGE_DEEP),
    # Right Mid Cheek
    ([(362, 240), (460, 280), (382, 310)], ORANGE_DEEP),

    # --- White / Light Cheek Tufts ---
    # Left Lower Cheek White Outer
    ([(36, 330), (100, 390), (130, 310)], WHITE_SOFT),
    # Right Lower Cheek White Outer
    ([(476, 330), (412, 390), (382, 310)], WHITE_SOFT),
    # Left Cheek Inner White
    ([(130, 310), (100, 390), (190, 350)], WHITE_PURE),
    # Right Cheek Inner White
    ([(382, 310), (412, 390), (322, 350)], WHITE_PURE),

    # --- Eye Sockets & Cheeks Lower ---
    # Left Sub-Eye Bridge
    ([(150, 240), (130, 310), (190, 300)], ORANGE_DEEP),
    # Right Sub-Eye Bridge
    ([(362, 240), (382, 310), (322, 300)], ORANGE_DEEP),
    # Left Eye Under-Tuft
    ([(190, 300), (130, 310), (190, 350)], SLATE_LIGHT),
    # Right Eye Under-Tuft
    ([(322, 300), (382, 310), (322, 350)], SLATE_LIGHT),

    # --- Glowing Cyan Tech Eyes ---
    # Left Eye (Sharp, Sleek, Futuristic)
    ([(162, 252), (212, 268), (178, 280)], CYAN_ACCENT),
    ([(178, 280), (212, 268), (206, 276)], CYAN_DARK),
    ([(174, 260), (194, 266), (182, 274)], CYAN_GLOW),
    # Right Eye
    ([(350, 252), (300, 268), (334, 280)], CYAN_ACCENT),
    ([(334, 280), (300, 268), (306, 276)], CYAN_DARK),
    ([(338, 260), (318, 266), (330, 274)], CYAN_GLOW),

    # --- Snout & Muzzle ---
    # Bridge of Nose (Center Ridge)
    ([(256, 290), (216, 260), (210, 350)], ORANGE_LIGHT),
    ([(256, 290), (296, 260), (302, 350)], ORANGE_MAIN),
    ([(256, 290), (210, 350), (256, 390)], ORANGE_LIGHT),
    ([(256, 290), (302, 350), (256, 390)], ORANGE_MAIN),
    # Muzzle Flanks (White Snout)
    ([(210, 350), (190, 350), (220, 420), (256, 420)], WHITE_PURE),
    ([(302, 350), (322, 350), (292, 420), (256, 420)], WHITE_SOFT),
    ([(210, 350), (256, 390), (256, 420)], WHITE_PURE),
    ([(302, 350), (256, 390), (256, 420)], WHITE_SOFT),
    # Muzzle Lower Chin (Jawline)
    ([(220, 420), (256, 420), (256, 464)], WHITE_SOFT),
    ([(292, 420), (256, 420), (256, 464)], SLATE_LIGHT),

    # --- Nose Tip (Black / Deep Obsidian Triangle) ---
    ([(256, 422), (242, 436), (270, 436)], NOSE_COLOR),
    ([(242, 436), (270, 436), (256, 448)], NOSE_COLOR),
]

def generate_svg(standalone=True):
    """Generate canonical SVG code."""
    lines = []
    if standalone:
        lines.append('<?xml version="1.0" encoding="UTF-8"?>')
        lines.append('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="100%" height="100%" shape-rendering="geometricPrecision">')
    else:
        lines.append('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" shape-rendering="geometricPrecision">')

    lines.append('  <!-- SqlFox Canonical Brand Mark - Geometric Low-Poly Fox Head -->')
    lines.append('  <g id="sqlfox-fox-mark">')
    for points, color in FACETS:
        pts_str = " ".join(f"{x},{y}" for x, y in points)
        lines.append(f'    <polygon points="{pts_str}" fill="{color}" />')
    lines.append('  </g>')
    lines.append('</svg>')
    return "\n".join(lines)

def render_raster_image(target_size):
    """Render anti-aliased image at target_size using 4x supersampling."""
    scale = 4
    canvas_size = 512 * scale
    img = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    for points, color in FACETS:
        scaled_pts = [(x * scale, y * scale) for x, y in points]
        draw.polygon(scaled_pts, fill=color)

    # Downsample with Lanczos filter for crisp edge antialiasing
    return img.resize((target_size, target_size), Image.Resampling.LANCZOS)

def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    print(f"🦊 Generating SqlFox Standard Brand Assets in: {root}")

    # 1. Generate Master SVGs
    svg_code = generate_svg(standalone=True)
    svg_paths = [
        os.path.join(root, "docs", "assets", "logo.svg"),
        os.path.join(root, "web", "dist", "assets", "logo.svg"),
        os.path.join(root, "web", "dist", "favicon.svg"),
    ]
    for p in svg_paths:
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w", encoding="utf-8") as f:
            f.write(svg_code)
        print(f"  ✅ Created {os.path.relpath(p, root)}")

    # 2. Render Hi-Res Master Image (512x512)
    master_512 = render_raster_image(512)
    master_192 = render_raster_image(192)
    apple_180 = render_raster_image(180)
    master_256 = render_raster_image(256)
    ico_48 = render_raster_image(48)
    ico_32 = render_raster_image(32)
    ico_16 = render_raster_image(16)

    # Save PNG Assets
    png_targets = [
        (os.path.join(root, "assets", "icon.png"), master_512),
        (os.path.join(root, "web", "dist", "icon-512.png"), master_512),
        (os.path.join(root, "web", "dist", "icon-192.png"), master_192),
        (os.path.join(root, "web", "dist", "apple-touch-icon.png"), apple_180),
    ]
    for path, img in png_targets:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        img.save(path, format="PNG", optimize=True)
        print(f"  ✅ Created {os.path.relpath(path, root)}")

    # 3. Save Multi-layer Windows ICO Assets
    ico_targets = [
        os.path.join(root, "web", "dist", "favicon.ico"),
        os.path.join(root, "assets", "icon.ico"),
    ]
    for p in ico_targets:
        os.makedirs(os.path.dirname(p), exist_ok=True)
        master_256.save(
            p,
            format="ICO",
            sizes=[(16, 16), (32, 32), (48, 48), (256, 256)]
        )
        print(f"  ✅ Created {os.path.relpath(p, root)}")

    # 4. Generate Web App Manifest
    manifest_path = os.path.join(root, "web", "dist", "manifest.webmanifest")
    manifest_content = """{
  "name": "SqlFox Database Manager",
  "short_name": "SqlFox",
  "description": "High-Performance Modern Database Manager with AI Copilot & Safety Guard",
  "start_url": "/",
  "display": "standalone",
  "theme_color": "#090d16",
  "background_color": "#090d16",
  "icons": [
    {
      "src": "/icon-192.png",
      "sizes": "192x192",
      "type": "image/png"
    },
    {
      "src": "/icon-512.png",
      "sizes": "512x512",
      "type": "image/png"
    },
    {
      "src": "/favicon.svg",
      "sizes": "any",
      "type": "image/svg+xml"
    }
  ]
}
"""
    with open(manifest_path, "w", encoding="utf-8") as f:
        f.write(manifest_content)
    print(f"  ✅ Created {os.path.relpath(manifest_path, root)}")

    print("🎉 All graphic icon assets successfully standardized!")

if __name__ == "__main__":
    main()
