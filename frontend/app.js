// NeoDM - Neo Download Manager PRO Frontend Controller

let ws = null;
let downloads = [];
let queues = [];
let currentCategory = 'all';
let currentQueueFilter = null;
let currentStatusFilter = 'all';
let searchQuery = '';
let activeInspectId = null;
let selectedCtxTask = null;
let selectedTaskIds = new Set();
let speedHistory = new Array(30).fill(0);
let bottomSpeedHistory = new Array(40).fill(0);
let speedCanvas, speedCtx;
let bottomSpeedCanvas, bottomSpeedCtx;
let activeViewMode = 'table'; // 'table' or 'grid'

document.addEventListener('DOMContentLoaded', () => {
  initTitlebarAndThemes();
  initNavigationAndToolbar();
  initQuickDownloadBar();
  initCanvases();
  initShortcutsAndDragDrop();
  initContextMenu();
  initModals();
  connectWebSocket();
  loadInitialData();
});

// Titlebar & Theme Management
function initTitlebarAndThemes() {
  const btnMin = document.getElementById('win-btn-min');
  const btnMax = document.getElementById('win-btn-max');
  const btnClose = document.getElementById('win-btn-close');
  const titlebar = document.getElementById('window-titlebar');

  if (btnMin) btnMin.addEventListener('click', () => windowAction('minimize'));
  if (btnMax) btnMax.addEventListener('click', () => windowAction('maximize'));
  if (btnClose) btnClose.addEventListener('click', () => windowAction('close'));

  if (titlebar) {
    titlebar.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return; // Only drag on primary left click
      if (e.target.closest('button, select, input, .search-pill, .theme-select-wrap, .window-controls')) return;
      windowAction('drag');
    });
    titlebar.addEventListener('dblclick', (e) => {
      if (e.target.closest('button, select, input, .search-pill, .theme-select-wrap, .window-controls')) return;
      windowAction('maximize');
    });
  }

  // Search input in titlebar
  const searchInput = document.getElementById('global-search-input');
  if (searchInput) {
    searchInput.addEventListener('input', (e) => {
      searchQuery = e.target.value.toLowerCase().trim();
      renderDownloads();
    });
  }

  // Stats drawer toggle button (Default is collapsed for a clean, simple, un-congested layout)
  const btnToggleStats = document.getElementById('btn-toggle-stats');
  const statsDrawer = document.getElementById('stats-overview-drawer');
  const statsBtnText = document.getElementById('stats-btn-text');

  const isStatsCollapsed = localStorage.getItem('neodm-stats-collapsed') !== 'false';
  if (statsDrawer) {
    if (isStatsCollapsed) {
      statsDrawer.classList.add('collapsed');
      if (statsBtnText) statsBtnText.textContent = 'Show Stats';
    } else {
      statsDrawer.classList.remove('collapsed');
      if (statsBtnText) statsBtnText.textContent = 'Hide Stats';
    }
  }

  if (btnToggleStats && statsDrawer) {
    btnToggleStats.addEventListener('click', () => {
      statsDrawer.classList.toggle('collapsed');
      const collapsed = statsDrawer.classList.contains('collapsed');
      localStorage.setItem('neodm-stats-collapsed', collapsed ? 'true' : 'false');
      if (statsBtnText) statsBtnText.textContent = collapsed ? 'Show Stats' : 'Hide Stats';
    });
  }

  // Theme Dropdown (Default is Vercel Minimalist Clean Dark Theme)
  const themeSelector = document.getElementById('theme-selector');
  const settingThemePicker = document.getElementById('setting-theme-picker');
  const savedTheme = localStorage.getItem('neodm-theme') || 'vercel';

  applyTheme(savedTheme, false);

  if (themeSelector) {
    themeSelector.value = savedTheme;
    themeSelector.addEventListener('change', (e) => applyTheme(e.target.value, true));
  }

  if (settingThemePicker) {
    settingThemePicker.value = savedTheme;
    settingThemePicker.addEventListener('change', (e) => applyTheme(e.target.value, true));
  }
}

function applyTheme(theme, notify = true) {
  document.documentElement.setAttribute('data-theme', theme);
  localStorage.setItem('neodm-theme', theme);

  const themeSelector = document.getElementById('theme-selector');
  if (themeSelector) themeSelector.value = theme;
  const settingThemePicker = document.getElementById('setting-theme-picker');
  if (settingThemePicker) settingThemePicker.value = theme;

  const names = {
    vercel: '▲ Vercel Dark (Minimalist)',
    nova: 'Nova Dark PRO',
    glass: 'Bento Glassmorphism',
    dracula: 'Dracula Pro',
    nord: 'Nordic Frost',
    cyberpunk: 'Cyberpunk Neon',
    oled: 'Midnight OLED',
    light: 'Fluent Light'
  };
  if (notify) {
    showToast(`Theme: ${names[theme] || theme}`);
  }
}

function windowAction(action) {
  if (typeof window.nativeWindowAction === 'function') {
    try {
      window.nativeWindowAction(action);
      return;
    } catch (err) {
      console.warn('nativeWindowAction error:', err);
    }
  }
  fetch('/api/window/action', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action })
  }).catch(() => {});
}

// Quick Download Bar (Easy 1-Click URL Input for General Users)
function initQuickDownloadBar() {
  const input = document.getElementById('quick-url-input');
  const btn = document.getElementById('btn-quick-start');

  if (!input || !btn) return;

  async function handleQuickDownload() {
    const raw = input.value.trim();
    if (!raw) {
      input.focus();
      return;
    }
    const targetUrl = extractUrlFromText(raw) || raw;
    if (!targetUrl.startsWith('http://') && !targetUrl.startsWith('https://') && !targetUrl.startsWith('ftp://') && !targetUrl.startsWith('magnet:?')) {
      showToast('Please enter a valid HTTP, HTTPS, or FTP URL', 'warning');
      input.focus();
      return;
    }

    try {
      btn.disabled = true;
      btn.textContent = 'Starting...';

      const res = await fetch('/api/download/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: targetUrl,
          filename: '',
          saveDir: '',
          queueName: 'main_queue'
        })
      });

      const data = await res.json();
      if (data.success) {
        input.value = '';
        showToast(`⚡ Download started: ${data.task ? data.task.filename : 'File added'}`, 'success');
        loadInitialData();
      } else {
        showToast(`Failed: ${data.error || 'Could not start download'}`, 'danger');
      }
    } catch (err) {
      showToast(`Network error: ${err.message}`, 'danger');
    } finally {
      btn.disabled = false;
      btn.textContent = 'Download Now';
    }
  }

  btn.addEventListener('click', handleQuickDownload);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleQuickDownload();
    }
  });

  input.addEventListener('paste', () => {
    setTimeout(() => {
      const extracted = extractUrlFromText(input.value);
      if (extracted) {
        input.value = extracted;
      }
    }, 10);
  });

  // Automatically suggest clipboard link when user clicks into the quick input
  input.addEventListener('focus', async () => {
    if (!input.value.trim()) {
      try {
        const text = await readAppClipboard();
        const extracted = extractUrlFromText(text);
        if (extracted) {
          input.value = extracted;
          input.select();
        }
      } catch (_) {}
    }
  });
}

