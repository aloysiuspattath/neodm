import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import puppeteer from 'puppeteer-core';

const FRONTEND_DIR = 'd:\\new projects\\ifd\\frontend';
const CHROME_PATH = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';

const MIME_TYPES = {
  '.html': 'text/html',
  '.css': 'text/css',
  '.js': 'application/javascript',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.ico': 'image/x-icon',
  '.svg': 'image/svg+xml'
};

const MOCK_DOWNLOADS = [
  {
    id: 'task-1',
    filename: 'ubuntu-24.04-desktop-amd64.iso',
    status: 'Downloading',
    percent: 74,
    downloadedBytes: 3712000000,
    totalSize: 5013000000,
    speed: 48234000,
    eta: 27,
    timeLeft: '27s',
    category: 'Programs',
    url: 'https://releases.ubuntu.com/24.04/ubuntu-24.04-desktop-amd64.iso',
    savePath: 'D:\\Downloads\\Programs',
    queueId: 'main_queue',
    maxConns: 16,
    supportsRange: true,
    segments: [
      { id: 1, startOffset: 0, currentOffset: 313312500, endOffset: 313312500, done: true, active: false },
      { id: 2, startOffset: 313312501, currentOffset: 626625000, endOffset: 626625000, done: true, active: false },
      { id: 3, startOffset: 626625001, currentOffset: 939937500, endOffset: 939937500, done: true, active: false },
      { id: 4, startOffset: 939937501, currentOffset: 1253250000, endOffset: 1253250000, done: true, active: false },
      { id: 5, startOffset: 1253250001, currentOffset: 1566562500, endOffset: 1566562500, done: true, active: false },
      { id: 6, startOffset: 1566562501, currentOffset: 1879875000, endOffset: 1879875000, done: true, active: false },
      { id: 7, startOffset: 1879875001, currentOffset: 2193187500, endOffset: 2193187500, done: true, active: false },
      { id: 8, startOffset: 2193187501, currentOffset: 2506500000, endOffset: 2506500000, done: true, active: false },
      { id: 9, startOffset: 2506500001, currentOffset: 2819812500, endOffset: 2819812500, done: true, active: false },
      { id: 10, startOffset: 2819812501, currentOffset: 3133125000, endOffset: 3133125000, done: true, active: false },
      { id: 11, startOffset: 313312501, currentOffset: 3390000000, endOffset: 3446437500, done: false, active: true },
      { id: 12, startOffset: 3446437501, currentOffset: 3650000000, endOffset: 3759750000, done: false, active: true },
      { id: 13, startOffset: 375975001, currentOffset: 3950000000, endOffset: 4073062500, done: false, active: true },
      { id: 14, startOffset: 4073062501, currentOffset: 4200000000, endOffset: 4386375000, done: false, active: true },
      { id: 15, startOffset: 438637501, currentOffset: 4550000000, endOffset: 4699687500, done: false, active: true },
      { id: 16, startOffset: 4699687501, currentOffset: 4820000000, endOffset: 5013000000, done: false, active: true }
    ]
  },
  {
    id: 'task-2',
    filename: 'Blender-4.2.0-windows-x64.zip',
    status: 'Finished',
    percent: 100,
    downloadedBytes: 335544320,
    totalSize: 335544320,
    speed: 0,
    eta: 0,
    timeLeft: 'Completed',
    category: 'Compressed',
    url: 'https://download.blender.org/release/Blender4.2/blender-4.2.0-windows-x64.zip',
    savePath: 'D:\\Downloads\\Compressed',
    queueId: 'main_queue',
    maxConns: 16,
    supportsRange: true
  },
  {
    id: 'task-3',
    filename: 'VSCodeUserSetup-x64-1.93.0.exe',
    status: 'Finished',
    percent: 100,
    downloadedBytes: 98450124,
    totalSize: 98450124,
    speed: 0,
    eta: 0,
    timeLeft: 'Completed',
    category: 'Programs',
    url: 'https://update.code.visualstudio.com/1.93.0/win32-x64-user/stable',
    savePath: 'D:\\Downloads\\Programs',
    queueId: 'main_queue',
    maxConns: 8,
    supportsRange: true
  },
  {
    id: 'task-4',
    filename: 'Nature_Documentary_4K_60fps.mp4',
    status: 'Downloading',
    percent: 58,
    downloadedBytes: 1280000000,
    totalSize: 2200000000,
    speed: 22400000,
    eta: 41,
    timeLeft: '41s',
    category: 'Video',
    url: 'https://youtube.com/watch?v=dQw4w9WgXcQ',
    savePath: 'D:\\Downloads\\Video',
    queueId: 'main_queue',
    maxConns: 16,
    supportsRange: true
  },
  {
    id: 'task-5',
    filename: 'Cyberpunk_Soundtrack_Lossless.flac',
    status: 'Downloading',
    percent: 88,
    downloadedBytes: 748000000,
    totalSize: 850000000,
    speed: 16500000,
    eta: 6,
    timeLeft: '6s',
    category: 'Music',
    url: 'https://music.example.com/soundtrack.flac',
    savePath: 'D:\\Downloads\\Music',
    queueId: 'night_queue',
    maxConns: 8,
    supportsRange: true
  }
];

