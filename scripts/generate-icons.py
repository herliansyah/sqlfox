#!/usr/bin/env python3
"""
SqlFox Brand Mark & Standard Icon Generator
Generates canonical multi-platform icon assets from Modern Geometric Tech Fox Head (GitLab style):
- web/dist/assets/logo.svg & docs/assets/logo.svg (Master Vector)
- web/dist/favicon.svg (Optimized browser favicon)
- web/dist/favicon.ico (Multi-res 16, 32, 48, 256 Windows/Browser ICO)
- web/dist/apple-touch-icon.png (180x180 iOS/iPad bookmark)
- web/dist/icon-192.png & web/dist/icon-512.png (PWA Manifest)
- web/dist/manifest.webmanifest (PWA configuration)
- web/dist/index.html (Inline SVG Brand Symbol)
- assets/icon.ico & assets/icon.png (Desktop package archives)
"""

import os
import sys
from PIL import Image

# Canonical Brand Colors
ORANGE_BRAND = "#ea580c"  # SqlFox Deep Vibrant Orange

# Vector SVG Path for the Modern Geometric Tech Fox Head with Eyes
FOX_SVG_PATH = (
    "M 89,36 L 203,130 L 254,117 L 308,130 L 420,36 L 423,37 L 440,112 L 421,223 "
    "L 460,307 L 459,311 L 399,371 L 351,387 L 294,463 L 258,476 L 218,463 L 161,388 "
    "L 113,371 L 51,309 L 90,223 L 71,111 L 89,37 Z "
    "M 445,311 L 341,329 L 285,412 L 283,426 L 262,443 L 288,452 L 292,452 L 296,441 "
    "L 335,378 L 445,316 L 450,312 L 446,311 Z "
    "M 63,312 L 177,378 L 219,452 L 249,443 L 228,426 L 225,411 L 170,329 L 64,312 Z "
    "M 160,260 L 175,290 L 215,302 L 200,272 Z "
    "M 352,260 L 337,290 L 297,302 L 312,272 Z"
)

def generate_svg(standalone=True):
    """Generate canonical SVG code."""
    lines = []
    if standalone:
        lines.append('<?xml version="1.0" encoding="UTF-8"?>')
        lines.append('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="100%" height="100%" shape-rendering="geometricPrecision">')
    else:
        lines.append('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" shape-rendering="geometricPrecision">')

    lines.append('  <!-- SqlFox Brand Mark - Modern Geometric Tech Fox Head (Option 1) -->')
    lines.append(f'  <path id="sqlfox-fox-mark" fill="{ORANGE_BRAND}" fill-rule="evenodd" d="{FOX_SVG_PATH}" />')
    lines.append('</svg>')
    return "\n".join(lines)

def load_master_square(root):
    """Load canonical master 512x512 square icon."""
    canonical_master = os.path.join(root, "assets", "icon.png")
    if os.path.exists(canonical_master):
        return Image.open(canonical_master).convert("RGBA")
    raise FileNotFoundError(f"Missing canonical master icon: {canonical_master}")

def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    print(f"🦊 Generating SqlFox Modern Geometric Fox Brand Assets in: {root}")

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

    # 2. Render Hi-Res Master Image & Downsamples
    master_512 = load_master_square(root)
    master_192 = master_512.resize((192, 192), Image.Resampling.LANCZOS)
    apple_180  = master_512.resize((180, 180), Image.Resampling.LANCZOS)
    master_256 = master_512.resize((256, 256), Image.Resampling.LANCZOS)

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

    # 3b. Recompile Native Windows PE Resource Objects (.syso)
    import subprocess
    ico_asset = os.path.join(root, "assets", "icon.ico")
    for arch in ["amd64", "arm64"]:
        syso_target = os.path.join(root, "cmd", "sqlfox", f"rsrc_windows_{arch}.syso")
        try:
            subprocess.run(
                ["go", "run", "github.com/akavel/rsrc@latest", "-ico", ico_asset, "-arch", arch, "-o", syso_target],
                check=True,
                cwd=root,
                capture_output=True,
            )
            print(f"  ✅ Compiled Windows PE resource: {os.path.relpath(syso_target, root)}")
        except Exception as e:
            print(f"  ⚠️ Warning: Could not compile Windows {arch} .syso resource: {e}")

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

    # 5. Synchronize Inline SVG Symbol in web/dist/index.html
    index_html_path = os.path.join(root, "web", "dist", "index.html")
    if os.path.exists(index_html_path):
        import re
        with open(index_html_path, "r", encoding="utf-8") as f:
            content = f.read()

        symbol_pattern = r'<symbol id="sqlfox-brand-mark"[^>]*>[\s\S]*?</symbol>'
        new_symbol = (
            f'<symbol id="sqlfox-brand-mark" viewBox="0 0 512 512" shape-rendering="geometricPrecision">\n'
            f'      <path id="sqlfox-fox-mark" fill="{ORANGE_BRAND}" fill-rule="evenodd" d="{FOX_SVG_PATH}" />\n'
            f'    </symbol>'
        )
        updated_content = re.sub(symbol_pattern, new_symbol, content)
        with open(index_html_path, "w", encoding="utf-8") as f:
            f.write(updated_content)
        print(f"  ✅ Updated inline brand mark symbol in {os.path.relpath(index_html_path, root)}")

    print("🎉 All graphic icon assets successfully replaced with Option 1 (Modern Geometric Fox)!")

if __name__ == "__main__":
    main()