// Navigation & Toolbar Management
function initNavigationAndToolbar() {
  // Category nav
  const catItems = document.querySelectorAll('#category-nav-list .nav-item');
  catItems.forEach(item => {
    item.addEventListener('click', () => {
      catItems.forEach(i => i.classList.remove('active'));
      const queueItems = document.querySelectorAll('#queue-nav-list .nav-item');
      queueItems.forEach(q => q.classList.remove('active'));

      item.classList.add('active');
      currentCategory = item.getAttribute('data-category');
      currentQueueFilter = null;
      renderDownloads();
    });
  });

  // Tools nav
  const toolBrowser = document.getElementById('tool-browser-integration');
  if (toolBrowser) toolBrowser.addEventListener('click', () => openModal('modal-browser-integration'));

  const toolVideo = document.getElementById('tool-video-downloader');
  if (toolVideo) toolVideo.addEventListener('click', () => openModal('modal-video-downloader'));

  const toolBatch = document.getElementById('tool-batch-downloader');
  if (toolBatch) toolBatch.addEventListener('click', () => {
    openModal('modal-add-download');
    switchAddTab('tab-add-batch');
  });

  const toolScheduler = document.getElementById('tool-scheduler');
  if (toolScheduler) toolScheduler.addEventListener('click', () => openQueueManager());

  const toolGrabber = document.getElementById('tool-site-grabber');
  if (toolGrabber) toolGrabber.addEventListener('click', () => {
    openModal('modal-add-download');
    switchAddTab('tab-add-grabber');
  });

  const btnOpenSettings = document.getElementById('btn-open-settings');
  if (btnOpenSettings) btnOpenSettings.addEventListener('click', () => openSettingsModal());

  const btnAddHero = document.getElementById('btn-add-download');
  if (btnAddHero) btnAddHero.addEventListener('click', () => {
    openModal('modal-add-download');
    switchAddTab('tab-add-url');
    const input = document.getElementById('add-url-input');
    if (input) setTimeout(() => input.focus(), 100);
  });

  // Toolbar Actions
  const btnStartAll = document.getElementById('btn-start-all');
  if (btnStartAll) btnStartAll.addEventListener('click', () => batchAction('start_all'));

  const btnPauseAll = document.getElementById('btn-pause-all');
  if (btnPauseAll) btnPauseAll.addEventListener('click', () => batchAction('pause_all'));

  const btnDeleteCompleted = document.getElementById('btn-delete-completed');
  if (btnDeleteCompleted) {
    btnDeleteCompleted.addEventListener('click', () => {
      if (selectedTaskIds.size > 0) {
        batchAction('delete_selected');
      } else {
        batchAction('delete_completed');
      }
    });
  }

  // More Actions Dropdown
  const btnMoreActions = document.getElementById('btn-more-actions');
  const moreActionsMenu = document.getElementById('more-actions-menu');
  const toolbarMoreWrap = document.getElementById('toolbar-more-actions-wrap');
  if (btnMoreActions && moreActionsMenu) {
    btnMoreActions.addEventListener('click', (e) => {
      e.stopPropagation();
      const isOpen = moreActionsMenu.classList.toggle('open');
      btnMoreActions.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
      if (toolbarMoreWrap) toolbarMoreWrap.classList.toggle('open', isOpen);
    });

    document.addEventListener('click', (e) => {
      if (!moreActionsMenu.contains(e.target) && e.target !== btnMoreActions) {
        moreActionsMenu.classList.remove('open');
        btnMoreActions.setAttribute('aria-expanded', 'false');
        if (toolbarMoreWrap) toolbarMoreWrap.classList.remove('open');
      }
    });

    moreActionsMenu.querySelectorAll('.dropdown-item').forEach(item => {
      item.addEventListener('click', () => {
        moreActionsMenu.classList.remove('open');
        btnMoreActions.setAttribute('aria-expanded', 'false');
        if (toolbarMoreWrap) toolbarMoreWrap.classList.remove('open');
      });
    });
  }

  const actionResumeAll = document.getElementById('action-resume-all');
  if (actionResumeAll) actionResumeAll.addEventListener('click', () => {
    batchAction('start_all');
  });

  const actionPauseAll = document.getElementById('action-pause-all');
  if (actionPauseAll) actionPauseAll.addEventListener('click', () => {
    batchAction('pause_all');
  });

  const actionCleanList = document.getElementById('action-clean-list');
  if (actionCleanList) actionCleanList.addEventListener('click', () => {
    batchAction('delete_completed');
  });

  const actionDeleteAllFiles = document.getElementById('action-delete-all-files');
  if (actionDeleteAllFiles) actionDeleteAllFiles.addEventListener('click', () => {
    if (confirm('Are you sure you want to delete ALL downloaded files on disk? This cannot be undone.')) {
      batchAction('delete_all_files');
    }
  });

  // Filter Dropdown
  const statusFilter = document.getElementById('status-filter');
  if (statusFilter) {
    statusFilter.addEventListener('change', (e) => {
      currentStatusFilter = e.target.value;
      renderDownloads();
    });
  }

  // View Mode Toggles
  const viewModeTable = document.getElementById('view-mode-table');
  const viewModeGrid = document.getElementById('view-mode-grid');
  const mainTable = document.getElementById('main-downloads-table');
  const mainGrid = document.getElementById('downloads-grid');

  if (viewModeTable && viewModeGrid) {
    viewModeTable.addEventListener('click', () => {
      viewModeTable.classList.add('active');
      viewModeGrid.classList.remove('active');
      activeViewMode = 'table';
      if (mainTable) mainTable.style.display = 'table';
      if (mainGrid) mainGrid.style.display = 'none';
      renderDownloads();
    });

    viewModeGrid.addEventListener('click', () => {
      viewModeGrid.classList.add('active');
      viewModeTable.classList.remove('active');
      activeViewMode = 'grid';
      if (mainTable) mainTable.style.display = 'none';
      if (mainGrid) mainGrid.style.display = 'grid';
      renderDownloads();
    });
  }

  // Check all master checkbox
  const checkAll = document.getElementById('check-all-downloads');
  if (checkAll) {
    checkAll.addEventListener('change', (e) => {
      const filtered = getFilteredDownloads();
      if (e.target.checked) {
        filtered.forEach(d => selectedTaskIds.add(d.id));
      } else {
        selectedTaskIds.clear();
      }
      renderDownloads();
    });
  }
}

async function batchAction(action, ids = null) {
  try {
    const res = await fetch('/api/download/batch_action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, ids: ids || Array.from(selectedTaskIds) })
    });
    if (res.ok) {
      if (action === 'delete_completed' || action === 'delete_selected') {
        selectedTaskIds.clear();
      }
      showToast(`Batch ${action.replace('_', ' ')} executed`);
      loadInitialData();
    }
  } catch (e) {
    console.error('Batch action error:', e);
  }
}

// Sparklines & Telemetry Canvases
function initCanvases() {
  speedCanvas = document.getElementById('speed-canvas');
  if (speedCanvas) speedCtx = speedCanvas.getContext('2d');

  bottomSpeedCanvas = document.getElementById('bottom-speed-canvas');
  if (bottomSpeedCanvas) bottomSpeedCtx = bottomSpeedCanvas.getContext('2d');
}

function updateThroughputCharts(currentSpeed) {
  // Update drawer sparkline
  speedHistory.push(currentSpeed);
  speedHistory.shift();

  if (speedCtx && speedCanvas) {
    drawSplineArea(speedCtx, speedCanvas.width, speedCanvas.height, speedHistory, '#38bdf8', 'rgba(56, 189, 248, 0.25)');
  }

  // Update bottom telemetry chart
  bottomSpeedHistory.push(currentSpeed);
  bottomSpeedHistory.shift();

  if (bottomSpeedCtx && bottomSpeedCanvas) {
    drawSplineArea(bottomSpeedCtx, bottomSpeedCanvas.width, bottomSpeedCanvas.height, bottomSpeedHistory, '#2563eb', 'rgba(37, 99, 235, 0.3)');
  }
}

function drawSplineArea(ctx, width, height, data, strokeColor, fillColor) {
  ctx.clearRect(0, 0, width, height);
  const maxVal = Math.max(...data, 1024 * 1024); // at least 1MB/s scale

  const step = width / (data.length - 1);
  const points = data.map((val, idx) => ({
    x: idx * step,
    y: height - (val / maxVal) * (height - 6) - 3
  }));

  // Path with smooth bezier curves
  ctx.beginPath();
  ctx.moveTo(points[0].x, points[0].y);

  for (let i = 0; i < points.length - 1; i++) {
    const xc = (points[i].x + points[i + 1].x) / 2;
    const yc = (points[i].y + points[i + 1].y) / 2;
    ctx.quadraticCurveTo(points[i].x, points[i].y, xc, yc);
  }
  ctx.lineTo(points[points.length - 1].x, points[points.length - 1].y);

  // Stroke line
  ctx.strokeStyle = strokeColor;
  ctx.lineWidth = 2;
  ctx.stroke();

  // Fill gradient underneath
  ctx.lineTo(width, height);
  ctx.lineTo(0, height);
  ctx.closePath();

  const grad = ctx.createLinearGradient(0, 0, 0, height);
  grad.addColorStop(0, fillColor);
  grad.addColorStop(1, 'transparent');
  ctx.fillStyle = grad;
  ctx.fill();
}