const MOCK_DRIVES = [
  { mount: "C:", label: "NVMe System OS", fs: "NTFS", free_bytes: 234400000000, total_bytes: 512000000000, free_str: "218.3 GB", total_str: "476.8 GB", used_percent: 54.2 },
  { mount: "D:", label: "High-Speed SSD", fs: "NTFS", free_bytes: 689000000000, total_bytes: 1000000000000, free_str: "641.7 GB", total_str: "931.3 GB", used_percent: 31.1 },
  { mount: "E:", label: "Media Archive", fs: "NTFS", free_bytes: 1240000000000, total_bytes: 2000000000000, free_str: "1.15 TB", total_str: "1.82 TB", used_percent: 36.8 },
  { mount: "F:", label: "Work Data", fs: "NTFS", free_bytes: 350000000000, total_bytes: 500000000000, free_str: "325.9 GB", total_str: "465.7 GB", used_percent: 30.0 }
];

const MOCK_QUEUES = [
  { id: "main_queue", name: "Main Queue", active: true, maxConcurrent: 3, scheduled: false },
  { id: "night_queue", name: "Night Batch Queue", active: false, maxConcurrent: 5, scheduled: true, startTime: "01:00", stopTime: "06:00", shutdownOnComplete: true }
];

const server = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://127.0.0.1:8999');

  if (url.pathname === '/api/downloads' || url.pathname === '/api/tasks') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ downloads: MOCK_DOWNLOADS, tasks: MOCK_DOWNLOADS }));
  }

  if (url.pathname === '/api/queues') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ queues: MOCK_QUEUES }));
  }

  if (url.pathname === '/api/system/drives' || url.pathname === '/api/storage/drives') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ success: true, drives: MOCK_DRIVES }));
  }

  if (url.pathname === '/api/extension/info') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({
      installed: true,
      path: 'C:\\Users\\aloys\\AppData\\Local\\Programs\\NeoDM\\extension',
      browsers: ['chrome', 'edge', 'brave', 'firefox']
    }));
  }

  if (url.pathname === '/api/settings') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({
      autoStart: true,
      autoOpenCompleted: false,
      soundAlertOnComplete: true,
      defaultDownloadDir: 'D:\\Downloads',
      temporaryDirectory: 'D:\\Downloads\\.scratch',
      maxConcurrentDownloads: 5,
      maxConnectionsPerDownload: 16,
      globalSpeedLimit: 0
    }));
  }

  if (url.pathname === '/api/video/analyze') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({
      title: '4K Nature & Wildlife Cinematic Timelapse',
      duration: 624,
      duration_str: '10:24',
      author: 'Earth Cinematics',
      platform: 'YouTube • 60fps Ultra HD',
      thumbnail: 'https://images.unsplash.com/photo-1470071459604-3b5ec3a7fe05?w=640&q=80',
      formats: [
        { id: '1080p', resolution: '1080p (Full HD 60fps)', filesize: 482000000, filesize_str: '482 MB', ext: 'mp4', note: 'Recommended (Auto-Muxed Audio)' },
        { id: '720p', resolution: '720p (HD 60fps)', filesize: 245000000, filesize_str: '245 MB', ext: 'mp4', note: 'Merged Audio' },
        { id: '480p', resolution: '480p (SD)', filesize: 112000000, filesize_str: '112 MB', ext: 'mp4', note: 'Standard' },
        { id: 'audio', resolution: 'Audio Only (320kbps MP3)', filesize: 24000000, filesize_str: '24 MB', ext: 'mp3', note: 'Pristine Stereo' }
      ]
    }));
  }

  // Serve static files from FRONTEND_DIR
  let filePath = path.join(FRONTEND_DIR, url.pathname === '/' ? 'index.html' : url.pathname);
  if (fs.existsSync(filePath) && fs.statSync(filePath).isFile()) {
    const ext = path.extname(filePath);
    res.writeHead(200, { 'Content-Type': MIME_TYPES[ext] || 'application/octet-stream' });
    return fs.createReadStream(filePath).pipe(res);
  }

  res.writeHead(404);
  res.end('Not Found');
});

