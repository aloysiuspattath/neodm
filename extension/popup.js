// IFD Extension Popup Script

const IFD_API_BASE = 'http://127.0.0.1:19765';

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

    try {
      const res = await fetch(`${IFD_API_BASE}/api/download/add`, {
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
      alert('IFD Engine is not running. Please start the IFD app.');
    }
  });

  // Grab Page links
  btnGrab.addEventListener('click', async () => {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab && tab.url) {
      try {
        await fetch(`${IFD_API_BASE}/api/grabber/crawl`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: tab.url, depth: 1 })
        });
        window.open(`${IFD_API_BASE}/index.html`);
      } catch (e) {
        alert('Could not connect to IFD Engine.');
      }
    }
  });

  // Open IFD App UI
  btnOpen.addEventListener('click', () => {
    window.open(`${IFD_API_BASE}/index.html`);
  });
});

async function checkEngineStatus() {
  const badge = document.getElementById('engine-status');
  const text = document.getElementById('status-text');

  try {
    const res = await fetch(`${IFD_API_BASE}/api/settings`);
    if (res.ok) {
      badge.className = 'status-badge status-online';
      text.textContent = 'Engine Active';
    } else {
      throw new Error();
    }
  } catch (e) {
    badge.className = 'status-badge status-offline';
    text.textContent = 'Engine Offline';
  }
}
