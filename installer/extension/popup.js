// NeoDM Extension Popup Script

const DEFAULT_PORT = 8998;
const FALLBACK_PORT = 19765;
let activeApiBase = `http://127.0.0.1:${DEFAULT_PORT}`;

async function getApiBase() {
  try {
    const res = await fetch(`http://127.0.0.1:${DEFAULT_PORT}/api/settings`, { signal: AbortSignal.timeout(1000) });
    if (res.ok) {
      activeApiBase = `http://127.0.0.1:${DEFAULT_PORT}`;
      return activeApiBase;
    }
  } catch (e) {
    try {
      const resFallback = await fetch(`http://127.0.0.1:${FALLBACK_PORT}/api/settings`, { signal: AbortSignal.timeout(1000) });
      if (resFallback.ok) {
        activeApiBase = `http://127.0.0.1:${FALLBACK_PORT}`;
        return activeApiBase;
      }
    } catch (e2) {}
  }
  return activeApiBase;
}

document.addEventListener('DOMContentLoaded', async () => {
  const toggleAuto = document.getElementById('toggle-auto');
  const btnDownload = document.getElementById('btn-quick-download');
  const inputUrl = document.getElementById('quick-url-input');
  const btnGrab = document.getElementById('btn-grab-page');
  const btnOpen = document.getElementById('btn-open-app');

  // Load autoIntercept setting
  chrome.storage.local.get(['autoIntercept'], (res) => {
    toggleAuto.checked = res.autoIntercept !== false;
  });

  toggleAuto.addEventListener('change', () => {
    chrome.storage.local.set({ autoIntercept: toggleAuto.checked });
  });

  // Check engine health
  checkEngineStatus();

  // Quick download
  btnDownload.addEventListener('click', async () => {
    const url = inputUrl.value.trim();
    if (!url) return;

    const apiBase = await getApiBase();
    try {
      const res = await fetch(`${apiBase}/api/download/add`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url })
      });
      const data = await res.json();
      if (data.success) {
        inputUrl.value = '';
        btnDownload.textContent = '✓ Added';
        setTimeout(() => btnDownload.textContent = 'Download', 1500);
      }
    } catch (e) {
      alert('NeoDM Engine is not running. Please start the NeoDM application.');
    }
  });

  // Grab Page links
  btnGrab.addEventListener('click', async () => {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab && tab.url) {
      const apiBase = await getApiBase();
      try {
        await fetch(`${apiBase}/api/grabber/crawl`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: tab.url, depth: 1 })
        });
        window.open(`${apiBase}/index.html`);
      } catch (e) {
        alert('Could not connect to NeoDM Engine.');
      }
    }
  });

  // Open NeoDM App UI
  btnOpen.addEventListener('click', async () => {
    const apiBase = await getApiBase();
    window.open(`${apiBase}/index.html`);
  });
});

async function checkEngineStatus() {
  const badge = document.getElementById('engine-status');
  const text = document.getElementById('status-text');

  const apiBase = await getApiBase();
  try {
    const res = await fetch(`${apiBase}/api/settings`, { signal: AbortSignal.timeout(1500) });
    if (res.ok) {
      badge.className = 'status-badge status-online';
      text.textContent = 'NeoDM Online';
    } else {
      throw new Error();
    }
  } catch (e) {
    badge.className = 'status-badge status-offline';
    text.textContent = 'NeoDM Offline';
  }
}