// Robust URL extractor supporting quoted strings, multi-line blocks, angle brackets, and protocols
function extractUrlFromText(text) {
  if (!text || typeof text !== 'string') return '';
  const trimmed = text.trim();
  if (!trimmed) return '';

  // Match http, https, ftp, or magnet URI anywhere in text
  const match = trimmed.match(/(https?:\/\/[^\s"'<>]+|ftp:\/\/[^\s"'<>]+|magnet:\?[^\s"'<>]+)/i);
  if (match) {
    let url = match[0].trim();
    // Strip trailing punctuation like .,;:)>]
    url = url.replace(/[.,;:)>\]]+$/, '');
    return url;
  }
  return '';
}

// Native & Web Clipboard Reader with Triple-Fallback
async function readAppClipboard() {
  if (typeof window.nativeReadClipboard === 'function') {
    try {
      const text = await window.nativeReadClipboard();
      if (text && typeof text === 'string') return text.trim();
    } catch (_) {}
  }
  if (navigator.clipboard && navigator.clipboard.readText) {
    try {
      const text = await navigator.clipboard.readText();
      if (text && typeof text === 'string') return text.trim();
    } catch (_) {}
  }
  try {
    const res = await fetch('/api/clipboard/read');
    if (res.ok) {
      const data = await res.json();
      if (data && data.text) return data.text.trim();
    }
  } catch (_) {}
  return '';
}

// Shortcuts & Drag and Drop
function initShortcutsAndDragDrop() {
  // Global Ctrl+V / Ctrl+N / Ctrl+F / Escape
  window.addEventListener('keydown', async (e) => {
    // Escape key closes open menus and modals
    if (e.key === 'Escape') {
      const moreActionsMenu = document.getElementById('more-actions-menu');
      if (moreActionsMenu) moreActionsMenu.classList.remove('open');
      const ctxMenu = document.getElementById('custom-context-menu');
      if (ctxMenu) ctxMenu.style.display = 'none';
      document.querySelectorAll('.modal-overlay.open').forEach(m => m.classList.remove('open'));
      return;
    }

    // Native Ctrl+V handling anywhere in the app
    if ((e.ctrlKey || e.metaKey) && (e.key === 'v' || e.key === 'V' || e.code === 'KeyV')) {
      const activeEl = document.activeElement;
      const tag = activeEl ? activeEl.tagName : '';
      const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || activeEl?.isContentEditable;
      const isQuickInput = activeEl && activeEl.id === 'quick-url-input';
      if (isInput && !isQuickInput) {
        return; // Allow native paste into active input controls (e.g. search, add URL dialog)
      }

      e.preventDefault();
      const text = await readAppClipboard();
      if (!text) {
        showToast('Clipboard is empty', 'warning');
        return;
      }

      const extracted = extractUrlFromText(text);
      if (extracted) {
        const quickInput = document.getElementById('quick-url-input');
        if (quickInput) {
          quickInput.value = extracted;
          quickInput.focus();
          quickInput.select();

          const wrap = document.querySelector('.quick-input-wrap');
          if (wrap) {
            wrap.classList.add('clipboard-highlight');
            setTimeout(() => wrap.classList.remove('clipboard-highlight'), 1200);
          }

          const domain = extractDomain(extracted);
          showToast(`⚡ Link detected: ${domain || extracted}`, 'info', {
            text: 'Download Now',
            onClick: () => {
              const btn = document.getElementById('btn-quick-start');
              if (btn) btn.click();
            }
          });
        }
      } else {
        showToast('Clipboard does not contain a valid URL', 'warning');
      }
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'n' || e.key === 'N')) {
      e.preventDefault();
      openModal('modal-add-download');
    }

    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F')) {
      e.preventDefault();
      const s = document.getElementById('global-search-input');
      if (s) s.focus();
    }
  });

  // Drag and Drop URLs
  const overlay = document.getElementById('drop-zone-overlay');
  let dragCounter = 0;

  window.addEventListener('dragenter', (e) => {
    dragCounter++;
    if (overlay) overlay.classList.add('drag-active');
  });

  window.addEventListener('dragleave', (e) => {
    dragCounter--;
    if (dragCounter <= 0 && overlay) {
      overlay.classList.remove('drag-active');
      dragCounter = 0;
    }
  });

  window.addEventListener('dragover', (e) => e.preventDefault());

  window.addEventListener('drop', (e) => {
    e.preventDefault();
    dragCounter = 0;
    if (overlay) overlay.classList.remove('drag-active');

    const text = e.dataTransfer.getData('text');
    if (text && (text.startsWith('http://') || text.startsWith('https://'))) {
      openModal('modal-add-download');
      switchAddTab('tab-add-url');
      const input = document.getElementById('add-url-input');
      if (input) {
        input.value = text.trim();
        setTimeout(() => input.focus(), 100);
      }
      showToast('Dropped link captured');
    }
  });
}

// Context Menu Management
function initContextMenu() {
  const ctxMenu = document.getElementById('custom-context-menu');
  if (!ctxMenu) return;

  document.addEventListener('click', () => {
    ctxMenu.style.display = 'none';
  });

  window.addEventListener('contextmenu', (e) => {
    const row = e.target.closest('tr[data-id], .grid-card[data-id]');
    if (!row) {
      ctxMenu.style.display = 'none';
      return;
    }
    e.preventDefault();
    const taskId = row.getAttribute('data-id');
    selectedCtxTask = downloads.find(d => d.id === taskId);
    if (!selectedCtxTask) return;

    // Position menu safely inside window bounds
    const menuWidth = 200;
    const menuHeight = 320;
    let posX = e.clientX;
    let posY = e.clientY;

    if (posX + menuWidth > window.innerWidth) posX = window.innerWidth - menuWidth - 10;
    if (posY + menuHeight > window.innerHeight) posY = window.innerHeight - menuHeight - 10;

    ctxMenu.style.left = `${posX}px`;
    ctxMenu.style.top = `${posY}px`;
    ctxMenu.style.display = 'block';

    // Populate dynamic queue choices
    renderContextQueueList();
  });

  // Action listeners
  document.getElementById('ctx-open-file')?.addEventListener('click', () => {
    if (selectedCtxTask?.finalPath) openFileOrFolder(selectedCtxTask.finalPath, 'file');
  });
  document.getElementById('ctx-open-folder')?.addEventListener('click', () => {
    if (selectedCtxTask?.savePath) openFileOrFolder(selectedCtxTask.savePath, 'folder');
  });
  document.getElementById('ctx-resume')?.addEventListener('click', () => {
    if (selectedCtxTask) downloadAction(selectedCtxTask.id, 'resume');
  });
  document.getElementById('ctx-pause')?.addEventListener('click', () => {
    if (selectedCtxTask) downloadAction(selectedCtxTask.id, 'pause');
  });
  document.getElementById('ctx-restart')?.addEventListener('click', () => {
    if (selectedCtxTask) downloadAction(selectedCtxTask.id, 'restart');
  });
  document.getElementById('ctx-delete-list')?.addEventListener('click', () => {
    if (selectedCtxTask) downloadAction(selectedCtxTask.id, 'delete');
  });
  document.getElementById('ctx-delete-file')?.addEventListener('click', () => {
    if (selectedCtxTask) downloadAction(selectedCtxTask.id, 'delete_file');
  });
  document.getElementById('ctx-copy-link')?.addEventListener('click', () => {
    if (selectedCtxTask?.url) {
      navigator.clipboard.writeText(selectedCtxTask.url);
      showToast('Download link copied to clipboard');
    }
  });
  document.getElementById('ctx-view-segments')?.addEventListener('click', () => {
    if (selectedCtxTask) openSegmentVisualizer(selectedCtxTask.id);
  });

  // Speed Limit Submenu
  document.querySelectorAll('.ctx-submenu [data-speed]').forEach(el => {
    el.addEventListener('click', () => {
      const limit = parseInt(el.getAttribute('data-speed'), 10);
      if (selectedCtxTask) setTaskSpeedLimit(selectedCtxTask.id, limit);
    });
  });

  // Conns Submenu
  document.querySelectorAll('.ctx-submenu [data-conns]').forEach(el => {
    el.addEventListener('click', () => {
      const conns = parseInt(el.getAttribute('data-conns'), 10);
      if (selectedCtxTask) setTaskConns(selectedCtxTask.id, conns);
    });
  });
}

function renderContextQueueList() {
  const list = document.getElementById('ctx-queue-list');
  if (!list) return;
  list.innerHTML = '';

  queues.forEach(q => {
    const item = document.createElement('div');
    item.className = 'ctx-subitem';
    item.textContent = q.name;
    item.addEventListener('click', () => {
      if (selectedCtxTask) {
        moveTaskQueue(selectedCtxTask.id, q.id);
      }
    });
    list.appendChild(item);
  });
}

