# Neo Download Manager PRO (NeoDM)

[![GitHub Release](https://img.shields.io/badge/Release-v1.1.0-blue.svg?logo=github)](https://github.com/aloysiuspattath/neodm/releases)
[![Build & Tests](https://img.shields.io/badge/Tests-29%20Passing%20(100%25)-success?logo=go)](https://golang.org)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3%20%2F%20Commercial-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%20%7C%2011%20(x64)-0078D6?logo=windows)](https://microsoft.com)
[![Memory Footprint](https://img.shields.io/badge/RAM-%3C35%20MB-success)]()
[![Startup](https://img.shields.io/badge/Cold%20Start-%3C0.2s-brightgreen)]()

> **Neo Download Manager PRO (NeoDM)** is a next-generation, high-performance Windows download manager, multimedia sniffer, and download accelerator. Engineered in pure Go with Microsoft Edge WebView2, NeoDM delivers 5x–10x download acceleration, zero-concatenation sparse file storage, automated browser extension registration, and an ultra-clean **Bento UI** with light and dark themes.

---

## ⚡ Comparison Matrix: NeoDM vs. Legacy Download Managers

| Feature | **NeoDM PRO** | **Internet Download Manager (IDM)** | **Free Download Manager (FDM)** | **XDM** |
| :--- | :---: | :---: | :---: | :---: |
| **Max Concurrent Connections** | **Up to 32 Dynamic** | Up to 32 Dynamic | Up to 10 Static | Up to 32 |
| **Direct Offset Writing (`WriteAt`)** | **Yes (0s disk freeze)** | No (Disk concatenation delay) | Partial | No |
| **Cold Boot Time** | **< 200 ms** | ~ 800 ms | ~ 2.5 s | ~ 4.0 s (Java) |
| **Memory Footprint** | **< 35 MB RAM** | ~ 45 MB RAM | ~ 150 MB RAM | ~ 280 MB RAM |
| **Modern Bento UI (Light / Dark)** | **Yes (Bento 2.0)** | No (90s Win32 GUI) | Standard Web | Swing/Java |
| **YouTube Video Grabber (Muxed Audio+Video)** | **Yes (Zero Terminals)** | Yes | Limited | Fragile |
| **Dual CLI & Desktop GUI** | **Yes (`neodm` CLI)** | No | No | No |
| **Browser Extension Support** | **Chrome, Edge, Brave, Firefox** | Chrome, Edge, Firefox | Chrome, Edge | Chrome, Edge |
| **Open Source / Auditable** | **Yes (Pure Go)** | No (Proprietary) | Open Source | Open Source |

---

## 🚀 Key Features

### ⚡ 1. Dynamic Turbo Multi-Threading
- **Dynamic Segment Subdivision**: Unlike legacy static chunk downloaders, NeoDM dynamically splits the largest remaining byte range whenever a worker finishes early, ensuring all 32 TCP connections remain saturated until the final byte.
- **Direct Offset Writing (`WriteAt`)**: Pre-allocates target sparse files and streams incoming bytes directly to their exact disk offsets. Eliminates post-download file assembling, disk freezing, and high I/O pauses.
- **Fault-Tolerant Resumption**: Automatically resumes interrupted or paused downloads from exact byte boundaries.
- **Configurable Temporary Directory**: Direct scratch parts to high-capacity secondary drives (e.g. `D:\Scratch`) to safeguard system drives with low disk space.

### 🎥 2. Silent YouTube & Video Grabber
- **Full Video + Audio Muxing**: Downloads 1080p (Full HD), 720p, 480p, 360p, or Audio-Only (MP3/M4A) with video and audio merged into a pristine MP4 container via `ffmpeg`.
- **Zero Terminal Windows**: All video analysis and downloading executes in silent background mode (`HideWindow: true`)—no flashing command prompt popups.
- **Real-Time Telemetry**: Real-time speed metrics, ETA countdown, and progress bar directly within the Bento interface.

### 🌐 3. Browser Companion Extension (Manifest V3)
- Supports **Google Chrome**, **Microsoft Edge**, **Brave**, **Opera**, and **Mozilla Firefox**.
- **1-Click Auto-Registration**: Installer and in-app settings automatically configure Windows Registry keys for Chrome, Edge, and Firefox.
- **Automatic Interception**: Intercepts downloadable links (`.iso`, `.zip`, `.exe`, `.tar.gz`, `.mp4`, `.pdf`).
- **Authentication & Cookie Forwarding**: Forwards session cookies and authorization headers to download from private portals, Google Drive, Mega, and internal servers.
- **Dynamic In-App Path Detection**: 1-click button to copy extension path or reveal folder directly in Windows Explorer.

### 🍱 4. Industry-Standard Bento UI (Light & Dark)
- **🌙 Bento Dark (Graphite Slate)**: Clean deep slate palette with subtle border elevation and ergonomic typography.
- **☀️ Bento Light (Cloud Slate)**: High-usability light theme with crisp white card panels, soft borders, and zero eye glare.
- **🌌 Midnight Obsidian**: Ultra-deep pitch black theme optimized for OLED monitors.
- **🌲 Forest Emerald**: Sophisticated slate green theme for distraction-free focus.
- **Frameless Window with Dark DWM Aero Snap**: Native Windows 11 title bar integration and window controls.

### 🗂️ 5. Power User Tools
- **Site Grabber**: Multi-depth recursive webpage crawler with file extension and regex filters (`.*\.pdf$`, `chapter_\d+`).
- **Batch URL Pattern Generator**: Sequential pattern expander (e.g. `https://cdn.example.com/data_[01-50].zip`) to enqueue bulk files in seconds.
- **Queue Scheduler & Power Management**: Main and Night download queues with automated start/stop times and automatic computer shutdown upon completion.
- **Token-Bucket Bandwidth Limiter**: Global and per-download speed throttles.
- **One-Click Folder Revealer**: Opens Windows Explorer with the downloaded file pre-selected and highlighted.

---

## 💻 Command-Line Interface (CLI)

The exact same binary (`neodm.exe` / `ifd.exe`) functions as a complete command-line download tool:

```powershell
# Rapid direct download
neodm https://speed.cloudflare.com/__down?bytes=104857600 -o 100mb.bin

# Advanced download (32 connections, 10MB/s speed limit, custom folder)
neodm download https://example.com/ubuntu-24.04.iso -d D:\ISOs -c 32 -s 10M

# Inspect active downloads
neodm list

# Check engine version
neodm version
```

---

## 📦 Installation & Setup

### Option A: Setup Wizard (Recommended)
Download and run **`NeoDM-PRO-Setup.exe`**:
1. Deploys NeoDM to `%LOCALAPPDATA%\Programs\NeoDM`.
2. Creates Desktop and Start Menu shortcuts.
3. Automatically registers the browser extension for Chrome, Edge, Brave, and Firefox.
4. Registers `uninstall.exe` in Windows **Add or Remove Programs**.

### Option B: Portable Mode
Download **`NeoDM-PRO-windows-x64.zip`**, extract to any directory or USB drive, and run `neodm.exe`. Settings and downloads are stored portably.

### Option C: Browser Extension Setup
1. In NeoDM, click **Tools** → **Browser Integration**.
2. Click **Open Folder in Explorer** or **Copy Path**.
3. In Chrome or Edge, navigate to `chrome://extensions` (or `edge://extensions`), enable **Developer mode**, click **Load unpacked**, and select the extension folder!

---

## 🧹 Clean Uninstallation

NeoDM provides a 100% clean, native uninstallation workflow:
- Open **Windows Settings** → **Apps** → **Installed Apps** → **Neo Download Manager PRO** → **Uninstall**.
- Or run `uninstall.exe` directly from the install directory.
- Prompts for confirmation, unregisters browser extensions, clears Windows registry entries, deletes shortcuts, and completely cleans the installation folder.

---

## 🏗️ Architecture & Project Layout

```
neodm/
├── main.go                       # Entrypoint (CLI / GUI / Uninstaller) & Edge WebView2 host
├── backend/
│   ├── cli/                      # Interactive terminal CLI mode & progress bars
│   ├── engine/                   # Multi-connection chunk scheduler, storage, queue
│   │   ├── downloader.go         # Task state machine, HTTP worker & silent yt-dlp coordinator
│   │   ├── segment.go            # Dynamic segment re-splitting algorithm
│   │   ├── storage.go            # Direct offset writer (WriteAt)
│   │   ├── queue.go              # Main & Night queue managers & schedulers
│   │   ├── limiter.go            # Token-bucket rate limiter
│   │   └── store.go              # ACID persistent state store
│   ├── grabber/                  # Site Grabber, pattern generator, video stream analyzer
│   │   ├── video.go              # Silent YouTube/HTML5 media sniffer (HideWindow: true)
│   │   └── crawler.go            # Multi-depth webpage asset spider
│   ├── platform/                 # Win32 DWM dark mode, native message boxes, shortcuts, tray
│   │   ├── installer.go          # InstallApp, UninstallApp, Browser Registry Auto-Registration
│   │   ├── window_windows.go     # DWM dark titlebar & Aero snap
│   │   └── tray_windows.go       # System tray dock & balloon notifications
│   └── server/                   # Local REST API & WebSocket telemetry hub
├── frontend/                     # Bento UI (HTML, CSS, JS, Canvas Speed Sparklines)
├── extension/                    # Manifest V3 browser companion extension
└── installer/                    # Standalone setup wizard compiler
```

---

## ⚖️ License

Neo Download Manager PRO is dual-licensed:

1. **Open-Source Community**: Free software under the **GNU General Public License v3.0 (GPLv3)**.
2. **Commercial / OEM License**: For enterprise distribution without GPLv3 copyleft restrictions, contact the project maintainers.

See [LICENSE](LICENSE) for full details.
