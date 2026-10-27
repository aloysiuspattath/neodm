// NeoDM Chrome / Edge / Brave / Firefox Companion Extension (Manifest V3)

const DEFAULT_PORT = 8998;
const FALLBACK_PORT = 19765;
let activeApiBase = `http://127.0.0.1:${DEFAULT_PORT}`;

// Dynamically ping API to verify active NeoDM port
async function getApiBase() {
  try {
    const res = await fetch(`http://127.0.0.1:${DEFAULT_PORT}/api/settings`, { signal: AbortSignal.timeout(1000) });
    if (res.ok) {
      activeApiBase = `http://127.0.0.1:${DEFAULT_PORT}`;
      return activeApiBase;
    }
  } catch (e) {
    // Try fallback port
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

const DEFAULT_EXTENSIONS = [
  'zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'iso', 'dmg',
  'exe', 'msi', 'bin', 'apk',
  'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx',
  'mp4', 'mkv', 'avi', 'mov', 'webm', 'flv',
  'mp3', 'flac', 'wav', 'aac', 'ogg', 'm4a'
];

// Initialize context menus and settings on installation
chrome.runtime.onInstalled.addListener(() => {
  setupContextMenus();
  chrome.storage.local.get(['autoIntercept', 'extensions'], (result) => {
    if (result.autoIntercept === undefined) {
      chrome.storage.local.set({ autoIntercept: true });
    }
    if (!result.extensions) {
      chrome.storage.local.set({ extensions: DEFAULT_EXTENSIONS });
    }
  });
});

function setupContextMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({
      id: 'neodm-download-link',
      title: 'Download with NeoDM',
      contexts: ['link']
    });

    chrome.contextMenus.create({
      id: 'neodm-download-media',
      title: 'Download Media with NeoDM',
      contexts: ['video', 'audio', 'image', 'page', 'frame']
    });

    chrome.contextMenus.create({
      id: 'neodm-download-page',
      title: 'Download All Links with NeoDM (Site Grabber)',
      contexts: ['page']
    });
  });
}

// Handle Context Menu Clicks
chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId === 'neodm-download-link' || info.menuItemId === 'ifd-download-link') {
    if (info.linkUrl) {
      await sendToNeoDM(info.linkUrl, tab);
    }
  } else if (info.menuItemId === 'neodm-download-media' || info.menuItemId === 'ifd-download-media') {
    let targetUrl = info.srcUrl;
    // On video platforms (YouTube, Vimeo, etc.), or if right-clicked on an overlay/page/frame,
    // or if srcUrl is missing / a blob: or data: URL, fallback to tab.url
    if (!targetUrl || targetUrl.startsWith('blob:') || targetUrl.startsWith('data:')) {
      if (tab && tab.url && (tab.url.startsWith('http://') || tab.url.startsWith('https://'))) {
        targetUrl = tab.url;
      }
    }
    if (targetUrl) {
      const suggestedName = (tab && tab.title) ? tab.title : '';
      await sendToNeoDM(targetUrl, tab, suggestedName);
    }
  } else if (info.menuItemId === 'neodm-download-page' || info.menuItemId === 'ifd-download-page') {
    if (tab && tab.url) {
      await crawlWithGrabber(tab.url);
    }
  }
});

// Automatic Browser Download Interception
chrome.downloads.onCreated.addListener(async (downloadItem) => {
  const settings = await chrome.storage.local.get(['autoIntercept', 'extensions']);
  if (settings.autoIntercept === false) return;

  const url = downloadItem.finalUrl || downloadItem.url;
  if (!url || url.startsWith('blob:') || url.startsWith('data:')) return;

  const exts = settings.extensions || DEFAULT_EXTENSIONS;
  const isMatch = matchesExtension(url, exts) || matchesExtension(downloadItem.filename, exts);

  if (isMatch) {
    // 1. Cancel browser's standard download
    try {
      await chrome.downloads.cancel(downloadItem.id);
      await chrome.downloads.erase({ id: downloadItem.id });
    } catch (e) {
      // Ignore if already completed/cancelled
    }

    // 2. Query active tab for cookies & referer
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    await sendToNeoDM(url, tab, downloadItem.filename);
  }
});

function matchesExtension(targetPath, exts) {
  if (!targetPath) return false;
  try {
    const clean = targetPath.split('?')[0].split('#')[0].toLowerCase();
    return exts.some(ext => clean.endsWith('.' + ext.toLowerCase()));
  } catch (e) {
    return false;
  }
}

// Send Download to NeoDM Engine via Loopback REST API
async function sendToNeoDM(targetUrl, tab, suggestedFilename) {
  let cookieHeader = '';
  let refererHeader = '';

  if (tab && tab.url) {
    refererHeader = tab.url;
    try {
      const cookies = await chrome.cookies.getAll({ url: targetUrl });
      cookieHeader = cookies.map(c => `${c.name}=${c.value}`).join('; ');
    } catch (e) {
      // Best-effort cookie retrieval
    }
  }

  const payload = {
    url: targetUrl,
    filename: suggestedFilename || '',
    cookies: cookieHeader,
    headers: {
      'Referer': refererHeader,
      'User-Agent': navigator.userAgent
    }
  };

  const apiBase = await getApiBase();
  try {
    const res = await fetch(`${apiBase}/api/download/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (data.success) {
      showBadge('✓', '#10b981');
    }
  } catch (err) {
    console.warn('NeoDM Engine is offline or not responding:', err);
    showBadge('!', '#ef4444');
  }
}

// Trigger Site Grabber Crawl
async function crawlWithGrabber(pageUrl) {
  const apiBase = await getApiBase();
  try {
    await fetch(`${apiBase}/api/grabber/crawl`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: pageUrl, depth: 1 })
    });
  } catch (err) {
    console.warn('NeoDM Grabber request failed:', err);
  }
}

function showBadge(text, color) {
  chrome.action.setBadgeText({ text });
  chrome.action.setBadgeBackgroundColor({ color });
  setTimeout(() => {
    chrome.action.setBadgeText({ text: '' });
  }, 2500);
}

// Listen for messages from content scripts (e.g. video stream sniffer)
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.type === 'SNIFFED_MEDIA') {
    sendToNeoDM(message.url, sender.tab, message.title);
    sendResponse({ success: true });
  } else if (message.type === 'CHECK_ENGINE') {
    getApiBase().then(apiBase => {
      fetch(`${apiBase}/api/settings`)
        .then(res => res.json())
        .then(data => sendResponse({ online: true, data, apiBase }))
        .catch(() => sendResponse({ online: false }));
    });
    return true; // Keep channel open for async response
  }
});