async function moveTaskQueue(taskId, targetQueueId) {
  try {
    const res = await fetch('/api/queue/move_task', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ taskId, targetQueueId })
    });
    if (res.ok) {
      showToast('Moved to queue');
      loadInitialData();
    }
  } catch (e) {
    console.error('Move queue error:', e);
  }
}

// WebSocket Connection
function connectWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    updateEngineStatus(true);
  };

  ws.onclose = () => {
    updateEngineStatus(false);
    setTimeout(connectWebSocket, 2000);
  };

  ws.onerror = () => {
    updateEngineStatus(false);
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleWebSocketMessage(msg);
    } catch (e) {
      console.error('WS parse error:', e);
    }
  };
}

function updateEngineStatus(connected) {
  const dot = document.querySelector('.sidebar-status-pill .status-dot');
  const label = document.getElementById('backend-status');
  if (dot) dot.style.background = connected ? 'var(--success)' : 'var(--danger)';
  if (label) label.textContent = connected ? 'Engine Connected' : 'Disconnected (Retrying)';
}

function handleWebSocketMessage(msg) {
  if (msg.type === 'telemetry') {
    handleTelemetry(msg.data);
  } else if (msg.type === 'task_added') {
    const existing = downloads.find(d => d.id === msg.data.id);
    if (!existing) {
      downloads.unshift(msg.data);
      renderDownloads();
    }
  } else if (msg.type === 'task_deleted') {
    downloads = downloads.filter(d => d.id !== msg.data.id);
    renderDownloads();
  } else if (msg.type === 'clipboard_url') {
    const raw = typeof msg.data === 'string' ? msg.data : (msg.data?.url || '');
    const url = extractUrlFromText(raw);
    if (url) {
      const quickInput = document.getElementById('quick-url-input');
      if (quickInput && !quickInput.value.trim()) {
        quickInput.value = url.trim();
        const wrap = document.querySelector('.quick-input-wrap');
        if (wrap) {
          wrap.classList.add('clipboard-highlight');
          setTimeout(() => wrap.classList.remove('clipboard-highlight'), 1200);
        }
        showToast(`⚡ Link detected: ${extractDomain(url)}`, 'info', {
          text: 'Download Now',
          onClick: () => {
            const btn = document.getElementById('btn-quick-start');
            if (btn) btn.click();
          }
        });
      }
    }
  }
}

function handleTelemetry(data) {
  if (!data) return;

  // Update tasks in local state
  if (data.tasks) {
    data.tasks.forEach(updated => {
      const idx = downloads.findIndex(d => d.id === updated.id);
      if (idx !== -1) {
        downloads[idx] = { ...downloads[idx], ...updated };
      } else {
        downloads.push(updated);
      }
    });
  }

  // Calculate totals
  const totalSpeed = data.totalSpeed || 0;
  updateThroughputCharts(totalSpeed);

  // Update Bottom Telemetry Bar
  const speedVal = document.getElementById('telemetry-speed-val');
  if (speedVal) speedVal.textContent = formatSpeed(totalSpeed);

  const activeCountEl = document.getElementById('telemetry-active-count');
  if (activeCountEl) activeCountEl.textContent = data.activeDownloads || 0;

  const queuedCountEl = document.getElementById('telemetry-queued-count');
  const qCount = downloads.filter(d => d.status === 'Queued').length;
  if (queuedCountEl) queuedCountEl.textContent = qCount;

  const completedTodayEl = document.getElementById('telemetry-completed-count');
  const cCount = downloads.filter(d => d.status === 'Completed').length;
  if (completedTodayEl) completedTodayEl.textContent = cCount;

  // Update Overview Drawer metrics
  const bentoSpeed = document.getElementById('bento-speed-val');
  const bentoUnit = document.getElementById('bento-speed-unit');
  if (bentoSpeed && bentoUnit) {
    const [num, unit] = formatSpeedParts(totalSpeed);
    bentoSpeed.textContent = num;
    bentoUnit.textContent = unit;
  }

  const bentoActive = document.getElementById('bento-active-count');
  if (bentoActive) bentoActive.textContent = data.activeDownloads || 0;

  const bentoDone = document.getElementById('bento-completed-count');
  if (bentoDone) bentoDone.textContent = cCount;

  const bentoPaused = document.getElementById('bento-paused-count');
  if (bentoPaused) bentoPaused.textContent = downloads.filter(d => d.status === 'Paused').length;

  const bentoThreads = document.getElementById('bento-threads-val');
  if (bentoThreads) {
    let threads = 0;
    downloads.forEach(d => {
      if ((d.status === 'Downloading' || d.status === 'Connecting') && d.segments) {
        threads += d.segments.filter(s => !s.done).length;
      }
    });
    bentoThreads.textContent = threads;
  }

  // Update live row speeds & progress smoothly without re-rendering entire table
  updateLiveRowElements();

  // If segment visualizer is currently open, refresh it
  if (activeInspectId) {
    const task = downloads.find(d => d.id === activeInspectId);
    if (task) renderSegmentModalData(task);
  }
}

// Initial Data Load
async function loadInitialData() {
  try {
    const [dRes, qRes] = await Promise.all([
      fetch('/api/downloads'),
      fetch('/api/queues')
    ]);

    if (dRes.ok) {
      const dJson = await dRes.json();
      downloads = dJson.downloads || [];
      renderDownloads();
      updateCategoryBadges();
    }

    if (qRes.ok) {
      const qJson = await qRes.json();
      queues = qJson.queues || [];
      renderQueueNavList();
      populateQueueDropdowns();
    }
  } catch (e) {
    console.error('Initial load error:', e);
  }
}

// Downloads Table / Grid Rendering
function getFilteredDownloads() {
  return downloads.filter(d => {
    // Category filter
    if (currentCategory !== 'all') {
      if (d.category !== currentCategory) return false;
    }
    // Queue filter
    if (currentQueueFilter) {
      if (d.queueId !== currentQueueFilter) return false;
    }
    // Status filter
    if (currentStatusFilter !== 'all') {
      if (d.status !== currentStatusFilter) return false;
    }
    // Search query
    if (searchQuery) {
      const filenameMatch = d.filename?.toLowerCase().includes(searchQuery);
      const urlMatch = d.url?.toLowerCase().includes(searchQuery);
      if (!filenameMatch && !urlMatch) return false;
    }
    return true;
  });
}

function renderDownloads() {
  const filtered = getFilteredDownloads();
  const tbody = document.getElementById('downloads-tbody');
  const grid = document.getElementById('downloads-grid');
  const emptyState = document.getElementById('empty-state');
  const deleteBtnLabel = document.getElementById('btn-delete-label');
  const deleteBtn = document.getElementById('btn-delete-completed');

  updateCategoryBadges();

  if (deleteBtnLabel) {
    deleteBtnLabel.textContent = selectedTaskIds.size > 0 ? `Delete (${selectedTaskIds.size})` : 'Delete';
  }
  if (deleteBtn) {
    deleteBtn.title = selectedTaskIds.size > 0 ? 'Delete Selected Downloads' : 'Clear Completed Downloads';
  }

  if (filtered.length === 0) {
    if (tbody) tbody.innerHTML = '';
    if (grid) grid.innerHTML = '';
    if (emptyState) emptyState.style.display = 'flex';
    return;
  }
  if (emptyState) emptyState.style.display = 'none';

  if (activeViewMode === 'table') {
    if (tbody) tbody.innerHTML = filtered.map(d => renderTableRow(d)).join('');
    attachRowEventListeners();
  } else {
    if (grid) grid.innerHTML = filtered.map(d => renderGridCard(d)).join('');
    attachGridEventListeners();
  }
}

