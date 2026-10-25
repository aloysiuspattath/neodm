# Neo Download Manager PRO (NeoDM)

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3%20%2F%20Commercial-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%20%7C%2011%20(x64)-0078D6?logo=windows)](https://microsoft.com)
[![Memory Footprint](https://img.shields.io/badge/RAM-%3C35%20MB-success)]()
[![Startup](https://img.shields.io/badge/Cold%20Start-%3C0.2s-brightgreen)]()

**Neo Download Manager PRO (NeoDM)** is a next-generation, high-performance Windows download manager and CLI download accelerator. Designed to surpass legacy download managers in raw transfer speed, memory efficiency, and UI aesthetics, NeoDM consumes **under 35 MB of RAM** and cold boots in **under 200ms**.

Built in pure Go with Microsoft Edge WebView2, NeoDM is available as an interactive desktop GUI and a lightning-fast terminal CLI tool (`neodm`).

---

## 🚀 Key Features

### ⚡ 1. Dynamic Turbo Multi-Threading
- **Dynamic Re-splitting Engine**: Rather than static chunking, NeoDM dynamically splits the largest remaining unfinished segment whenever any worker finishes early. Up to 32 concurrent TCP connections remain 100% saturated until the last byte.
- **Direct Offset Writing (`WriteAt`)**: Pre-allocates target sparse files and streams bytes directly into absolute byte offsets. When 100% is reached, the file is immediately ready—zero post-download disk concatenation or freezing.
- **Robust Resumption**: Resumes interrupted, failed, or paused downloads from exact byte offsets without data corruption.

### 💻 2. Dual CLI & Desktop GUI in One Binary
- **Interactive Windows 11 GUI**: Frameless window with native Aero snap and dragging, clean Vercel Dark theme, quick 1-click download bar, real-time speed sparklines, and an IDM-style segmented connection visualizer.
- **Full CLI Mode (`neodm`)**: Download files directly from PowerShell, Windows Terminal, or CMD with an animated high-speed progress bar:
  ```powershell
  neodm https://example.com/archive.iso -o disk.iso -c 32
  ```

### 🌐 3. Browser Integration Extension (Manifest V3)
- Supports **Google Chrome**, **Microsoft Edge**, **Brave**, **Opera**, and **Firefox**.
- **Automatic Interception**: Captures browser downloads for `.zip`, `.exe`, `.iso`, `.mp4`, `.pdf`, `.tar.gz`, etc.
- **Cookie & Header Forwarding**: Seamlessly forwards browser authentication headers, session cookies, and referrers to download protected files (Google Drive, Mega, Dropbox, private intranets).
- **Floating Media Sniffer**: Injects a discreet floating *"⚡ Download with NeoDM"* badge over web audio/video players.

### 🕸️ 4. Power User Tools
- **Site Grabber**: Multi-depth webpage crawler that extracts all downloadable assets with category and regex filters (`.*\.pdf$`, `episode_\d+`).
- **Batch URL Pattern Generator**: Range expansion generator (e.g. `https://example.com/file_[01-50].zip`) to enqueue bulk sequences.
- **Download Queues & Scheduler**: Main and Night queues with customizable schedules and automatic PC shutdown on completion.
- **Token-Bucket Bandwidth Limiter**: Configurable global and per-task speed limits.
- **Clipboard Monitoring**: Watches clipboard for copied HTTP/HTTPS download links.

---

## 🖥️ Command-Line Interface (CLI)

NeoDM includes an integrated command-line tool. You can invoke it using `neodm.exe`:

```powershell
# Quick direct download
neodm https://speed.cloudflare.com/__down?bytes=104857600 -o 100mb.bin

# Advanced options (custom directory, 32 connections, speed limit)
neodm download https://example.com/archive.tar.gz -d C:\Downloads -c 32 -s 15M

# List all downloads and status
neodm list

# Check version & dual-license info
neodm version

# Display CLI help
neodm help
```

---

## 📦 Installation & Setup

### Option A: Setup Wizard (Recommended)
Run `NeoDM-Setup.exe` to deploy NeoDM to `%LOCALAPPDATA%\Programs\NeoDM`, create Desktop and Start Menu shortcuts, and register with Windows Add/Remove Programs.

### Option B: Portable Mode
Simply run `neodm.exe` from any directory. NeoDM creates its portable data store in `%APPDATA%\NeoDM`.

### Option C: Browser Extension
1. Open your Chromium browser (`chrome://extensions` or `edge://extensions`).
2. Enable **Developer Mode** (top-right).
3. Click **Load unpacked** and select the `extension/` directory.

---

## 🎨 Design System & Theming

NeoDM features a minimalist, de-congested design engineered for everyday users:
- **▲ Vercel Dark (Default)**: Pure black canvas (`#000000`), `#080808` sidebar, `#222222` subtle borders, high-contrast crisp action buttons, and Vercel blue accents.
- **🌌 Nova Dark PRO**: Deep space navy theme with fluent elevation.
- **🔮 Bento Glass**: Translucent glassmorphism with dynamic backdrop filters.
- **🧛 Dracula Pro**: Official Dracula palette.
- **❄️ Nordic Frost**: Clean cold Arctic theme.
- **⚡ Cyberpunk Neon**: High-contrast electric yellow and neon cyan.
- **🖤 Midnight OLED**: Ultra-deep pitch black for OLED panels.
- **☀️ Fluent Light**: Windows 11 Fluent light theme.

---

## 🏗️ Architecture & Tech Stack

```
neodm/
├── main.go                       # Dual CLI/GUI entrypoint & Edge WebView2 host
├── backend/
│   ├── cli/                      # Interactive terminal CLI mode & progress bars
│   ├── engine/                   # Multi-connection chunk scheduler, storage, queue
│   │   ├── downloader.go         # Task state machine & worker coordinator
│   │   ├── segment.go            # Dynamic segment re-splitting algorithm
│   │   ├── storage.go            # Direct offset writer (WriteAt)
│   │   ├── queue.go              # Main/Night queue manager & scheduler
│   │   ├── limiter.go            # Token-bucket rate limiter
│   │   └── store.go              # ACID persistent state store
│   ├── grabber/                  # Site Grabber crawler & pattern expander
│   ├── platform/                 # Windows Win32 API, frameless shell, toasts, shortcuts
│   └── server/                   # Local REST API & WebSocket telemetry hub
├── frontend/                     # Modern Web UI (HTML, CSS, JS, Canvas)
└── extension/                    # Manifest V3 browser companion extension
```

---

## ⚖️ Dual License

Neo Download Manager PRO (NeoDM) is dual-licensed:

1. **Open-Source Community**: Free software licensed under the **GNU General Public License v3.0 (GPLv3)**.
2. **Commercial / OEM License**: For proprietary commercial software redistribution without GPLv3 copyleft restrictions, contact commercial licensing.

See [LICENSE](LICENSE) for full legal terms.