server.listen(8999, '127.0.0.1', async () => {
  console.log('Mock server listening on http://127.0.0.1:8999');

  try {
    const browser = await puppeteer.launch({
      executablePath: CHROME_PATH,
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox', '--window-size=1280,820']
    });

    const page = await browser.newPage();
    await page.setViewport({ width: 1280, height: 820, deviceScaleFactor: 1.5 });
    await page.goto('http://127.0.0.1:8999', { waitUntil: 'networkidle0' });

    // Wait for initial renders
    await page.waitForSelector('.download-item-card, .table-row-item, #downloads-table', { timeout: 5000 }).catch(() => {});
    await new Promise(r => setTimeout(r, 1200));

    // Ensure clean state: remove any toast or error banner
    await page.evaluate(() => {
      document.querySelectorAll('.toast, .error-banner, #toast-container').forEach(el => el.innerHTML = '');
      // Update global telemetry numbers if available
      const countEl = document.getElementById('stat-active-count');
      if (countEl) countEl.textContent = '3 Active';
      const speedEl = document.getElementById('stat-global-speed');
      if (speedEl) speedEl.textContent = '87.1 MB/s';
    });

    const outDocs = 'd:\\new projects\\ifd\\docs\\screenshots';
    const outShowcase = 'd:\\new projects\\ifd\\neodm-showcase\\public\\screenshots';

    async function saveShot(name) {
      const p1 = path.join(outDocs, name);
      const p2 = path.join(outShowcase, name);
      await page.screenshot({ path: p1 });
      fs.copyFileSync(p1, p2);
      console.log(`Saved screenshot: ${name}`);
    }

    // 1. Dashboard screenshot
    await saveShot('dashboard.png');

    // 2. Download Properties / Segments modal
    await page.evaluate(() => {
      if (typeof window.showSegmentInspector === 'function') {
        window.showSegmentInspector('task-1');
      } else {
        openModal('modal-segment-view');
      }
    });
    await new Promise(r => setTimeout(r, 700));
    await saveShot('download-properties.png');
    await page.evaluate(() => closeModal('modal-segment-view'));
    await new Promise(r => setTimeout(r, 500));

    // 3. Video Downloader Modal (Populated with clean analyzed video)
    await page.evaluate(() => {
      openModal('modal-video-downloader');
      const urlInput = document.getElementById('video-url-input');
      if (urlInput) urlInput.value = 'https://www.youtube.com/watch?v=dQw4w9WgXcQ';
      
      const previewCard = document.getElementById('video-preview-card');
      if (previewCard) previewCard.style.display = 'block';

      const thumbImg = document.getElementById('video-thumb-img-element');
      if (thumbImg) {
        thumbImg.src = 'https://images.unsplash.com/photo-1470071459604-3b5ec3a7fe05?w=640&q=80';
        thumbImg.style.display = 'block';
      }
      const thumbPlaceholder = document.getElementById('video-thumb-placeholder');
      if (thumbPlaceholder) thumbPlaceholder.style.display = 'none';

      const titleEl = document.getElementById('video-title-display');
      if (titleEl) titleEl.textContent = 'Cinematic 4K Nature & Wildlife Timelapse';

      const durationEl = document.getElementById('video-duration-display');
      if (durationEl) durationEl.textContent = '10:24';

      const authorEl = document.getElementById('video-author-display');
      if (authorEl) authorEl.textContent = 'Earth Cinematics';

      const platformEl = document.getElementById('video-platform-display');
      if (platformEl) platformEl.textContent = 'YouTube • 1080p 60fps';

      const optionsWrap = document.getElementById('video-options-wrap');
      if (optionsWrap) optionsWrap.style.display = 'block';

      const selectEl = document.getElementById('video-quality-select');
      if (selectEl) {
        selectEl.innerHTML = `
          <option value="1080p" selected>1080p (Full HD 60fps) — 482 MB (Merged Audio)</option>
          <option value="720p">720p (HD 60fps) — 245 MB (Merged Audio)</option>
          <option value="480p">480p (SD) — 112 MB</option>
          <option value="audio">Audio Only (320kbps MP3) — 24 MB</option>
        `;
      }

      const filenameInput = document.getElementById('video-filename-input');
      if (filenameInput) filenameInput.value = 'Cinematic 4K Nature & Wildlife Timelapse.mp4';

      const folderInput = document.getElementById('video-folder-input');
      if (folderInput) folderInput.value = 'D:\\Downloads\\Video';

      // Ensure no error banner in modal
      const errEl = document.getElementById('video-error-banner');
      if (errEl) errEl.style.display = 'none';
    });
    await new Promise(r => setTimeout(r, 700));
    await saveShot('video-downloader.png');

    // 4. Video Extractor / URL Input (clean empty state)
    await page.evaluate(() => {
      const previewCard = document.getElementById('video-preview-card');
      if (previewCard) previewCard.style.display = 'none';
      const optionsWrap = document.getElementById('video-options-wrap');
      if (optionsWrap) optionsWrap.style.display = 'none';
      const urlInput = document.getElementById('video-url-input');
      if (urlInput) urlInput.value = 'https://www.youtube.com/watch?v=dQw4w9WgXcQ';
    });
    await new Promise(r => setTimeout(r, 500));
    await saveShot('video-extractor.png');
    await page.evaluate(() => closeModal('modal-video-downloader'));
    await new Promise(r => setTimeout(r, 500));

    // 5. Storage Settings (Showing clean host drive capacity gauges)
    await page.evaluate(async () => {
      openModal('modal-settings');
      // Switch to storage tab
      const storageTabBtn = document.querySelector('button[data-tab="stab-storage"]');
      if (storageTabBtn) storageTabBtn.click();
      if (typeof window.refreshHostDrives === 'function') {
        await window.refreshHostDrives();
      }
      const warningBanner = document.getElementById('drive-low-space-banner');
      if (warningBanner) warningBanner.style.display = 'none';
    });
    await new Promise(r => setTimeout(r, 700));
    await saveShot('storage-settings.png');
    await page.evaluate(() => closeModal('modal-settings'));
    await new Promise(r => setTimeout(r, 500));

    // 6. Browser Integration Modal
    await page.evaluate(() => {
      openModal('modal-browser-integration');
    });
    await new Promise(r => setTimeout(r, 700));
    await saveShot('browser-integration.png');
    await page.evaluate(() => closeModal('modal-browser-integration'));
    await new Promise(r => setTimeout(r, 500));

    // 7. Queue Manager Modal
    await page.evaluate(() => {
      openModal('modal-queue-manager');
    });
    await new Promise(r => setTimeout(r, 700));
    await saveShot('queue-manager.png');
    await page.evaluate(() => closeModal('modal-queue-manager'));

    console.log('All fresh screenshots captured and synced successfully!');
    await browser.close();
  } catch (err) {
    console.error('Error during screenshot generation:', err);
  } finally {
    server.close();
    process.exit(0);
  }
});