function renderTableRow(d) {
  const isSelected = selectedTaskIds.has(d.id);
  const percent = calculatePercent(d);
  const speedText = (d.status === 'Downloading' && d.speed > 0) ? formatSpeed(d.speed) : '—';
  const sizeText = formatSize(d.downloadedBytes || 0) + ' / ' + formatSize(d.totalSize || 0);
  const etaText = (d.status === 'Downloading' && d.eta > 0) ? formatETA(d.eta) : (d.status === 'Completed' ? 'Complete' : '—');
  const icon = getFileIcon(d.filename, d.category);
  const domain = extractDomain(d.url);

  return `
    <tr data-id="${d.id}" class="${isSelected ? 'selected' : ''}">
      <td style="text-align: center;">
        <input type="checkbox" class="row-checkbox" ${isSelected ? 'checked' : ''} data-id="${d.id}">
      </td>
      <td>
        <div class="file-cell">
          <div class="file-icon-badge">${icon}</div>
          <div class="file-text-wrap">
            <span class="file-title" title="${escapeHtml(d.filename)}">${escapeHtml(d.filename)}</span>
            <span class="file-subtitle">${escapeHtml(domain)}</span>
          </div>
        </div>
      </td>
      <td>
        <span class="status-pill status-${d.status.toLowerCase()}">${d.status}</span>
      </td>
      <td>
        <div class="progress-cell-wrap">
          <div class="progress-info-row">
            <span class="row-percent">${percent}%</span>
          </div>
          <div class="progress-track">
            <div class="progress-fill ${d.status.toLowerCase()}" style="width: ${percent}%;"></div>
          </div>
        </div>
      </td>
      <td><span class="row-speed">${speedText}</span></td>
      <td><span class="row-size">${sizeText}</span></td>
      <td><span class="row-eta">${etaText}</span></td>
      <td style="text-align: right;">
        <div class="row-actions">
          ${d.status === 'Downloading' ? `
            <button class="btn-row-action" data-action="pause" title="Pause">⏸</button>
          ` : d.status === 'Completed' ? `
            <button class="btn-row-action" data-action="open-file" title="Open File">📄</button>
          ` : `
            <button class="btn-row-action" data-action="resume" title="Resume">▶</button>
          `}
          <button class="btn-row-action" data-action="folder" title="Open Folder">📁</button>
          <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
          <button class="btn-row-action" data-action="inspect" title="Segments & Details">···</button>
        </div>
      </td>
    </tr>
  `;
}

function renderGridCard(d) {
  const percent = calculatePercent(d);
  const icon = getFileIcon(d.filename, d.category);
  const isSelected = selectedTaskIds.has(d.id);
  const speedText = (d.status === 'Downloading' && d.speed > 0) ? formatSpeed(d.speed) : (d.status === 'Completed' ? 'Complete' : '—');
  const sizeText = formatSize(d.downloadedBytes || 0) + ' / ' + formatSize(d.totalSize || 0);
  const domain = extractDomain(d.url);

  return `
    <div class="grid-card ${isSelected ? 'selected' : ''}" data-id="${d.id}">
      <div class="grid-card-header">
        <div class="grid-card-identity">
          <div class="grid-card-icon">${icon}</div>
          <div class="grid-card-title-wrap">
            <h4 class="grid-card-title" title="${escapeHtml(d.filename)}">${escapeHtml(d.filename)}</h4>
            <span class="grid-card-domain">${escapeHtml(domain)}</span>
          </div>
        </div>
        <span class="status-pill status-${d.status.toLowerCase()}">${d.status}</span>
      </div>

      <div class="grid-card-progress-wrap">
        <div class="grid-card-progress-info">
          <span class="grid-card-percent">${percent}%</span>
          <span class="grid-card-speed">${speedText}</span>
        </div>
        <div class="progress-track">
          <div class="progress-fill ${d.status.toLowerCase()}" style="width: ${percent}%;"></div>
        </div>
      </div>

      <div class="grid-card-actions">
        <span class="grid-card-size-info">${sizeText}</span>
        <div class="row-actions">
          ${d.status === 'Downloading' ? `
            <button class="btn-row-action" data-action="pause" title="Pause">⏸</button>
          ` : d.status === 'Completed' ? `
            <button class="btn-row-action" data-action="open-file" title="Open File">📄</button>
          ` : `
            <button class="btn-row-action" data-action="resume" title="Resume">▶</button>
          `}
          <button class="btn-row-action" data-action="folder" title="Open Folder">📁</button>
          <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
          <button class="btn-row-action" data-action="inspect" title="Segments & Details">···</button>
        </div>
      </div>
    </div>
  `;
}

function attachRowEventListeners() {
  document.querySelectorAll('#downloads-tbody tr[data-id]').forEach(row => {
    const id = row.getAttribute('data-id');

    // Row selection on click
    row.addEventListener('click', (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      if (selectedTaskIds.has(id)) {
        selectedTaskIds.delete(id);
      } else {
        selectedTaskIds.add(id);
      }
      renderDownloads();
    });

    // Checkbox toggle
    row.querySelector('.row-checkbox')?.addEventListener('change', (e) => {
      if (e.target.checked) selectedTaskIds.add(id);
      else selectedTaskIds.delete(id);
      renderDownloads();
    });

    // Double click to open completed file or inspect segments
    row.addEventListener('dblclick', (e) => {
      if (e.target.closest('button, input')) return;
      const task = downloads.find(d => d.id === id);
      if (task?.status === 'Completed' && (task.finalPath || task.savePath)) {
        openFileOrFolder(task.finalPath || task.savePath, 'file');
      } else {
        openSegmentVisualizer(id);
      }
    });

    // Action buttons
    row.querySelector('[data-action="open-file"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      const task = downloads.find(d => d.id === id);
      if (task?.finalPath || task?.savePath) openFileOrFolder(task.finalPath || task.savePath, 'file');
    });
    row.querySelector('[data-action="pause"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'pause');
    });
    row.querySelector('[data-action="resume"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'resume');
    });
    row.querySelector('[data-action="folder"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      const task = downloads.find(d => d.id === id);
      if (task?.savePath) openFileOrFolder(task.savePath, 'folder');
    });
    row.querySelector('[data-action="delete"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'delete');
    });
    row.querySelector('[data-action="inspect"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      openSegmentVisualizer(id);
    });
  });
}

function attachGridEventListeners() {
  document.querySelectorAll('.grid-card[data-id]').forEach(card => {
    const id = card.getAttribute('data-id');

    card.addEventListener('click', (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      if (selectedTaskIds.has(id)) selectedTaskIds.delete(id);
      else selectedTaskIds.add(id);
      renderDownloads();
    });

    card.addEventListener('dblclick', (e) => {
      if (e.target.closest('button, input')) return;
      const task = downloads.find(d => d.id === id);
      if (task?.status === 'Completed' && (task.finalPath || task.savePath)) {
        openFileOrFolder(task.finalPath || task.savePath, 'file');
      } else {
        openSegmentVisualizer(id);
      }
    });

    card.querySelector('[data-action="open-file"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      const task = downloads.find(d => d.id === id);
      if (task?.finalPath || task?.savePath) openFileOrFolder(task.finalPath || task.savePath, 'file');
    });
    card.querySelector('[data-action="pause"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'pause');
    });
    card.querySelector('[data-action="resume"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'resume');
    });
    card.querySelector('[data-action="folder"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      const task = downloads.find(d => d.id === id);
      if (task?.savePath) openFileOrFolder(task.savePath, 'folder');
    });
    card.querySelector('[data-action="delete"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      downloadAction(id, 'delete');
    });
    card.querySelector('[data-action="inspect"]')?.addEventListener('click', (e) => {
      e.stopPropagation();
      openSegmentVisualizer(id);
    });
  });
}

function updateLiveRowElements() {
  downloads.forEach(d => {
    const percent = calculatePercent(d);
    const speedText = (d.status === 'Downloading' && d.speed > 0) ? formatSpeed(d.speed) : '—';
    const sizeText = formatSize(d.downloadedBytes || 0) + ' / ' + formatSize(d.totalSize || 0);
    const etaText = (d.status === 'Downloading' && d.eta > 0) ? formatETA(d.eta) : (d.status === 'Completed' ? 'Complete' : '—');

    // Update table row if present
    const row = document.querySelector(`tr[data-id="${d.id}"]`);
    if (row) {
      const percentEl = row.querySelector('.row-percent');
      if (percentEl) percentEl.textContent = `${percent}%`;

      const fillEl = row.querySelector('.progress-fill');
      if (fillEl) fillEl.style.width = `${percent}%`;

      const speedEl = row.querySelector('.row-speed');
      if (speedEl) speedEl.textContent = speedText;

      const sizeEl = row.querySelector('.row-size');
      if (sizeEl) sizeEl.textContent = sizeText;

      const etaEl = row.querySelector('.row-eta');
      if (etaEl) etaEl.textContent = etaText;
    }

    // Update grid card if present
    const card = document.querySelector(`.grid-card[data-id="${d.id}"]`);
    if (card) {
      const cardFill = card.querySelector('.progress-fill');
      if (cardFill) cardFill.style.width = `${percent}%`;

      const cardPercent = card.querySelector('.grid-card-percent');
      if (cardPercent) cardPercent.textContent = `${percent}%`;

      const cardSpeed = card.querySelector('.grid-card-speed');
      if (cardSpeed) cardSpeed.textContent = (d.status === 'Downloading' && d.speed > 0) ? speedText : (d.status === 'Completed' ? 'Complete' : '—');

      const cardSize = card.querySelector('.grid-card-size-info');
      if (cardSize) cardSize.textContent = sizeText;
    }
  });
}

