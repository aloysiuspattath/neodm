# 🌐 NeoDM Product Showcase & Cloudflare Worker

This directory contains the production-ready product showcase landing page and edge worker for **Neo Download Manager PRO (NeoDM)**, designed for deployment on **Cloudflare Workers with Static Assets**.

---

## 🏗️ Architecture

- **Static Assets (`public/`)**: Bento Dark/Light responsive product showcase, comparison benchmarks, feature tours, and local application screenshots.
- **Edge Worker (`src/index.ts`)**: Serverless edge handler providing:
  - `GET /download` or `GET /download/installer`: Dynamically resolves and 302 redirects to the latest `NeoDM-PRO-Setup.exe` on GitHub.
  - `GET /download/portable`: Dynamically resolves and 302 redirects to the latest `.zip` release.
  - `GET /api/release`: Edge-cached JSON endpoint providing version tags, asset sizes, and release telemetry.
  - Fallback to `./public` static assets with enhanced security headers (`X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`).

---

## 🚀 Quick Start

### 1. Install Dependencies
```powershell
npm install
```

### 2. Local Development & Edge Simulation
```powershell
npm run dev
# or: npx wrangler dev
```
Open your browser at `http://localhost:8787`.

### 3. Deploy to Cloudflare
```powershell
npm run deploy
# or: npx wrangler deploy
```

---

## ⚙️ Configuration (`wrangler.jsonc`)

To customize the GitHub repository or domain:
```jsonc
{
  "name": "neodm-showcase",
  "main": "src/index.ts",
  "compatibility_date": "2025-03-07",
  "compatibility_flags": ["nodejs_compat"],
  "assets": {
    "directory": "./public",
    "binding": "ASSETS",
    "not_found_handling": "single-page-application"
  },
  "vars": {
    "GITHUB_REPO": "aloysiuspattath/neodm"
  }
}
```
