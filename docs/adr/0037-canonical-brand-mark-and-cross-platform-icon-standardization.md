# 0037: Canonical Brand Mark & Cross-Platform Icon Standardization

## Status
Accepted

## Context
SqlFox previously utilized an ad-hoc emoji (`🦊`) across its user interfaces (navbar header, native dialogs, lock screens, and documentation headers), while lacking official icon assets for web browsers and OS environments:
1. **Missing Web Favicons**: The web frontend lacked standard favicon links in `<head>`, causing browsers to fire redundant 404 or SPA fallback requests for `/favicon.ico` on every session load.
2. **Missing Mobile/Tablet Web Manifest**: SqlFox features LAN/remote access capabilities, but without a Web App Manifest (`manifest.webmanifest`) and `apple-touch-icon.png`, bookmarking or adding to home screen on tablets produced generic screenshot thumbnails instead of a standalone app experience.
3. **Missing Windows Executable Icon**: Standalone Windows executables (`sqlfox.exe`) rendered with Windows' default blank application icon in File Explorer and taskbar due to the lack of an embedded COFF resource (`.syso`).
4. **Visual Inconsistency**: Emojis render differently across operating systems (Apple, Microsoft, Google/Noto, Linux font variations) and lack the crisp technical branding required for an enterprise-grade database management tool.

## Decisions

### 1. Vector Master Brand Mark
A canonical, modern geometric tech fox head emblem was designed and recorded as the master vector (`docs/assets/logo.svg` and `web/dist/assets/logo.svg`):
- **Color & Style**: Solid vibrant brand orange (`#ea580c`) featuring sleek minimalist geometric contours, replacing legacy multi-faceted polygons for instant visual recognition at any scale.
- **Precision Scaling**: Centered vector paths within a `512x512` viewBox, guaranteeing razor-sharp legibility at micro scales (16x16 px) and high resolutions (512x512 px).

### 2. Multi-Platform Icon Set
An automated, reproducible generator (`scripts/generate-icons.py`) derives all required cross-platform icon artifacts directly from the master facet definitions using 4x supersampling anti-aliasing:
- **Web & Browser**: `favicon.svg` (crisp scalable SVG favicon) and multi-resolution `favicon.ico` (16x16, 32x32, 48x48, 256x256).
- **Mobile & Tablet**: `apple-touch-icon.png` (180x180) and `manifest.webmanifest` configured with `display: "standalone"`, `theme_color: "#090d16"`, and PNG icon references (`icon-192.png`, `icon-512.png`).
- **Desktop Distribution**: `assets/icon.png` and `assets/icon.ico` bundled into release archives (`.tar.gz` and `.zip`) in `.github/workflows/release.yml`.

### 3. Native Windows PE Embedding (`cmd/sqlfox/rsrc_windows_*.syso`)
To maintain zero external build-time dependencies during CI/CD compilation:
- Multi-architecture COFF resource objects (`rsrc_windows_amd64.syso` and `rsrc_windows_arm64.syso`) are pre-compiled and committed to `cmd/sqlfox/`.
- The Go compiler automatically detects and links these object files into `sqlfox.exe` whenever `GOOS=windows` is targeted, embedding the official icon resource natively into the PE executable header without requiring external resource compilers during GitHub Actions release builds.

### 4. In-App Titlebar & Dialog Integration
- `web/dist/index.html` embeds the master Brand Mark as an SVG `<symbol id="sqlfox-brand-mark">`, referenced via `<use href="#sqlfox-brand-mark"/>` across the top navbar header, native `<dialog id="app-native-dialog">`, and remote lock screen modal.
- `README.md` replaces ad-hoc emoji headings with a centered, high-resolution vector logo hero banner.

## Consequences
- Every platform (Web, Windows Explorer, macOS/Linux desktop archives, iOS/Android home screens, GitHub) presents a unified, professional, and crisp visual identity.
- Eliminates 404 network requests for `/favicon.ico` on browser launch.
- Adheres strictly to Ponytail principles: zero runtime dependencies, reproducible offline generator script, and zero build-time friction in CI/CD.