function updateCategoryBadges() {
  const counts = { all: downloads.length, Music: 0, Videos: 0, Programs: 0, Documents: 0, Compressed: 0, Pictures: 0 };
  downloads.forEach(d => {
    if (counts[d.category] !== undefined) counts[d.category]++;
  });

  document.getElementById('count-all') && (document.getElementById('count-all').textContent = counts.all);
  document.getElementById('count-music') && (document.getElementById('count-music').textContent = counts.Music);
  document.getElementById('count-videos') && (document.getElementById('count-videos').textContent = counts.Videos);
  document.getElementById('count-programs') && (document.getElementById('count-programs').textContent = counts.Programs);
  document.getElementById('count-documents') && (document.getElementById('count-documents').textContent = counts.Documents);
  document.getElementById('count-compressed') && (document.getElementById('count-compressed').textContent = counts.Compressed);
  document.getElementById('count-pictures') && (document.getElementById('count-pictures').textContent = counts.Pictures);
}

// Queue Navigation List in Sidebar
function renderQueueNavList() {
  const list = document.getElementById('queue-nav-list');
  if (!list) return;

  list.innerHTML = '';
  queues.forEach(q => {
    const qCount = downloads.filter(d => d.queueId === q.id).length;
    const li = document.createElement('li');
    li.className = `nav-item ${currentQueueFilter === q.id ? 'active' : ''}`;
    li.setAttribute('data-queue', q.id);
    li.innerHTML = `
      <span class="queue-status-dot ${q.running ? 'active' : 'idle'}"></span>
      <span class="nav-label">${escapeHtml(q.name)}</span>
      <span class="badge">${qCount}</span>
    `;
    li.addEventListener('click', () => {
      document.querySelectorAll('#category-nav-list .nav-item').forEach(c => c.classList.remove('active'));
      document.querySelectorAll('#queue-nav-list .nav-item').forEach(qi => qi.classList.remove('active'));
      li.classList.add('active');
      currentQueueFilter = q.id;
      currentCategory = 'all';
      renderDownloads();
    });
    list.appendChild(li);
  });
}

function populateQueueDropdowns() {
  const selects = ['add-queue-select', 'setting-default-queue'];
  selects.forEach(sId => {
    const el = document.getElementById(sId);
    if (!el) return;
    el.innerHTML = '';
    queues.forEach(q => {
      const opt = document.createElement('option');
      opt.value = q.id;
      opt.textContent = q.name;
      el.appendChild(opt);
    });
  });
}

// Actions API
async function downloadAction(id, action) {
  try {
    const res = await fetch('/api/download/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, action })
    });
    if (res.ok) {
      if (action === 'delete' || action === 'delete_file') {
        downloads = downloads.filter(d => d.id !== id);
        renderDownloads();
      }
      showToast(`Action ${action} triggered`);
    }
  } catch (e) {
    console.error('Download action error:', e);
  }
}

async function setTaskSpeedLimit(id, speedLimit) {
  try {
    await fetch('/api/download/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, action: 'speed_limit', speedLimit })
    });
    showToast(speedLimit === 0 ? 'Speed: Unlimited' : `Speed Limit: ${formatSpeed(speedLimit)}`);
  } catch (e) {
    console.error('Speed limit error:', e);
  }
}

async function setTaskConns(id, maxConns) {
  try {
    await fetch('/api/download/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, action: 'set_conns', maxConns })
    });
    showToast(`Connections set to ${maxConns}`);
  } catch (e) {
    console.error('Conns error:', e);
  }
}

async function openFileOrFolder(path, type) {
  try {
    await fetch('/api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, type })
    });
  } catch (e) {
    console.error('Open error:', e);
  }
}

// Modals Management
function initModals() {
  // Add Download modal tab switcher
  document.querySelectorAll('.modal-nav-tabs .modal-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const tabId = btn.getAttribute('data-tab');
      switchAddTab(tabId);
    });
  });

  // Click on modal backdrop to dismiss
  document.querySelectorAll('.modal-overlay').forEach(overlay => {
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) {
        closeModal(overlay.id);
      }
    });
  });

  // Native Browse Folder Buttons
  setupBrowseFolderButton('btn-browse-add-dir', 'add-savedir-input');
  setupBrowseFolderButton('btn-browse-batch-dir', 'batch-savedir-input');
  setupBrowseFolderButton('btn-browse-default-dir', 'setting-default-dir');

  // Submit Add Download
  const btnSubmitAdd = document.getElementById('btn-submit-add-url');
  if (btnSubmitAdd) {
    btnSubmitAdd.addEventListener('click', () => submitAddDownload(true));
  }

  const btnQueueDownload = document.getElementById('btn-queue-download');
  if (btnQueueDownload) {
    btnQueueDownload.addEventListener('click', () => submitAddDownload(false));
  }

  // Site Grabber within Add Modal
  const btnStartCrawl = document.getElementById('btn-start-crawl');
  if (btnStartCrawl) btnStartCrawl.addEventListener('click', handleSiteGrabberCrawl);

  // Settings Save
  const btnSaveSettings = document.getElementById('btn-save-settings');
  if (btnSaveSettings) btnSaveSettings.addEventListener('click', saveSettingsFromModal);

  // Settings Reset
  const btnResetDefaults = document.getElementById('btn-reset-defaults');
  if (btnResetDefaults) btnResetDefaults.addEventListener('click', resetSettingsToDefaults);

  // Settings tab switching
  document.querySelectorAll('.settings-tabs-sidebar .settings-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.settings-tabs-sidebar .settings-tab-btn').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('.settings-panes-wrap .settings-pane').forEach(p => p.classList.remove('active'));

      btn.classList.add('active');
      const paneId = btn.getAttribute('data-tab');
      const pane = document.getElementById(paneId);
      if (pane) pane.classList.add('active');
    });
  });
}

function switchAddTab(tabId) {
  document.querySelectorAll('.modal-nav-tabs .modal-tab-btn').forEach(b => {
    b.classList.toggle('active', b.getAttribute('data-tab') === tabId);
  });
  document.querySelectorAll('#modal-add-download .modal-tab-pane').forEach(p => {
    p.classList.toggle('active', p.id === tabId);
  });
}

function setupBrowseFolderButton(btnId, targetInputId) {
  const btn = document.getElementById(btnId);
  const input = document.getElementById(targetInputId);
  if (!btn || !input) return;

  btn.addEventListener('click', async () => {
    try {
      const res = await fetch('/api/select_folder', { method: 'POST' });
      if (res.ok) {
        const data = await res.json();
        if (data.success && data.path) {
          input.value = data.path;
        }
      }
    } catch (e) {
      console.error('Folder selection error:', e);
    }
  });
}

async function submitAddDownload(startImmediately) {
  const activeTab = document.querySelector('#modal-add-download .modal-tab-pane.active')?.id;

  if (activeTab === 'tab-add-url') {
    const url = document.getElementById('add-url-input')?.value.trim();
    if (!url) {
      showToast('Please enter a valid URL');
      return;
    }
    const filename = document.getElementById('add-filename-input')?.value.trim();
    const saveDir = document.getElementById('add-savedir-input')?.value.trim();
    const queueId = document.getElementById('add-queue-select')?.value;
    const cookies = document.getElementById('add-cookies-input')?.value.trim();

    try {
      const res = await fetch('/api/download/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url, filename, saveDir, queueId, cookies })
      });
      if (res.ok) {
        closeModal('modal-add-download');
        document.getElementById('add-url-input').value = '';
        showToast('Download added successfully');
        loadInitialData();
      }
    } catch (e) {
      console.error('Add download error:', e);
    }
  } else if (activeTab === 'tab-add-batch') {
    const template = document.getElementById('batch-template-input')?.value.trim();
    const saveDir = document.getElementById('batch-savedir-input')?.value.trim();
    if (!template) return;

    try {
      const res = await fetch('/api/grabber/batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template, saveDir })
      });
      if (res.ok) {
        const data = await res.json();
        closeModal('modal-add-download');
        showToast(`Batch queued: ${data.count} items`);
        loadInitialData();
      }
    } catch (e) {
      console.error('Batch add error:', e);
    }
  }
}

