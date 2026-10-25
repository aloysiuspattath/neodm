// IFD Chrome / Firefox Browser Integration Service Worker (Manifest V3)

const IFD_API_BASE = 'http://127.0.0.1:19765';

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
      id: 'ifd-download-link',
      title: 'Download with IFD',
      contexts: ['link']
    });

    chrome.contextMenus.create({
      id: 'ifd-download-media',
      title: 'Download Media with IFD',
      contexts: ['image', 'video', 'audio']
    });

    chrome.contextMenus.create({
      id: 'ifd-download-page',
      title: 'Download All Links with IFD (Site Grabber)',
      contexts: ['page']
    });
  });
}

// Handle Context Menu Clicks
chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId === 'ifd-download-link') {
    if (info.linkUrl) {
      await sendToIFD(info.linkUrl, tab);
    }
  } else if (info.menuItemId === 'ifd-download-media') {
    if (info.srcUrl) {
      await sendToIFD(info.srcUrl, tab);
    }
  } else if (info.menuItemId === 'ifd-download-page') {
    if (tab && tab.url) {
      // Trigger Site Grabber on the active page
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
    await sendToIFD(url, tab, downloadItem.filename);
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

// Send Download to IFD Engine via Loopback REST API
async function sendToIFD(targetUrl, tab, suggestedFilename) {
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

  try {
    const res = await fetch(`${IFD_API_BASE}/api/download/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (data.success) {
      showBadge('✓', '#10b981');
    }
  } catch (err) {
    console.warn('IFD Engine is offline or not responding:', err);
    showBadge('!', '#ef4444');
  }
}

// Trigger Site Grabber Crawl
async function crawlWithGrabber(pageUrl) {
  try {
    await fetch(`${IFD_API_BASE}/api/grabber/crawl`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: pageUrl, depth: 1 })
    });
  } catch (err) {
    console.warn('IFD Grabber request failed:', err);
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
    sendToIFD(message.url, sender.tab, message.title);
    sendResponse({ success: true });
  } else if (message.type === 'CHECK_ENGINE') {
    fetch(`${IFD_API_BASE}/api/settings`)
      .then(res => res.json())
      .then(data => sendResponse({ online: true, data }))
      .catch(() => sendResponse({ online: false }));
    return true; // Keep channel open for async response
  }
});