// Site Grabber Crawler within Add Modal
async function handleSiteGrabberCrawl() {
  const url = document.getElementById('grabber-url')?.value.trim();
  const depth = parseInt(document.getElementById('grabber-depth')?.value, 10) || 1;
  const preset = document.getElementById('grabber-preset')?.value;
  const regex = document.getElementById('grabber-regex')?.value.trim();

  if (!url) {
    showToast('Please enter a target webpage URL');
    return;
  }

  showToast('Crawling webpage for links...');
  try {
    const res = await fetch('/api/grabber/crawl', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, depth, preset, regex })
    });
    if (res.ok) {
      const data = await res.json();
      renderGrabberResults(data.links || []);
    }
  } catch (e) {
    console.error('Crawl error:', e);
  }
}

function renderGrabberResults(links) {
  const tbody = document.getElementById('grabber-tbody');
  const countLbl = document.getElementById('grabber-count-label');
  if (countLbl) countLbl.textContent = `Found ${links.length} download links`;
  if (!tbody) return;

  tbody.innerHTML = links.map((l, i) => `
    <tr>
      <td style="text-align: center;"><input type="checkbox" class="grabber-item-check" value="${escapeHtml(l.url)}" checked></td>
      <td><strong>${escapeHtml(l.filename)}</strong></td>
      <td><span class="badge">${escapeHtml(l.category)}</span></td>
      <td style="max-width: 250px; overflow: hidden; text-overflow: ellipsis;">${escapeHtml(l.url)}</td>
    </tr>
  `).join('');
}

// Segment Visualizer Modal
function openSegmentVisualizer(taskId) {
  activeInspectId = taskId;
  const task = downloads.find(d => d.id === taskId);
  if (!task) return;

  renderSegmentModalData(task);
  openModal('modal-segment-view');
}

function renderSegmentModalData(task) {
  document.getElementById('seg-modal-title').textContent = `Properties: ${task.filename}`;
  document.getElementById('seg-modal-filename').textContent = task.url;
  document.getElementById('seg-modal-downloaded').textContent = `${formatSize(task.downloadedBytes || 0)} / ${formatSize(task.totalSize || 0)}`;
  document.getElementById('seg-modal-speed').textContent = formatSpeed(task.speed || 0);
  document.getElementById('seg-modal-eta').textContent = task.eta > 0 ? formatETA(task.eta) : '—';
  document.getElementById('seg-modal-conns').textContent = task.segments ? task.segments.filter(s => !s.done).length : 0;
  document.getElementById('seg-modal-percent').textContent = `${calculatePercent(task)}%`;

  // Visual segment bar
  const bar = document.getElementById('seg-visualizer-bar');
  if (bar && task.segments && task.totalSize > 0) {
    bar.innerHTML = task.segments.map(s => {
      const segWidth = ((s.endOffset - s.startOffset + 1) / task.totalSize) * 100;
      let colorClass = s.done ? 'color-done' : (s.active ? 'color-active' : 'color-unassigned');
      return `<div style="width: ${segWidth}%; height: 100%;" class="${colorClass}" title="Segment ${s.id}: ${formatSize(s.currentOffset - s.startOffset)} / ${formatSize(s.endOffset - s.startOffset + 1)}"></div>`;
    }).join('');
  }

  // Worker table
  const tbody = document.getElementById('worker-table-tbody');
  if (tbody && task.segments) {
    tbody.innerHTML = task.segments.map(s => {
      const segDone = s.done;
      const progress = s.endOffset > s.startOffset ? Math.min(100, Math.floor(((s.currentOffset - s.startOffset) / (s.endOffset - s.startOffset + 1)) * 100)) : 0;
      return `
        <tr>
          <td>#${s.id}</td>
          <td>${s.startOffset}</td>
          <td>${s.currentOffset}</td>
          <td>${s.endOffset}</td>
          <td>${progress}%</td>
          <td><span class="badge ${segDone ? 'badge-success' : 'badge-active'}">${segDone ? 'Done' : 'Downloading'}</span></td>
        </tr>
      `;
    }).join('');
  }
}

// Queue Manager Modal
async function openQueueManager() {
  try {
    const res = await fetch('/api/queues');
    if (res.ok) {
      const data = await res.json();
      queues = data.queues || [];
      renderQueueManagerUI();
      openModal('modal-queue-manager');
    }
  } catch (e) {
    console.error('Queue manager load error:', e);
  }
}

function renderQueueManagerUI() {
  const list = document.getElementById('queue-mgr-list');
  if (!list) return;

  list.innerHTML = '';
  queues.forEach((q, idx) => {
    const item = document.createElement('li');
    item.className = `queue-item-ui ${idx === 0 ? 'active' : ''}`;
    item.innerHTML = `
      <span>${escapeHtml(q.name)}</span>
      <span class="badge">${q.order ? q.order.length : 0}</span>
    `;
    item.addEventListener('click', () => {
      document.querySelectorAll('.queue-item-ui').forEach(i => i.classList.remove('active'));
      item.classList.add('active');
      renderQueueDetails(q);
    });
    list.appendChild(item);
  });

  if (queues.length > 0) {
    renderQueueDetails(queues[0]);
  }
}

function renderQueueDetails(q) {
  document.getElementById('queue-details-name').textContent = q.name;
  document.getElementById('queue-simultaneous-select').value = q.simultaneousDownloads || 1;
  document.getElementById('queue-oncomplete-select').value = q.onComplete || 'none';
  document.getElementById('queue-schedule-enabled').checked = !!q.scheduleEnabled;
  if (q.startTime) document.getElementById('queue-start-time').value = q.startTime;
  if (q.stopTime) document.getElementById('queue-stop-time').value = q.stopTime;

  // Render tasks order list
  const taskList = document.getElementById('queue-tasks-order-list');
  if (!taskList) return;
  taskList.innerHTML = '';

  if (q.order && q.order.length > 0) {
    q.order.forEach((taskId, index) => {
      const t = downloads.find(d => d.id === taskId);
      const name = t ? t.filename : taskId;

      const li = document.createElement('li');
      li.className = 'queue-order-item';
      li.innerHTML = `
        <span>${index + 1}. ${escapeHtml(name)}</span>
        <div class="order-btn-group">
          <button class="btn btn-xs" data-dir="up">▲</button>
          <button class="btn btn-xs" data-dir="down">▼</button>
        </div>
      `;

      li.querySelector('[data-dir="up"]')?.addEventListener('click', () => reorderTaskInQueue(q.id, index, -1));
      li.querySelector('[data-dir="down"]')?.addEventListener('click', () => reorderTaskInQueue(q.id, index, 1));
      taskList.appendChild(li);
    });
  } else {
    taskList.innerHTML = '<li class="queue-order-item" style="color: var(--text-dim);">No tasks assigned to this queue</li>';
  }

  // Hook Start / Stop buttons for this queue
  const btnStart = document.getElementById('btn-start-queue-active');
  const btnStop = document.getElementById('btn-stop-queue-active');
  if (btnStart) btnStart.onclick = () => startQueueAction(q.id);
  if (btnStop) btnStop.onclick = () => stopQueueAction(q.id);
}

async function startQueueAction(queueId) {
  try {
    await fetch('/api/queue/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: queueId })
    });
    showToast('Queue started');
    loadInitialData();
  } catch (e) {
    console.error('Start queue error:', e);
  }
}

async function stopQueueAction(queueId) {
  try {
    await fetch('/api/queue/stop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: queueId })
    });
    showToast('Queue stopped');
    loadInitialData();
  } catch (e) {
    console.error('Stop queue error:', e);
  }
}

async function reorderTaskInQueue(queueId, index, direction) {
  const q = queues.find(item => item.id === queueId);
  if (!q || !q.order) return;

  const targetIdx = index + direction;
  if (targetIdx < 0 || targetIdx >= q.order.length) return;

  const newOrder = [...q.order];
  const temp = newOrder[index];
  newOrder[index] = newOrder[targetIdx];
  newOrder[targetIdx] = temp;

  try {
    await fetch('/api/queue/reorder', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: queueId, order: newOrder })
    });
    q.order = newOrder;
    renderQueueDetails(q);
  } catch (e) {
    console.error('Reorder error:', e);
  }
}

// Settings Modal Management
async function openSettingsModal() {
  try {
    const res = await fetch('/api/settings');
    if (res.ok) {
      const settings = await res.json();
      populateSettingsForm(settings);
      openModal('modal-settings');
    }
  } catch (e) {
    console.error('Load settings error:', e);
  }
}

function populateSettingsForm(s) {
  document.getElementById('setting-autostart').checked = !!s.autoStart;
  document.getElementById('setting-autoopen').checked = !!s.autoOpenCompleted;
  document.getElementById('setting-sound').checked = !!s.soundAlertOnComplete;
  document.getElementById('setting-default-dir').value = s.defaultDownloadDir || '';
  document.getElementById('setting-duplicate-action').value = s.duplicateFileAction || 'rename';

  document.getElementById('setting-max-concurrent').value = s.maxConcurrentDownloads || 4;
  document.getElementById('setting-max-conns').value = s.maxConnectionsPerDownload || 8;
  document.getElementById('setting-buffer-size').value = s.networkBufferSize || 262144;
  document.getElementById('setting-timeout').value = s.connectTimeoutSeconds || 15;

  if (s.globalSpeedLimit > 0) {
    if (s.globalSpeedLimit >= 1048576) {
      document.getElementById('setting-speed-limit').value = Math.floor(s.globalSpeedLimit / 1048576);
      document.getElementById('setting-speed-unit').value = '1048576';
    } else {
      document.getElementById('setting-speed-limit').value = Math.floor(s.globalSpeedLimit / 1024);
      document.getElementById('setting-speed-unit').value = '1024';
    }
  } else {
    document.getElementById('setting-speed-limit').value = '0';
  }

  document.getElementById('setting-autofetch').checked = !!s.autoFetchEnabled;
  document.getElementById('setting-min-intercept-mb').value = s.minInterceptSizeBytes ? Math.floor(s.minInterceptSizeBytes / 1048576) : 1;
  document.getElementById('setting-intercept-exts').value = (s.interceptExtensions || []).join(', ');

  document.getElementById('setting-theme-picker').value = s.selectedTheme || 'nova';
  document.getElementById('setting-proxy-mode').value = s.proxyMode || 'direct';
  document.getElementById('setting-proxy-url').value = s.proxyUrl || '';

  // Render category folder inputs
  const catContainer = document.getElementById('category-folders-container');
  if (catContainer && s.categoryFolders) {
    catContainer.innerHTML = '';
    Object.keys(s.categoryFolders).forEach(cat => {
      const row = document.createElement('div');
      row.className = 'form-group';
      row.innerHTML = `
        <label>${cat} Folder:</label>
        <div class="input-with-button">
          <input type="text" data-cat="${cat}" value="${escapeHtml(s.categoryFolders[cat])}">
          <button class="btn btn-secondary btn-browse-cat" type="button">Browse...</button>
        </div>
      `;
      const inputEl = row.querySelector('input');
      row.querySelector('.btn-browse-cat')?.addEventListener('click', async () => {
        try {
          const res = await fetch('/api/select_folder', { method: 'POST' });
          if (res.ok) {
            const data = await res.json();
            if (data.success && data.path) inputEl.value = data.path;
          }
        } catch (e) {}
      });
      catContainer.appendChild(row);
    });
  }
}

async function saveSettingsFromModal() {
  const speedVal = parseInt(document.getElementById('setting-speed-limit').value, 10) || 0;
  const speedUnit = parseInt(document.getElementById('setting-speed-unit').value, 10) || 1048576;
  const globalSpeedLimit = speedVal * speedUnit;

  const catFolders = {};
  document.querySelectorAll('#category-folders-container input[data-cat]').forEach(inp => {
    catFolders[inp.getAttribute('data-cat')] = inp.value.trim();
  });

  const exts = document.getElementById('setting-intercept-exts').value
    .split(',')
    .map(e => e.trim().toLowerCase().replace(/^\./, ''))
    .filter(Boolean);

  const payload = {
    autoStart: document.getElementById('setting-autostart').checked,
    autoOpenCompleted: document.getElementById('setting-autoopen').checked,
    soundAlertOnComplete: document.getElementById('setting-sound').checked,
    defaultDownloadDir: document.getElementById('setting-default-dir').value.trim(),
    duplicateFileAction: document.getElementById('setting-duplicate-action').value,
    maxConcurrentDownloads: parseInt(document.getElementById('setting-max-concurrent').value, 10) || 4,
    maxConnectionsPerDownload: parseInt(document.getElementById('setting-max-conns').value, 10) || 8,
    networkBufferSize: parseInt(document.getElementById('setting-buffer-size').value, 10) || 262144,
    connectTimeoutSeconds: parseInt(document.getElementById('setting-timeout').value, 10) || 15,
    globalSpeedLimit,
    categoryFolders: catFolders,
    defaultQueueId: document.getElementById('setting-default-queue')?.value || 'main_queue',
    autoFetchEnabled: document.getElementById('setting-autofetch').checked,
    minInterceptSizeBytes: (parseInt(document.getElementById('setting-min-intercept-mb').value, 10) || 1) * 1048576,
    interceptExtensions: exts,
    selectedTheme: document.getElementById('setting-theme-picker').value,
    proxyMode: document.getElementById('setting-proxy-mode').value,
    proxyUrl: document.getElementById('setting-proxy-url').value.trim()
  };

  try {
    const res = await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    if (res.ok) {
      applyTheme(payload.selectedTheme, true);
      closeModal('modal-settings');
      showToast('Settings saved successfully');
    }
  } catch (e) {
    console.error('Save settings error:', e);
  }
}

function resetSettingsToDefaults() {
  if (confirm('Reset all settings to default configuration?')) {
    openSettingsModal();
    showToast('Settings restored');
  }
}

// Modal Helpers
function openModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.add('open');
}

function closeModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.remove('open');
  if (id === 'modal-segment-view') activeInspectId = null;
}

// Toast Notifications
function showToast(message, type = 'info', action = null) {
  const container = document.getElementById('toast-container');
  if (!container) return;

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;

  const iconMap = {
    info: '⚡',
    success: '✓',
    warning: '⚠️',
    danger: '✕'
  };
  const icon = iconMap[type] || '⚡';

  let actionHtml = '';
  if (action && action.text) {
    actionHtml = `<button class="btn btn-sm btn-primary toast-action-btn" type="button">${escapeHtml(action.text)}</button>`;
  }

  toast.innerHTML = `
    <span class="toast-icon">${icon}</span>
    <span class="toast-message">${escapeHtml(message)}</span>
    ${actionHtml}
  `;

  if (action && action.onClick) {
    const btn = toast.querySelector('.toast-action-btn');
    if (btn) {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        action.onClick();
        toast.remove();
      });
    }
  }

  container.appendChild(toast);

  const duration = action ? 5000 : 3000;
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(8px)';
    toast.style.transition = 'all 0.2s';
    setTimeout(() => toast.remove(), 200);
  }, duration);
}

// Utilities
function calculatePercent(d) {
  if (!d.totalSize || d.totalSize <= 0) return 0;
  return Math.min(100, Math.floor(((d.downloadedBytes || 0) / d.totalSize) * 100));
}

function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatSpeed(bytesPerSec) {
  const [num, unit] = formatSpeedParts(bytesPerSec);
  return `${num} ${unit}`;
}

function formatSpeedParts(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec <= 0) return ['0.0', 'KB/s'];
  if (bytesPerSec < 1048576) {
    return [(bytesPerSec / 1024).toFixed(1), 'KB/s'];
  }
  return [(bytesPerSec / 1048576).toFixed(1), 'MB/s'];
}

function formatETA(seconds) {
  if (!seconds || seconds <= 0) return '—';
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  const remM = m % 60;
  return `${h}h ${remM}m`;
}

function getFileIcon(filename, category) {
  switch (category) {
    case 'Music': return '🎵';
    case 'Videos': return '🎬';
    case 'Programs': return '💻';
    case 'Documents': return '📄';
    case 'Compressed': return '📦';
    case 'Pictures': return '🖼️';
    default: return '📁';
  }
}

function extractDomain(urlStr) {
  try {
    const parsed = new URL(urlStr);
    return parsed.hostname;
  } catch (e) {
    return urlStr || '';
  }
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
