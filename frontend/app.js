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
let lastSelectedId = null;
let currentRenewTaskId = null;
let pendingPauseTaskId = null;
let speedHistory = new Array(30).fill(0);
let bottomSpeedHistory = new Array(40).fill(0);
let speedCanvas, speedCtx;
let bottomSpeedCanvas, bottomSpeedCtx;
let activeViewMode = 'table'; // 'table' or 'grid'

document.addEventListener('DOMContentLoaded', () => {
  initTitlebarAndThemes();
  initNavigationAndToolbar();
  initQuickDownloadBar();
  initBentoBatchDock();
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
      if (e.target.closest('button, select, input, .search-pill, .bento-theme-switch, .theme-select-wrap, .window-controls')) return;
      windowAction('drag');
    });
    titlebar.addEventListener('dblclick', (e) => {
      if (e.target.closest('button, select, input, .search-pill, .bento-theme-switch, .theme-select-wrap, .window-controls')) return;
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

  // Bento Multi-Theme Switcher (Dark, Light, Midnight Obsidian, Forest Emerald)
  const themeBtnDark = document.getElementById('theme-btn-dark');
  const themeBtnLight = document.getElementById('theme-btn-light');
  const themeBtnMidnight = document.getElementById('theme-btn-midnight');
  const themeBtnEmerald = document.getElementById('theme-btn-emerald');
  const themeSelector = document.getElementById('theme-selector');
  const settingThemePicker = document.getElementById('setting-theme-picker');
  
  const validThemes = ['dark', 'light', 'midnight', 'emerald'];
  let savedTheme = localStorage.getItem('neodm-theme') || 'dark';
  if (!validThemes.includes(savedTheme)) {
    savedTheme = 'dark';
  }

  applyTheme(savedTheme, false);

  if (themeBtnDark) {
    themeBtnDark.addEventListener('click', () => applyTheme('dark', true));
  }
  if (themeBtnLight) {
    themeBtnLight.addEventListener('click', () => applyTheme('light', true));
  }
  if (themeBtnMidnight) {
    themeBtnMidnight.addEventListener('click', () => applyTheme('midnight', true));
  }
  if (themeBtnEmerald) {
    themeBtnEmerald.addEventListener('click', () => applyTheme('emerald', true));
  }
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
  const validThemes = ['dark', 'light', 'midnight', 'emerald'];
  if (!validThemes.includes(theme)) theme = 'dark';
  
  document.documentElement.setAttribute('data-theme', theme);
  document.documentElement.style.colorScheme = (theme === 'light') ? 'light' : 'dark';
  localStorage.setItem('neodm-theme', theme);

  const themeBtnDark = document.getElementById('theme-btn-dark');
  const themeBtnLight = document.getElementById('theme-btn-light');
  const themeBtnMidnight = document.getElementById('theme-btn-midnight');
  const themeBtnEmerald = document.getElementById('theme-btn-emerald');
  
  if (themeBtnDark) themeBtnDark.classList.toggle('active', theme === 'dark');
  if (themeBtnLight) themeBtnLight.classList.toggle('active', theme === 'light');
  if (themeBtnMidnight) themeBtnMidnight.classList.toggle('active', theme === 'midnight');
  if (themeBtnEmerald) themeBtnEmerald.classList.toggle('active', theme === 'emerald');

  const themeSelector = document.getElementById('theme-selector');
  if (themeSelector) themeSelector.value = theme;
  const settingThemePicker = document.getElementById('setting-theme-picker');
  if (settingThemePicker) settingThemePicker.value = theme;

  const names = {
    dark: '🌙 Bento Dark',
    light: '☀️ Bento Light',
    midnight: '🌌 Midnight Obsidian',
    emerald: '🌲 Forest Emerald'
  };
  if (notify) {
    showToast(`Theme changed: ${names[theme] || theme}`, 'info');
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
      btn.textContent = 'Checking...';

      const conflict = await resolveFileConflictIfNeeded(targetUrl, '', '');
      if (conflict.action === 'cancel') {
        btn.disabled = false;
        btn.textContent = 'Download Now';
        return;
      }

      btn.textContent = 'Starting...';

      const res = await fetch('/api/download/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: targetUrl,
          filename: conflict.filename || '',
          saveDir: '',
          queueName: 'main_queue',
          conflictAction: conflict.action
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
  // Category & Status nav
  const catItems = document.querySelectorAll('#category-nav-list .nav-item');
  catItems.forEach(item => {
    item.addEventListener('click', () => {
      catItems.forEach(i => i.classList.remove('active'));
      const queueItems = document.querySelectorAll('#queue-nav-list .nav-item');
      queueItems.forEach(q => q.classList.remove('active'));

      item.classList.add('active');
      const filter = item.getAttribute('data-filter');
      const cat = item.getAttribute('data-category');

      if (filter === 'downloading') {
        currentStatusFilter = 'Downloading';
        currentCategory = 'all';
      } else if (filter === 'completed') {
        currentStatusFilter = 'Completed';
        currentCategory = 'all';
      } else if (cat) {
        currentCategory = cat;
        currentStatusFilter = 'all';
      } else {
        currentCategory = 'all';
        currentStatusFilter = 'all';
      }
      currentQueueFilter = null;
      renderDownloads();
    });
  });

  // Tools nav
  const toolBrowser = document.getElementById('tool-browser-integration');
  if (toolBrowser) toolBrowser.addEventListener('click', () => {
    refreshBrowserExtensionInfo();
    openModal('modal-browser-integration');
  });

  const toolVideo = document.getElementById('tool-video-downloader');
  if (toolVideo) toolVideo.addEventListener('click', () => openVideoDownloaderModal());

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
        lastSelectedId = null;
      }
      renderDownloads();
      updateTelemetryDock();
      updateBentoBatchDock();
    });
  }
}

function updateBentoBatchDock() {
  const dock = document.getElementById('bento-batch-dock');
  const countEl = document.getElementById('batch-selected-count');
  if (!dock) return;
  if (selectedTaskIds.size > 0) {
    dock.style.display = 'flex';
    if (countEl) countEl.textContent = selectedTaskIds.size;
  } else {
    dock.style.display = 'none';
  }
}

function initBentoBatchDock() {
  document.getElementById('btn-batch-start')?.addEventListener('click', () => {
    if (selectedTaskIds.size > 0) batchAction('start_selected');
  });
  document.getElementById('btn-batch-pause')?.addEventListener('click', () => {
    if (selectedTaskIds.size > 0) batchAction('pause_selected');
  });
  document.getElementById('btn-batch-delete')?.addEventListener('click', () => {
    if (selectedTaskIds.size > 0) batchAction('delete_selected');
  });
  document.getElementById('btn-batch-clear')?.addEventListener('click', () => {
    selectedTaskIds.clear();
    lastSelectedId = null;
    renderDownloads();
    updateTelemetryDock();
    updateBentoBatchDock();
  });
}

async function batchAction(action, ids = null) {
  const targetIds = ids || Array.from(selectedTaskIds);
  if (action.endsWith('_selected') && targetIds.length === 0) {
    showToast('No downloads selected', 'warning');
    return;
  }

  // Optimistic UI updates for immediate tactile feedback
  if (action === 'pause_all') {
    downloads.forEach(d => {
      if (d.status === 'Downloading' || d.status === 'Connecting' || d.status === 'Queued') {
        d.status = 'Paused';
        d.speed = 0;
      }
    });
    renderDownloads();
    updateTelemetryDock();
  } else if (action === 'start_all') {
    downloads.forEach(d => {
      if (d.status === 'Paused' || d.status === 'Error' || d.status === 'Queued') {
        d.status = 'Downloading';
      }
    });
    renderDownloads();
    updateTelemetryDock();
  } else if (action === 'pause_selected') {
    downloads.forEach(d => {
      if (targetIds.includes(d.id) && (d.status === 'Downloading' || d.status === 'Connecting' || d.status === 'Queued')) {
        d.status = 'Paused';
        d.speed = 0;
      }
    });
    renderDownloads();
    updateTelemetryDock();
  } else if (action === 'start_selected') {
    downloads.forEach(d => {
      if (targetIds.includes(d.id)) {
        d.status = 'Downloading';
      }
    });
    renderDownloads();
    updateTelemetryDock();
  } else if (action === 'delete_completed') {
    downloads = downloads.filter(d => d.status !== 'Completed');
    renderDownloads();
    updateTelemetryDock();
  } else if (action === 'delete_selected') {
    downloads = downloads.filter(d => !targetIds.includes(d.id));
    selectedTaskIds.clear();
    lastSelectedId = null;
    renderDownloads();
    updateTelemetryDock();
    updateBentoBatchDock();
  } else if (action === 'delete_all' || action === 'delete_all_files') {
    downloads = [];
    selectedTaskIds.clear();
    lastSelectedId = null;
    renderDownloads();
    updateTelemetryDock();
    updateBentoBatchDock();
  }

  try {
    const res = await fetch('/api/download/batch_action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, ids: targetIds })
    });
    if (res.ok) {
      if (action.includes('delete')) {
        selectedTaskIds.clear();
        lastSelectedId = null;
      }
      const count = targetIds.length;
      const messages = {
        start_all: 'All downloads resumed',
        pause_all: 'All active downloads paused',
        start_selected: `Started ${count} selected download(s)`,
        pause_selected: `Paused ${count} selected download(s)`,
        delete_completed: 'Removed finished downloads from list',
        delete_selected: `Removed ${count} selected download(s)`,
        delete_all: 'All downloads removed from list',
        delete_all_files: 'All downloaded files deleted from disk'
      };
      showToast(messages[action] || 'Batch operation complete', action.includes('delete') ? 'info' : 'success');
      await loadInitialData();
      updateBentoBatchDock();
    }
  } catch (e) {
    console.error('Batch action error:', e);
  }
}

// Sparklines & Telemetry Canvases
const taskSpeedHistories = new Map();

function recordTaskSpeed(taskId, speed) {
  let hist = taskSpeedHistories.get(taskId);
  if (!hist) {
    hist = new Array(40).fill(0);
    taskSpeedHistories.set(taskId, hist);
  }
  hist.push(speed);
  hist.shift();
}

function initCanvases() {
  speedCanvas = document.getElementById('speed-canvas');
  if (speedCanvas) speedCtx = speedCanvas.getContext('2d');

  bottomSpeedCanvas = document.getElementById('bottom-speed-canvas');
  if (bottomSpeedCanvas) bottomSpeedCtx = bottomSpeedCanvas.getContext('2d');

  bottomSpeedHistory = new Array(40).fill(0);
  updateTelemetryDock();
}

function updateTelemetryDock() {
  const labelEl = document.getElementById('bottom-telemetry-label') || document.querySelector('.telemetry-speed-block .telemetry-label');
  const speedValEl = document.getElementById('telemetry-speed-val');

  if (selectedTaskIds.size === 0) {
    // Default: Total Speed across all active downloads
    if (labelEl) {
      labelEl.textContent = 'Total Speed';
      labelEl.title = 'Total download throughput across all active tasks';
    }
    const totalSpeed = downloads.reduce((acc, d) => acc + (d.status === 'Downloading' ? (d.speed || 0) : 0), 0);
    if (speedValEl) speedValEl.textContent = formatSpeed(totalSpeed);

    if (bottomSpeedCtx && bottomSpeedCanvas) {
      drawSplineArea(bottomSpeedCtx, bottomSpeedCanvas.width, bottomSpeedCanvas.height, bottomSpeedHistory, '#38bdf8', 'rgba(56, 189, 248, 0.28)');
    }
  } else if (selectedTaskIds.size === 1) {
    // Single selected download
    const selectedId = Array.from(selectedTaskIds)[0];
    const task = downloads.find(d => d.id === selectedId);
    if (labelEl) {
      if (task && task.filename) {
        const shortName = task.filename.length > 20 ? task.filename.substring(0, 18) + '…' : task.filename;
        labelEl.textContent = `Speed (${shortName})`;
        labelEl.title = `Speed of ${task.filename}`;
      } else {
        labelEl.textContent = 'Speed (Selected)';
        labelEl.title = 'Speed of selected download';
      }
    }
    const taskSpeed = task && task.status === 'Downloading' ? (task.speed || 0) : 0;
    if (speedValEl) speedValEl.textContent = formatSpeed(taskSpeed);

    let hist = taskSpeedHistories.get(selectedId);
    if (!hist) {
      hist = new Array(40).fill(taskSpeed);
      taskSpeedHistories.set(selectedId, hist);
    }
    if (bottomSpeedCtx && bottomSpeedCanvas) {
      drawSplineArea(bottomSpeedCtx, bottomSpeedCanvas.width, bottomSpeedCanvas.height, hist, '#38bdf8', 'rgba(56, 189, 248, 0.28)');
    }
  } else {
    // Multiple selected downloads
    if (labelEl) {
      labelEl.textContent = `Speed (${selectedTaskIds.size} Selected)`;
      labelEl.title = `Combined speed of ${selectedTaskIds.size} selected downloads`;
    }
    let combinedSpeed = 0;
    const combinedHist = new Array(40).fill(0);

    selectedTaskIds.forEach(id => {
      const task = downloads.find(d => d.id === id);
      if (task && task.status === 'Downloading') {
        combinedSpeed += (task.speed || 0);
      }
      const hist = taskSpeedHistories.get(id);
      if (hist) {
        for (let i = 0; i < 40; i++) {
          combinedHist[i] += (hist[i] || 0);
        }
      }
    });

    if (speedValEl) speedValEl.textContent = formatSpeed(combinedSpeed);
    if (bottomSpeedCtx && bottomSpeedCanvas) {
      drawSplineArea(bottomSpeedCtx, bottomSpeedCanvas.width, bottomSpeedCanvas.height, combinedHist, '#38bdf8', 'rgba(56, 189, 248, 0.28)');
    }
  }
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

  // If a single task is selected, push its current speed to keep its sparkline moving smoothly
  if (selectedTaskIds.size === 1) {
    const selId = Array.from(selectedTaskIds)[0];
    const task = downloads.find(d => d.id === selId);
    const selSpeed = (task && task.status === 'Downloading') ? (task.speed || 0) : 0;
    recordTaskSpeed(selId, selSpeed);
  }

  updateTelemetryDock();
}

function drawSplineArea(ctx, width, height, data, strokeColor, fillColor) {
  ctx.clearRect(0, 0, width, height);
  const maxVal = Math.max(...data, 1024 * 1024); // at least 1MB/s scale

  const step = width / (data.length - 1);
  const points = data.map((val, idx) => ({
    x: idx * step,
    y: height - (val / maxVal) * (height - 8) - 4
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

  // Stroke line with subtle glow
  ctx.save();
  ctx.strokeStyle = strokeColor;
  ctx.lineWidth = 2.5;
  ctx.shadowColor = strokeColor;
  ctx.shadowBlur = 8;
  ctx.stroke();
  ctx.restore();

  // Fill gradient underneath
  ctx.lineTo(width, height);
  ctx.lineTo(0, height);
  ctx.closePath();

  const grad = ctx.createLinearGradient(0, 0, 0, height);
  grad.addColorStop(0, fillColor);
  grad.addColorStop(1, 'rgba(0, 112, 243, 0)');
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
  // Global Ctrl+V / Ctrl+N / Ctrl+F / Ctrl+A / Escape
  window.addEventListener('keydown', async (e) => {
    // Escape key closes open menus and modals or clears selection
    if (e.key === 'Escape') {
      const moreActionsMenu = document.getElementById('more-actions-menu');
      if (moreActionsMenu) moreActionsMenu.classList.remove('open');
      const ctxMenu = document.getElementById('custom-context-menu');
      if (ctxMenu) ctxMenu.style.display = 'none';
      document.querySelectorAll('.modal-overlay.open').forEach(m => m.classList.remove('open'));

      if (selectedTaskIds.size > 0) {
        selectedTaskIds.clear();
        lastSelectedId = null;
        renderDownloads();
        updateTelemetryDock();
        updateBentoBatchDock();
      }
      return;
    }

    // Master Select-All (Ctrl+A)
    if ((e.ctrlKey || e.metaKey) && (e.key === 'a' || e.key === 'A')) {
      const activeEl = document.activeElement;
      const tag = activeEl ? activeEl.tagName : '';
      const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || activeEl?.isContentEditable;
      if (!isInput) {
        e.preventDefault();
        const filtered = getFilteredDownloads();
        filtered.forEach(d => selectedTaskIds.add(d.id));
        renderDownloads();
        updateTelemetryDock();
        updateBentoBatchDock();
      }
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

    // Populate dynamic queue choices first so the height is accurately calculated
    renderContextQueueList();

    // Measure exact rendered geometry offscreen to avoid clipping
    ctxMenu.style.visibility = 'hidden';
    ctxMenu.style.display = 'block';

    const menuRect = ctxMenu.getBoundingClientRect();
    const menuWidth = menuRect.width || 215;
    const menuHeight = menuRect.height || 470;

    let posX = e.clientX;
    let posY = e.clientY;

    // Horizontal bounds check
    if (posX + menuWidth > window.innerWidth - 12) {
      posX = window.innerWidth - menuWidth - 12;
    }
    if (posX < 8) posX = 8;

    // Vertical bounds check: if overflowing bottom of viewport, flip upwards or clamp safely
    if (posY + menuHeight > window.innerHeight - 12) {
      const flipY = e.clientY - menuHeight;
      if (flipY >= 10) {
        posY = flipY;
      } else {
        posY = Math.max(10, window.innerHeight - menuHeight - 12);
      }
    }

    ctxMenu.style.left = `${Math.round(posX)}px`;
    ctxMenu.style.top = `${Math.round(posY)}px`;
    ctxMenu.style.visibility = 'visible';
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
  document.getElementById('ctx-renew-url')?.addEventListener('click', () => {
    if (selectedCtxTask) openRenewUrlModal(selectedCtxTask);
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

  // Handle intelligent submenu collision avoidance (flips up/left if near screen edges)
  document.querySelectorAll('.ctx-has-sub').forEach(item => {
    item.addEventListener('mouseenter', () => {
      const sub = item.querySelector('.ctx-submenu');
      if (sub) {
        sub.style.display = 'block';
        const subRect = sub.getBoundingClientRect();
        if (subRect.bottom > window.innerHeight - 12) {
          sub.style.top = 'auto';
          sub.style.bottom = '-5px';
        } else {
          sub.style.top = '-5px';
          sub.style.bottom = 'auto';
        }
        if (subRect.right > window.innerWidth - 12) {
          sub.style.left = 'auto';
          sub.style.right = '100%';
        } else {
          sub.style.left = '100%';
          sub.style.right = 'auto';
        }
      }
    });
    item.addEventListener('mouseleave', () => {
      const sub = item.querySelector('.ctx-submenu');
      if (sub) {
        sub.style.display = '';
        sub.style.top = '';
        sub.style.bottom = '';
        sub.style.left = '';
        sub.style.right = '';
      }
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
  if (!msg) return;
  const data = msg.data !== undefined ? msg.data : msg.payload;

  if (msg.type === 'telemetry') {
    handleTelemetry(data || msg);
  } else if (msg.type === 'task_added') {
    const task = data;
    if (task && task.id) {
      downloads = downloads.filter(d => d.id && !d.id.startsWith('demo-') && !d.id.startsWith('showcase-'));
      const existingIdx = downloads.findIndex(d => d.id === task.id);
      if (existingIdx !== -1) {
        downloads[existingIdx] = { ...downloads[existingIdx], ...task };
      } else {
        downloads.unshift(task);
      }
      localStorage.setItem('neodm-real-downloads-seen', 'true');
      renderDownloads();
      updateCategoryBadges();
      updateTelemetryDock();
    }
  } else if (msg.type === 'task_updated') {
    const task = data;
    if (task && task.id) {
      downloads = downloads.filter(d => d.id && !d.id.startsWith('demo-') && !d.id.startsWith('showcase-'));
      const existingIdx = downloads.findIndex(d => d.id === task.id);
      if (existingIdx !== -1) {
        downloads[existingIdx] = { ...downloads[existingIdx], ...task };
      } else {
        downloads.push(task);
      }
      recordTaskSpeed(task.id, task.status === 'Downloading' ? (task.speed || 0) : 0);
      updateLiveRowElements();
      updateTelemetryDock();
      if (activeInspectId === task.id) {
        renderSegmentModalData(downloads[existingIdx !== -1 ? existingIdx : downloads.length - 1]);
      }
    }
  } else if (msg.type === 'task_completed') {
    const task = data;
    if (task && task.id) {
      const existingIdx = downloads.findIndex(d => d.id === task.id);
      if (existingIdx !== -1) {
        downloads[existingIdx] = { ...downloads[existingIdx], ...task, status: 'Completed', speed: 0, eta: 0 };
      }
      recordTaskSpeed(task.id, 0);
      renderDownloads();
      updateCategoryBadges();
      updateTelemetryDock();
      showToast(`Finished: ${task.filename || 'Download completed'}`, 'success');
    }
  } else if (msg.type === 'task_deleted') {
    const delId = (data && data.id) || (typeof data === 'string' ? data : null);
    if (delId) {
      downloads = downloads.filter(d => d.id !== delId);
      selectedTaskIds.delete(delId);
      taskSpeedHistories.delete(delId);
      renderDownloads();
      updateCategoryBadges();
      updateTelemetryDock();
    }
  } else if (msg.type === 'clipboard_url') {
    const raw = typeof data === 'string' ? data : (data?.url || '');
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

  // Extract tasks array safely whether payload is direct array or wrapped
  const taskList = Array.isArray(data) ? data : (data.tasks || data.payload || data.downloads || []);

  // Filter out any fake demo tasks completely
  const hadDemo = downloads.some(d => d.id && (d.id.startsWith('demo-') || d.id.startsWith('showcase-')));
  if (hadDemo) {
    downloads = downloads.filter(d => d.id && !d.id.startsWith('demo-') && !d.id.startsWith('showcase-'));
  }

  // Reconcile local downloads array against authoritative backend taskList
  const backendIds = new Set(taskList.map(t => t.id));
  const prevCount = downloads.length;

  // 1. Prune tasks that no longer exist on backend (e.g. deleted or cleared)
  downloads = downloads.filter(d => backendIds.has(d.id));
  let structureChanged = hadDemo || (downloads.length !== prevCount);

  // 2. Update existing or append newly discovered tasks from backend
  taskList.forEach(updated => {
    recordTaskSpeed(updated.id, updated.status === 'Downloading' ? (updated.speed || 0) : 0);
    const idx = downloads.findIndex(d => d.id === updated.id);
    if (idx !== -1) {
      downloads[idx] = { ...downloads[idx], ...updated };
    } else {
      downloads.push(updated);
      structureChanged = true;
    }
  });

  if (structureChanged) {
    renderDownloads();
    updateCategoryBadges();
  }

  // Calculate totals
  let totalSpeed = 0;
  if (typeof data.totalSpeed === 'number') {
    totalSpeed = data.totalSpeed;
  } else if (typeof data.globalSpeed === 'number') {
    totalSpeed = data.globalSpeed;
  } else {
    totalSpeed = downloads.reduce((acc, d) => acc + (d.status === 'Downloading' ? (d.speed || 0) : 0), 0);
  }

  updateThroughputCharts(totalSpeed);

  // Update counts
  const activeCountEl = document.getElementById('telemetry-active-count');
  const activeCount = typeof data.activeDownloads === 'number'
    ? data.activeDownloads
    : downloads.filter(d => d.status === 'Downloading' || d.status === 'Connecting').length;
  if (activeCountEl) activeCountEl.textContent = activeCount;

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
  if (bentoActive) bentoActive.textContent = activeCount;

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

const INITIAL_DEMO_DOWNLOADS = [
  {
    id: 'demo-1',
    filename: 'ubuntu-24.04.2-desktop-amd64.iso',
    status: 'Downloading',
    percent: 62,
    downloadedBytes: 2800000000,
    totalSize: 4500000000,
    speed: 12400000,
    eta: 192,
    timeLeft: '3m 12s',
    category: 'Documents',
    url: 'https://releases.ubuntu.com/24.04.2/ubuntu-24.04.2-desktop-amd64.iso',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-2',
    filename: 'Adobe.Photoshop.2025.exe',
    status: 'Downloading',
    percent: 34,
    downloadedBytes: 1200000000,
    totalSize: 3400000000,
    speed: 5600000,
    eta: 401,
    timeLeft: '6m 41s',
    category: 'Programs',
    url: 'https://creativecloud.adobe.com/downloads/Adobe.Photoshop.2025.exe',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-3',
    filename: 'Interstellar.2014.1080p.BluRay.x264.mkv',
    status: 'Downloading',
    percent: 78,
    downloadedBytes: 5600000000,
    totalSize: 7100000000,
    speed: 18900000,
    eta: 63,
    timeLeft: '1m 03s',
    category: 'Videos',
    url: 'https://archive.org/download/interstellar/Interstellar.2014.1080p.BluRay.x264.mkv',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-4',
    filename: 'FigmaSetup.exe',
    status: 'Queued',
    percent: 0,
    downloadedBytes: 0,
    totalSize: 118 * 1024 * 1024,
    speed: 0,
    eta: 0,
    timeLeft: '—',
    category: 'Programs',
    url: 'https://desktop.figma.com/win/FigmaSetup.exe',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-5',
    filename: 'Clarity-Wallpapers.zip',
    status: 'Completed',
    percent: 100,
    downloadedBytes: 432 * 1024 * 1024,
    totalSize: 432 * 1024 * 1024,
    speed: 0,
    eta: 0,
    timeLeft: 'Yesterday',
    completedTime: 'Yesterday',
    category: 'Compressed',
    url: 'https://wallpapers.clarity.io/Clarity-Wallpapers.zip',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-6',
    filename: 'React-Guide.pdf',
    status: 'Completed',
    percent: 100,
    downloadedBytes: 12.4 * 1024 * 1024,
    totalSize: 12.4 * 1024 * 1024,
    speed: 0,
    eta: 0,
    timeLeft: 'Yesterday',
    completedTime: 'Yesterday',
    category: 'Documents',
    url: 'https://react.dev/docs/React-Guide.pdf',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-7',
    filename: 'Music Collection.zip',
    status: 'Completed',
    percent: 100,
    downloadedBytes: 2.1 * 1024 * 1024 * 1024,
    totalSize: 2.1 * 1024 * 1024 * 1024,
    speed: 0,
    eta: 0,
    timeLeft: '2 days ago',
    completedTime: '2 days ago',
    category: 'Compressed',
    url: 'https://archive.org/download/music_collection/Music%20Collection.zip',
    queueId: 'main_queue',
    supportsRange: true
  },
  {
    id: 'demo-8',
    filename: 'Sample Video.mp4',
    status: 'Completed',
    percent: 100,
    downloadedBytes: 700 * 1024 * 1024,
    totalSize: 700 * 1024 * 1024,
    speed: 0,
    eta: 0,
    timeLeft: '3 days ago',
    completedTime: '3 days ago',
    category: 'Videos',
    url: 'https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/Sample%20Video.mp4',
    queueId: 'main_queue',
    supportsRange: true
  }
];

const SHOWCASE_DOWNLOADS = INITIAL_DEMO_DOWNLOADS;

function getFileTypeLabel(filename, category) {
  const name = (filename || '').toLowerCase();
  if (name.endsWith('.iso')) return 'ISO Image';
  if (name.endsWith('.dmg')) return 'Disk Image';
  if (name.endsWith('.pdf')) return 'Document';
  if (name.endsWith('.exe') || name.endsWith('.msi')) return 'Application';
  if (name.endsWith('.zip') || name.endsWith('.rar') || name.endsWith('.7z')) return 'Archive';
  if (name.endsWith('.mp4') || name.endsWith('.mkv') || name.endsWith('.avi')) return 'Video';
  if (name.endsWith('.mp3') || name.endsWith('.wav') || name.endsWith('.flac')) return 'Audio';
  if (category) return category;
  return 'File';
}

// Global helper to safely normalize video/audio filename extensions without duplicate extensions (.mp4.mp4)
function formatMediaFilename(name, isAudio) {
  let clean = (name || 'video').trim();
  clean = clean.replace(/\.(mp4|mkv|webm|avi|flv|mov|mp3|m4a|wav|aac)+$/gi, '');
  clean = clean.replace(/\.(mp4|mkv|webm|avi|flv|mov|mp3|m4a|wav|aac)$/gi, '');
  clean = clean.trim();
  if (!clean) clean = isAudio ? 'audio_track' : 'video';
  return isAudio ? `${clean}.mp3` : `${clean}.mp4`;
}

// Initial Data Load
async function loadInitialData() {
  refreshBrowserExtensionInfo();
  try {
    const [dRes, qRes] = await Promise.all([
      fetch('/api/downloads'),
      fetch('/api/queues')
    ]);

    if (dRes.ok) {
      const dJson = await dRes.json();
      const backendDownloads = dJson.downloads || [];
      // Use real backend downloads; purge any demo tasks in production
      downloads = backendDownloads.filter(d => d.id && !d.id.startsWith('demo-') && !d.id.startsWith('showcase-'));
      renderDownloads();
      updateCategoryBadges();
      updateTelemetryDock();
      updateBentoBatchDock();
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
window.fetchDownloads = loadInitialData;

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
      if (currentStatusFilter === 'Downloading') {
        if (d.status !== 'Downloading' && d.status !== 'Connecting' && d.status !== 'Queued') return false;
      } else if (d.status !== currentStatusFilter) {
        return false;
      }
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
    const checkAll = document.getElementById('check-all-downloads');
    if (checkAll) {
      checkAll.checked = false;
      checkAll.indeterminate = false;
    }
    updateBentoBatchDock();
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

  // Update master select-all checkbox
  const checkAll = document.getElementById('check-all-downloads');
  if (checkAll) {
    const allSelected = filtered.length > 0 && filtered.every(d => selectedTaskIds.has(d.id));
    const someSelected = filtered.some(d => selectedTaskIds.has(d.id));
    checkAll.checked = allSelected;
    checkAll.indeterminate = !allSelected && someSelected;
  }

  updateBentoBatchDock();
}

function formatRowSpeed(d) {
  if (d.status === 'Downloading') {
    return (d.speed && d.speed > 0) ? formatSpeed(d.speed) : '0 B/s';
  }
  if (d.status === 'Queued') {
    return '0 B/s';
  }
  return '';
}

function formatRowSize(d) {
  if (d.status === 'Completed') {
    return formatSize(d.totalSize || d.downloadedBytes || 0);
  }
  const downloaded = (d.downloadedBytes === 0) ? '0' : formatSize(d.downloadedBytes || 0);
  const total = formatSize(d.totalSize || 0);
  return `${downloaded} / ${total}`;
}

function formatRowTimeLeft(d) {
  if (d.timeLeft) return d.timeLeft;
  if (d.status === 'Downloading') {
    return d.eta > 0 ? formatETA(d.eta) : '—';
  }
  if (d.status === 'Queued') {
    return '—';
  }
  if (d.status === 'Completed') {
    return d.completedTime || 'Yesterday';
  }
  return '—';
}

function renderTableRow(d) {
  const isSelected = selectedTaskIds.has(d.id);
  const percent = calculatePercent(d);
  const speedText = formatRowSpeed(d);
  const sizeText = formatRowSize(d);
  const etaText = formatRowTimeLeft(d);
  const icon = getFileIcon(d.filename, d.category);
  const supportsRange = d.supportsRange !== false;
  const resumeBadge = supportsRange
    ? `<span class="resumable-badge is-resumable" title="Server supports range requests (Resumable)">✓ Resume</span>`
    : `<span class="resumable-badge is-non-resumable" title="Server does not support range requests (Non-resumable)">⚠ No Resume</span>`;

  let actionBtns = '';
  if (d.status === 'Downloading') {
    actionBtns = `
      <button class="btn-row-action" data-action="pause" title="Pause">⏸</button>
      <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
      <button class="btn-row-action" data-action="inspect" title="Details">⋮</button>
    `;
  } else if (d.status === 'Queued') {
    actionBtns = `
      <button class="btn-row-action" data-action="resume" title="Start">▶</button>
      <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
      <button class="btn-row-action" data-action="inspect" title="Details">⋮</button>
    `;
  } else if (d.status === 'Completed') {
    actionBtns = `
      <button class="btn-row-action" data-action="folder" title="Open Folder">📁</button>
      <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
      <button class="btn-row-action" data-action="inspect" title="Details">⋮</button>
    `;
  } else {
    actionBtns = `
      <button class="btn-row-action" data-action="resume" title="Resume">▶</button>
      <button class="btn-row-action action-delete" data-action="delete" title="Delete">✕</button>
      <button class="btn-row-action" data-action="inspect" title="Details">⋮</button>
    `;
  }

  return `
    <tr data-id="${d.id}" class="${isSelected ? 'selected' : ''}">
      <td style="text-align: center;">
        <input type="checkbox" class="row-checkbox table-checkbox" ${isSelected ? 'checked' : ''} data-id="${d.id}">
      </td>
      <td>
        <div class="file-cell">
          ${icon}
          <span class="file-title" title="${escapeHtml(d.filename)}">${escapeHtml(d.filename)}</span>
        </div>
      </td>
      <td>
        <span class="status-text status-${d.status.toLowerCase()}">${d.status}</span>
      </td>
      <td style="text-align: center;">
        ${resumeBadge}
      </td>
      <td>
        <div class="progress-cell-inline">
          <div class="progress-track">
            <div class="progress-fill ${d.status.toLowerCase()}" style="width: ${percent}%;"></div>
          </div>
          <span class="row-percent">${percent}%</span>
        </div>
      </td>
      <td><span class="row-speed">${speedText}</span></td>
      <td><span class="row-size">${sizeText}</span></td>
      <td><span class="row-eta">${etaText}</span></td>
      <td style="text-align: right;">
        <div class="row-actions">
          ${actionBtns}
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
  const supportsRange = d.supportsRange !== false;
  const resumeBadge = supportsRange
    ? `<span class="resumable-badge is-resumable" title="Server supports range requests (Resumable)">✓ Resume</span>`
    : `<span class="resumable-badge is-non-resumable" title="Server does not support range requests (Non-resumable)">⚠ No Resume</span>`;

  return `
    <div class="grid-card ${isSelected ? 'selected' : ''}" data-id="${d.id}">
      <div class="grid-card-header">
        <div class="grid-card-identity">
          <input type="checkbox" class="row-checkbox table-checkbox" ${isSelected ? 'checked' : ''} data-id="${d.id}" style="margin-right: 6px;">
          <div class="grid-card-icon">${icon}</div>
          <div class="grid-card-title-wrap">
            <h4 class="grid-card-title" title="${escapeHtml(d.filename)}">${escapeHtml(d.filename)}</h4>
            <span class="grid-card-domain">${escapeHtml(domain)}</span>
          </div>
        </div>
        <div style="display: flex; align-items: center; gap: 6px;">
          ${resumeBadge}
          <span class="status-pill status-${d.status.toLowerCase()}">${d.status}</span>
        </div>
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

    // Row selection on click with Shift range and Ctrl multi-select
    row.addEventListener('click', (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      const filtered = getFilteredDownloads();
      const currentIdx = filtered.findIndex(d => d.id === id);

      if (e.shiftKey && lastSelectedId) {
        const lastIdx = filtered.findIndex(d => d.id === lastSelectedId);
        if (lastIdx !== -1 && currentIdx !== -1) {
          const start = Math.min(lastIdx, currentIdx);
          const end = Math.max(lastIdx, currentIdx);
          if (!e.ctrlKey && !e.metaKey) {
            selectedTaskIds.clear();
          }
          for (let i = start; i <= end; i++) {
            selectedTaskIds.add(filtered[i].id);
          }
        }
      } else if (e.ctrlKey || e.metaKey) {
        if (selectedTaskIds.has(id)) {
          selectedTaskIds.delete(id);
        } else {
          selectedTaskIds.add(id);
          lastSelectedId = id;
        }
      } else {
        if (selectedTaskIds.has(id) && selectedTaskIds.size === 1) {
          selectedTaskIds.clear();
          lastSelectedId = null;
        } else {
          selectedTaskIds.clear();
          selectedTaskIds.add(id);
          lastSelectedId = id;
        }
      }
      renderDownloads();
      updateTelemetryDock();
      updateBentoBatchDock();
    });

    // Checkbox toggle
    row.querySelector('.row-checkbox')?.addEventListener('change', (e) => {
      if (e.target.checked) {
        selectedTaskIds.add(id);
        lastSelectedId = id;
      } else {
        selectedTaskIds.delete(id);
      }
      renderDownloads();
      updateTelemetryDock();
      updateBentoBatchDock();
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
      const filtered = getFilteredDownloads();
      const currentIdx = filtered.findIndex(d => d.id === id);

      if (e.shiftKey && lastSelectedId) {
        const lastIdx = filtered.findIndex(d => d.id === lastSelectedId);
        if (lastIdx !== -1 && currentIdx !== -1) {
          const start = Math.min(lastIdx, currentIdx);
          const end = Math.max(lastIdx, currentIdx);
          if (!e.ctrlKey && !e.metaKey) {
            selectedTaskIds.clear();
          }
          for (let i = start; i <= end; i++) {
            selectedTaskIds.add(filtered[i].id);
          }
        }
      } else if (e.ctrlKey || e.metaKey) {
        if (selectedTaskIds.has(id)) {
          selectedTaskIds.delete(id);
        } else {
          selectedTaskIds.add(id);
          lastSelectedId = id;
        }
      } else {
        if (selectedTaskIds.has(id) && selectedTaskIds.size === 1) {
          selectedTaskIds.clear();
          lastSelectedId = null;
        } else {
          selectedTaskIds.clear();
          selectedTaskIds.add(id);
          lastSelectedId = id;
        }
      }
      renderDownloads();
      updateTelemetryDock();
      updateBentoBatchDock();
    });

    card.querySelector('.row-checkbox')?.addEventListener('change', (e) => {
      if (e.target.checked) {
        selectedTaskIds.add(id);
        lastSelectedId = id;
      } else {
        selectedTaskIds.delete(id);
      }
      renderDownloads();
      updateTelemetryDock();
      updateBentoBatchDock();
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
      const statusEl = row.querySelector('.status-text');
      if (statusEl && statusEl.textContent !== d.status) {
        statusEl.className = `status-text status-${d.status.toLowerCase()}`;
        statusEl.textContent = d.status;
      }

      const percentEl = row.querySelector('.row-percent');
      if (percentEl) percentEl.textContent = `${percent}%`;

      const fillEl = row.querySelector('.progress-fill');
      if (fillEl) {
        fillEl.style.width = `${percent}%`;
        fillEl.className = `progress-fill ${d.status.toLowerCase()}`;
      }

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
      if (cardFill) {
        cardFill.style.width = `${percent}%`;
        cardFill.className = `progress-fill ${d.status.toLowerCase()}`;
      }

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
  const counts = {
    all: downloads.length,
    downloading: downloads.filter(d => d.status === 'Downloading' || d.status === 'Connecting' || d.status === 'Queued').length,
    completed: downloads.filter(d => d.status === 'Completed').length,
    Videos: downloads.filter(d => d.category === 'Videos').length,
    Music: downloads.filter(d => d.category === 'Music').length,
    Documents: downloads.filter(d => d.category === 'Documents').length,
    Programs: downloads.filter(d => d.category === 'Programs').length,
    Compressed: downloads.filter(d => d.category === 'Compressed').length,
    Pictures: downloads.filter(d => d.category === 'Pictures').length,
    Other: downloads.filter(d => d.category === 'Other').length
  };

  document.getElementById('count-all') && (document.getElementById('count-all').textContent = counts.all);
  document.getElementById('count-downloading') && (document.getElementById('count-downloading').textContent = counts.downloading);
  document.getElementById('count-completed') && (document.getElementById('count-completed').textContent = counts.completed);
  document.getElementById('count-video') && (document.getElementById('count-video').textContent = counts.Videos);
  document.getElementById('count-audio') && (document.getElementById('count-audio').textContent = counts.Music);
  document.getElementById('count-documents') && (document.getElementById('count-documents').textContent = counts.Documents);
  document.getElementById('count-programs') && (document.getElementById('count-programs').textContent = counts.Programs);
  document.getElementById('count-compressed') && (document.getElementById('count-compressed').textContent = counts.Compressed);
  document.getElementById('count-pictures') && (document.getElementById('count-pictures').textContent = counts.Pictures);
  document.getElementById('count-other') && (document.getElementById('count-other').textContent = counts.Other);
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

// Actions & Renewal API
function openRenewUrlModal(task) {
  if (!task) return;
  currentRenewTaskId = task.id;
  const filenameEl = document.getElementById('renew-filename');
  const urlEl = document.getElementById('renew-url-input');
  if (filenameEl) filenameEl.value = task.filename || '';
  if (urlEl) {
    urlEl.value = task.url || '';
    setTimeout(() => {
      urlEl.focus();
      urlEl.select();
    }, 120);
  }
  openModal('modal-renew-url');
}

async function executePauseDirect(id) {
  const task = downloads.find(d => d.id === id);
  const taskName = task ? `'${task.filename}'` : 'Download';
  try {
    const res = await fetch('/api/download/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, action: 'pause' })
    });
    if (res.ok) {
      showToast(`Paused non-resumable download: ${taskName}`, 'warning');
      loadInitialData();
    }
  } catch (err) {
    console.error('Pause non-resumable error:', err);
  }
}

async function downloadAction(id, action) {
  const task = downloads.find(d => d.id === id);
  const taskName = task ? `'${task.filename}'` : 'Download';

  // Smart Resumability check: if pausing a non-resumable download, warn user first
  if (action === 'pause' && task && task.supportsRange === false) {
    pendingPauseTaskId = id;
    openModal('modal-pause-warning');
    return;
  }

  // Handle demo/showcase tasks client-side for instant tactile UI feedback
  if (id && (id.startsWith('showcase-') || id.startsWith('demo-'))) {
    if (action === 'pause') {
      if (task) {
        task.status = 'Paused';
        task.speed = 0;
      }
      showToast(`Paused ${taskName}`);
      renderDownloads();
      return;
    }
    if (action === 'resume') {
      if (task) {
        task.status = 'Downloading';
        task.speed = 12400000;
      }
      showToast(`Resumed ${taskName}`, 'success');
      renderDownloads();
      return;
    }
    if (action === 'delete') {
      downloads = downloads.filter(d => d.id !== id);
      showToast(`Removed ${taskName} from list`);
      renderDownloads();
      return;
    }
    if (action === 'folder') {
      showToast(`Opening folder for ${taskName}`);
      return;
    }
  }

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
      const messages = {
        pause: `Paused ${taskName}`,
        resume: `Resumed ${taskName}`,
        restart: `Restarted ${taskName}`,
        delete: `Removed ${taskName} from list`,
        delete_file: `Deleted ${taskName} from disk`
      };
      const toastType = action === 'delete_file' ? 'danger' : (action === 'resume' ? 'success' : 'info');
      showToast(messages[action] || `${taskName} updated`, toastType);
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
  setupBrowseFolderButton('btn-browse-torrent-dir', 'torrent-savedir-input');
  setupBrowseFolderButton('btn-browse-default-dir', 'setting-default-dir');
  setupBrowseFolderButton('btn-browse-temp-dir', 'setting-temp-dir');

  // Open default folder button
  const btnOpenDefDir = document.getElementById('btn-open-default-dir');
  if (btnOpenDefDir) {
    btnOpenDefDir.addEventListener('click', async () => {
      const dir = document.getElementById('setting-default-dir')?.value.trim();
      if (!dir) return;
      try {
        await fetch('/api/tasks/open_folder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: dir, type: 'folder' })
        });
      } catch (e) {
        console.error('Open folder error:', e);
      }
    });
  }

  // Toggle Use Target for Temp Directory
  const toggleUseTarget = document.getElementById('setting-use-target-for-temp');
  if (toggleUseTarget) {
    toggleUseTarget.addEventListener('change', (e) => {
      const group = document.getElementById('temp-dir-input-group');
      if (group) group.classList.toggle('disabled', e.target.checked);
    });
  }

  // Refresh Drives Button
  const btnRefreshDrives = document.getElementById('btn-refresh-drives');
  if (btnRefreshDrives) {
    btnRefreshDrives.addEventListener('click', () => {
      fetchHostDrives();
    });
  }

  // Paste from Clipboard Button in Add Download dialog
  const btnPasteUrl = document.getElementById('btn-paste-url');
  if (btnPasteUrl) {
    btnPasteUrl.addEventListener('click', async () => {
      try {
        const text = await readAppClipboard();
        const extracted = extractUrlFromText(text);
        if (extracted) {
          const input = document.getElementById('add-url-input');
          if (input) {
            input.value = extracted;
            const nameInput = document.getElementById('add-filename-input');
            if (nameInput && !nameInput.value.trim()) {
              try {
                const p = new URL(extracted).pathname;
                const base = p.substring(p.lastIndexOf('/') + 1);
                if (base && base !== 'watch') nameInput.value = decodeURIComponent(base);
              } catch (_) {}
            }
          }
          showToast('Link pasted from clipboard', 'info');
        } else {
          showToast('Clipboard does not contain a valid URL', 'warning');
        }
      } catch (err) {
        showToast('Unable to read clipboard', 'warning');
      }
    });
  }

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

  // Settings Reset & Restart
  const btnResetDefaults = document.getElementById('btn-reset-defaults');
  if (btnResetDefaults) btnResetDefaults.addEventListener('click', resetSettingsToDefaults);

  const btnRestartApp = document.getElementById('btn-restart-app');
  if (btnRestartApp) {
    btnRestartApp.addEventListener('click', async () => {
      if (confirm('Restart NeoDM and background download services now?')) {
        btnRestartApp.disabled = true;
        btnRestartApp.textContent = 'Restarting...';
        try {
          await fetch('/api/app/restart', { method: 'POST' });
        } catch (e) {}
        setTimeout(() => {
          window.location.reload();
        }, 1500);
      }
    });
  }

  // Settings tab switching
  document.querySelectorAll('.settings-tabs-sidebar .settings-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.settings-tabs-sidebar .settings-tab-btn').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('.settings-panes-wrap .settings-pane').forEach(p => p.classList.remove('active'));

      btn.classList.add('active');
      const paneId = btn.getAttribute('data-tab');
      const pane = document.getElementById(paneId);
      if (pane) pane.classList.add('active');
      if (paneId === 'stab-storage') {
        fetchHostDrives();
      }
    });
  });

  // Download Properties tab switching
  document.querySelectorAll('.download-props-dialog .modal-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const ptab = btn.getAttribute('data-ptab');
      if (!ptab) return;
      document.querySelectorAll('.download-props-dialog .modal-tab-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      document.querySelectorAll('.props-tab-pane').forEach(p => p.style.display = 'none');
      const targetPane = document.getElementById(ptab);
      if (targetPane) targetPane.style.display = 'block';
    });
  });

  // Copy URL in Download Properties
  const btnCopyPropsUrl = document.getElementById('btn-copy-props-url');
  if (btnCopyPropsUrl) {
    btnCopyPropsUrl.addEventListener('click', async () => {
      const val = document.getElementById('props-url-val')?.value;
      if (val) {
        try {
          await navigator.clipboard.writeText(val);
          showToast('URL copied to clipboard');
        } catch (_) {
          showToast('URL copied');
        }
      }
    });
  }

  // Video Downloader Logic & State
  let currentVideoAnalysis = null;

  window.openVideoDownloaderModal = async function() {
    openModal('modal-video-downloader');
    const urlInput = document.getElementById('video-url-input');
    const folderInput = document.getElementById('video-folder-input');

    // Retrieve default download folder for Videos
    try {
      const resp = await fetch('/api/settings');
      if (resp.ok) {
        const s = await resp.json();
        if (folderInput) {
          folderInput.value = (s.categoryFolders && s.categoryFolders['Videos']) || s.defaultDownloadDir || 'Downloads';
        }
      }
    } catch (_) {}

    // Check if clipboard contains a valid video URL
    try {
      const text = await readAppClipboard();
      const extracted = extractUrlFromText(text);
      if (extracted && urlInput && (!urlInput.value || urlInput.value.trim() === '')) {
        urlInput.value = extracted;
        showToast('Pasted media URL from clipboard', 'info');
      }
    } catch (_) {}

    if (urlInput) setTimeout(() => urlInput.focus(), 120);
  };

  const btnPasteVideoUrl = document.getElementById('btn-paste-video-url');
  if (btnPasteVideoUrl) {
    btnPasteVideoUrl.addEventListener('click', async () => {
      try {
        const text = await readAppClipboard();
        const extracted = extractUrlFromText(text);
        if (extracted) {
          const input = document.getElementById('video-url-input');
          if (input) input.value = extracted;
          showToast('Pasted video URL', 'info');
        } else {
          showToast('Clipboard does not contain a valid URL', 'warning');
        }
      } catch (_) {
        showToast('Could not read clipboard', 'error');
      }
    });
  }

  const btnBrowseVideoFolder = document.getElementById('btn-browse-video-folder');
  if (btnBrowseVideoFolder) {
    btnBrowseVideoFolder.addEventListener('click', async () => {
      try {
        const resp = await fetch('/api/folder/select', { method: 'POST' });
        const data = await resp.json();
        if (data && data.success && data.path) {
          const folderInput = document.getElementById('video-folder-input');
          if (folderInput) folderInput.value = data.path;
        }
      } catch (err) {
        console.warn('Folder selection error:', err);
      }
    });
  }

  const btnAnalyzeVideo = document.getElementById('btn-analyze-video');
  if (btnAnalyzeVideo) {
    btnAnalyzeVideo.addEventListener('click', async () => {
      const urlInput = document.getElementById('video-url-input');
      const url = urlInput ? urlInput.value.trim() : '';
      if (!url) {
        showToast('Please enter or paste a video URL first', 'warning');
        return;
      }

      const spinner = document.getElementById('video-analyze-spinner');
      const previewCard = document.getElementById('video-preview-card');
      const optionsWrap = document.getElementById('video-options-wrap');
      const startBtn = document.getElementById('btn-start-video-download');
      const analyzeText = document.getElementById('btn-analyze-text');

      if (spinner) spinner.style.display = 'flex';
      if (previewCard) previewCard.style.display = 'none';
      if (optionsWrap) optionsWrap.style.display = 'none';
      if (startBtn) startBtn.disabled = true;
      btnAnalyzeVideo.disabled = true;
      if (analyzeText) analyzeText.textContent = 'Probing...';

      try {
        const resp = await fetch('/api/video/analyze', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url })
        });

        if (!resp.ok) {
          const errText = await resp.text();
          throw new Error(errText || 'Failed to analyze video');
        }

        const info = await resp.json();
        currentVideoAnalysis = info;

        // Populate metadata
        const titleEl = document.getElementById('video-title-display');
        const authorEl = document.getElementById('video-author-display');
        const platformEl = document.getElementById('video-platform-display');
        const durationEl = document.getElementById('video-duration-display');
        const thumbImg = document.getElementById('video-thumb-img-element');
        const thumbPlaceholder = document.getElementById('video-thumb-placeholder');
        const filenameInput = document.getElementById('video-filename-input');
        const qualitySelect = document.getElementById('video-quality-select');

        if (titleEl) titleEl.textContent = info.title || 'Untitled Video';
        if (authorEl) authorEl.textContent = info.author || 'Web Media';
        if (platformEl) platformEl.textContent = info.platform || 'Stream';
        if (durationEl) durationEl.textContent = info.duration || 'Live / Stream';

        if (thumbImg && thumbPlaceholder) {
          if (info.thumbnail && info.thumbnail.trim() !== '') {
            thumbImg.src = info.thumbnail;
            thumbImg.style.display = 'block';
            thumbPlaceholder.style.display = 'none';
          } else {
            thumbImg.style.display = 'none';
            thumbPlaceholder.style.display = 'flex';
          }
        }

        if (filenameInput) {
          filenameInput.value = info.filename || 'video.mp4';
        }

        if (qualitySelect) {
          qualitySelect.innerHTML = '';
          if (info.formats && info.formats.length > 0) {
            info.formats.forEach((fmt, idx) => {
              const opt = document.createElement('option');
              opt.value = fmt.url || info.bestUrl || url;
              opt.setAttribute('data-quality', fmt.quality || '');
              opt.setAttribute('data-format', fmt.format || '');
              const sizeLabel = fmt.sizeText ? ` (${fmt.sizeText})` : '';
              opt.textContent = `${fmt.quality} - ${fmt.format}${sizeLabel}`;
              if (idx === 0) opt.selected = true;
              qualitySelect.appendChild(opt);
            });
          } else {
            const opt = document.createElement('option');
            opt.value = info.bestUrl || url;
            opt.textContent = `Direct Stream (${info.platform})`;
            opt.selected = true;
            qualitySelect.appendChild(opt);
          }

          // Dynamically toggle filename extension (.mp4 vs .mp3) when stream format changes
          qualitySelect.onchange = () => {
            const opt = qualitySelect.selectedOptions[0];
            if (!opt) return;
            const qual = (opt.getAttribute('data-quality') || '').toLowerCase();
            const fmt = (opt.getAttribute('data-format') || '').toLowerCase();
            const fnInput = document.getElementById('video-filename-input');
            if (fnInput && fnInput.value) {
              const isAud = qual.startsWith('audio') || fmt === 'mp3';
              fnInput.value = formatMediaFilename(fnInput.value, isAud);
            }
          };
        }

        if (previewCard) previewCard.style.display = 'flex';
        if (optionsWrap) optionsWrap.style.display = 'block';
        if (startBtn) startBtn.disabled = false;

        showToast(`Stream ready: ${info.title}`, 'success');
      } catch (err) {
        showToast(`Analysis error: ${err.message}`, 'error');
      } finally {
        if (spinner) spinner.style.display = 'none';
        btnAnalyzeVideo.disabled = false;
        if (analyzeText) analyzeText.textContent = 'Analyze';
      }
    });
  }

  const btnStartVideo = document.getElementById('btn-start-video-download');
  if (btnStartVideo) {
    btnStartVideo.addEventListener('click', async () => {
      const qualitySelect = document.getElementById('video-quality-select');
      const filenameInput = document.getElementById('video-filename-input');
      const folderInput = document.getElementById('video-folder-input');

      const selectedOpt = qualitySelect ? qualitySelect.selectedOptions[0] : null;
      const downloadUrl = (selectedOpt && selectedOpt.value) ? selectedOpt.value : (currentVideoAnalysis ? currentVideoAnalysis.bestUrl : '');
      const quality = (selectedOpt && selectedOpt.getAttribute('data-quality')) ? selectedOpt.getAttribute('data-quality') : 'Default';
      const rawFilename = filenameInput ? filenameInput.value.trim() : 'video.mp4';
      const saveDir = folderInput ? folderInput.value.trim() : '';

      // Guarantee single pristine file extension (.mp4 or .mp3) without duplication
      const isAudio = quality.toLowerCase().startsWith('audio') || (selectedOpt && selectedOpt.getAttribute('data-format') === 'MP3');
      const filename = formatMediaFilename(rawFilename, isAudio);

      if (!downloadUrl) {
        showToast('Please analyze a video stream first', 'warning');
        return;
      }

      try {
        btnStartVideo.disabled = true;

        const conflict = await resolveFileConflictIfNeeded(downloadUrl, filename, saveDir);
        if (conflict.action === 'cancel') {
          btnStartVideo.disabled = false;
          return;
        }

        const resp = await fetch('/api/video/download', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            url: downloadUrl,
            filename: conflict.filename || filename,
            saveDir: saveDir,
            quality: quality,
            conflictAction: conflict.action
          })
        });

        if (!resp.ok) {
          const errText = await resp.text();
          throw new Error(errText || 'Failed to start video download');
        }

        closeModal('modal-video-downloader');
        showToast(`Started video capture: ${filename}`, 'success');
        loadInitialData();
      } catch (err) {
        showToast(`Video capture error: ${err.message}`, 'error');
      } finally {
        btnStartVideo.disabled = false;
      }
    });
  }

  // Renew URL Modal actions
  const btnPasteRenewUrl = document.getElementById('btn-paste-renew-url');
  if (btnPasteRenewUrl) {
    btnPasteRenewUrl.addEventListener('click', async () => {
      try {
        const text = await readAppClipboard();
        const extracted = extractUrlFromText(text);
        if (extracted) {
          const input = document.getElementById('renew-url-input');
          if (input) input.value = extracted;
          showToast('Fresh URL pasted from clipboard', 'info');
        } else {
          showToast('Clipboard does not contain a valid URL', 'warning');
        }
      } catch (_) {}
    });
  }

  const btnSubmitRenew = document.getElementById('btn-submit-renew-url');
  if (btnSubmitRenew) {
    btnSubmitRenew.addEventListener('click', async () => {
      const newUrl = document.getElementById('renew-url-input')?.value.trim();
      if (!newUrl || !currentRenewTaskId) {
        showToast('Please enter a valid URL', 'warning');
        return;
      }
      try {
        const res = await fetch('/api/download/action', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            id: currentRenewTaskId,
            action: 'renew_url',
            newUrl: newUrl
          })
        });
        if (res.ok) {
          showToast('Download URL renewed - resuming transfer', 'success');
          closeModal('modal-renew-url');
          loadInitialData();
        } else {
          showToast('Failed to renew download address', 'error');
        }
      } catch (err) {
        console.error('Renew error:', err);
        showToast('Network error updating download address', 'error');
      }
    });
  }

  // Non-resumable pause confirmation
  const btnConfirmPause = document.getElementById('btn-confirm-pause-nonresumable');
  if (btnConfirmPause) {
    btnConfirmPause.addEventListener('click', () => {
      if (pendingPauseTaskId) {
        const id = pendingPauseTaskId;
        pendingPauseTaskId = null;
        closeModal('modal-pause-warning');
        executePauseDirect(id);
      }
    });
  }

  // Browser Extension Action Handlers
  document.getElementById('btn-copy-ext-path')?.addEventListener('click', async () => {
    if (window._extensionPath) {
      try {
        await navigator.clipboard.writeText(window._extensionPath);
        showToast('Extension folder path copied to clipboard!', 'success');
      } catch (_) {
        showToast(window._extensionPath, 'info');
      }
    } else {
      showToast('Extension path not detected yet', 'warning');
    }
  });

  const handleOpenExtFolder = async () => {
    try {
      await fetch('/api/extension/open', { method: 'POST' });
      showToast('Opening extension folder in Windows Explorer', 'info');
    } catch (e) {
      showToast('Failed to open extension folder', 'error');
    }
  };
  document.getElementById('btn-open-ext-folder')?.addEventListener('click', handleOpenExtFolder);
  document.getElementById('btn-settings-open-ext-folder')?.addEventListener('click', handleOpenExtFolder);

  const handleLaunchBrowser = async (browserName) => {
    try {
      const res = await fetch('/api/extension/launch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ browser: browserName })
      });
      const data = await res.json();
      if (data.success) {
        showToast(data.message || `Launched ${browserName} with NeoDM extension!`, 'success');
      } else {
        showToast(data.error || `Could not launch ${browserName}`, 'warning');
      }
    } catch (e) {
      showToast(`Launch error: ${e.message}`, 'error');
    }
  };
  document.getElementById('btn-launch-chrome')?.addEventListener('click', () => handleLaunchBrowser('chrome'));
  document.getElementById('btn-launch-edge')?.addEventListener('click', () => handleLaunchBrowser('edge'));
  document.getElementById('btn-launch-brave')?.addEventListener('click', () => handleLaunchBrowser('brave'));

  const handleRegisterExtBrowsers = async () => {
    try {
      const res = await fetch('/api/extension/register', { method: 'POST' });
      const data = await res.json();
      if (data.success) {
        showToast(data.message || 'Auto-registered & launched browser with NeoDM extension!', 'success');
        const st = document.getElementById('extension-register-status');
        if (st) st.textContent = data.message || 'Auto-registered ✓ Browser launched';
      } else {
        showToast(`Registration: ${data.error || 'Failed'}`, 'warning');
      }
    } catch (e) {
      showToast(`Registration error: ${e.message}`, 'error');
    }
  };
  document.getElementById('btn-register-browsers')?.addEventListener('click', handleRegisterExtBrowsers);
  document.getElementById('btn-settings-register-browsers')?.addEventListener('click', handleRegisterExtBrowsers);
}

async function refreshBrowserExtensionInfo() {
  try {
    const res = await fetch('/api/extension/info');
    if (res.ok) {
      const data = await res.json();
      const pathEl = document.getElementById('browser-extension-path-display');
      const settingsPathEl = document.getElementById('settings-browser-ext-path');
      const displayPath = data.path || 'Not detected (running outside installed root)';
      if (pathEl) pathEl.textContent = displayPath;
      if (settingsPathEl) settingsPathEl.textContent = displayPath;
      window._extensionPath = data.path;
    }
  } catch (err) {
    console.error('Failed to get extension info:', err);
  }
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
    const autostartCheck = document.getElementById('add-autostart-check');
    const autoStart = autostartCheck ? autostartCheck.checked : true;

    try {
      const conflict = await resolveFileConflictIfNeeded(url, filename, saveDir);
      if (conflict.action === 'cancel') {
        return;
      }

      const res = await fetch('/api/download/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url,
          filename: conflict.filename || filename,
          saveDir,
          queueId: queueId || 'main_queue',
          cookies,
          autoStart: autoStart && startImmediately,
          conflictAction: conflict.action
        })
      });
      if (res.ok) {
        closeModal('modal-add-download');
        document.getElementById('add-url-input').value = '';
        showToast(`Starting '${filename || extractDomain(url) || 'download'}'`, 'success');
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
  const displayTitle = document.getElementById('seg-modal-title');
  if (displayTitle) displayTitle.textContent = 'Download Properties';

  const propsFilename = document.getElementById('props-display-filename');
  if (propsFilename) propsFilename.textContent = task.filename;

  const propsIcon = document.getElementById('props-file-icon');
  if (propsIcon) propsIcon.innerHTML = getFileIcon(task.filename, task.category);

  const urlInput = document.getElementById('props-url-val');
  if (urlInput) urlInput.value = task.url;

  const saveDirInput = document.getElementById('props-savedir-val');
  if (saveDirInput) saveDirInput.value = task.savePath || 'D:\\Downloads';

  const downloadedVal = document.getElementById('seg-modal-downloaded');
  if (downloadedVal) downloadedVal.textContent = formatSize(task.totalSize || task.downloadedBytes || 4500000000);

  const connVal = document.getElementById('props-connections-display');
  if (connVal) connVal.textContent = `${task.maxConns || 16} (Recommended)`;

  const speedVal = document.getElementById('seg-modal-speed');
  if (speedVal) speedVal.textContent = formatSpeed(task.speed || 0);

  const etaVal = document.getElementById('seg-modal-eta');
  if (etaVal) etaVal.textContent = task.eta > 0 ? formatETA(task.eta) : '—';

  const connsVal = document.getElementById('seg-modal-conns');
  if (connsVal) connsVal.textContent = task.segments ? task.segments.filter(s => !s.done).length : (task.maxConns || 16);

  const percentVal = document.getElementById('seg-modal-percent');
  if (percentVal) percentVal.textContent = `${calculatePercent(task)}%`;

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

// Host Drives Query & Capacity Monitoring
async function fetchHostDrives() {
  const container = document.getElementById('drive-gauges-container');
  const banner = document.getElementById('drive-low-space-banner');
  const bannerMsg = document.getElementById('drive-warning-msg');
  if (!container) return;

  try {
    const res = await fetch('/api/system/drives');
    if (!res.ok) return;
    const data = await res.json();
    if (!data.success || !Array.isArray(data.drives)) return;

    container.innerHTML = '';
    let criticalDrive = null;
    let lowDrive = null;

    data.drives.forEach(drive => {
      const mount = drive.mount || drive.letter || 'Disk';
      const label = drive.label || '';
      const fsType = drive.fs || 'NTFS';
      const freeBytes = (drive.free_bytes !== undefined) ? drive.free_bytes : ((drive.freeBytes !== undefined) ? drive.freeBytes : 0);
      const totalBytes = (drive.total_bytes !== undefined) ? drive.total_bytes : ((drive.totalBytes !== undefined) ? drive.totalBytes : 0);
      const freeStr = drive.free_str || formatSize(freeBytes);
      const totalStr = drive.total_str || formatSize(totalBytes);

      let usedPercent = drive.used_percent;
      if (usedPercent === undefined && totalBytes > 0) {
        usedPercent = ((totalBytes - freeBytes) / totalBytes) * 100;
      }
      usedPercent = Math.min(100, Math.max(2, usedPercent || 0));

      const freeMB = freeBytes / (1024 * 1024);
      const isCritical = freeMB < 100; // < 100MB (User low disk space condition)
      const isLow = freeMB < 5120;     // < 5GB

      if (isCritical && !criticalDrive) criticalDrive = { mount, freeStr, label };
      else if (isLow && !lowDrive) lowDrive = { mount, freeStr, label };

      const card = document.createElement('div');
      card.className = 'drive-gauge-item';
      card.innerHTML = `
        <div class="drive-gauge-top">
          <div class="drive-name-mount">
            <span class="drive-mount">${escapeHtml(mount)}</span>
            ${label ? `<span class="drive-label" style="font-size: 11px; color: var(--text-muted, #94a3b8); margin-left: 4px;">(${escapeHtml(label)})</span>` : ''}
            <span class="drive-fs">${escapeHtml(fsType)}</span>
          </div>
          <div class="drive-free-text ${isCritical ? 'critical' : (isLow ? 'low' : '')}">
            ${escapeHtml(freeStr)} free of ${escapeHtml(totalStr)}
          </div>
        </div>
        <div class="drive-progress-track">
          <div class="drive-progress-fill ${isCritical ? 'critical' : (isLow ? 'low' : '')}" style="width: ${usedPercent}%"></div>
        </div>
      `;
      container.appendChild(card);
    });

    if (banner && bannerMsg) {
      if (criticalDrive) {
        banner.style.display = 'flex';
        bannerMsg.innerHTML = `<strong>Critical:</strong> Drive <code>${escapeHtml(criticalDrive.mount)}</code> has only ${escapeHtml(criticalDrive.free_str)} remaining! Keep <em>"Use target download folder"</em> enabled so segments save directly to your destination drive without exhausting ${escapeHtml(criticalDrive.mount)}.`;
      } else if (lowDrive) {
        banner.style.display = 'flex';
        bannerMsg.innerHTML = `<strong>Notice:</strong> Drive <code>${escapeHtml(lowDrive.mount)}</code> has only ${escapeHtml(lowDrive.free_str)} free space remaining.`;
      } else {
        banner.style.display = 'none';
      }
    }
  } catch (err) {
    console.error('Fetch host drives error:', err);
  }
}

// Settings Modal Management
async function openSettingsModal() {
  refreshBrowserExtensionInfo();
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

  const tempDirInput = document.getElementById('setting-temp-dir');
  if (tempDirInput) tempDirInput.value = s.temporaryDirectory || '';
  const useTargetToggle = document.getElementById('setting-use-target-for-temp');
  const tempGroup = document.getElementById('temp-dir-input-group');
  const useTargetVal = s.useTargetForTemp !== undefined ? !!s.useTargetForTemp : true;
  if (useTargetToggle) {
    useTargetToggle.checked = useTargetVal;
    if (tempGroup) tempGroup.classList.toggle('disabled', useTargetVal);
  }

  fetchHostDrives();

  document.getElementById('setting-duplicate-action').value = s.duplicateFileAction || 'prompt';

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

  document.getElementById('setting-theme-picker').value = (s.selectedTheme === 'light') ? 'light' : 'dark';
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
    temporaryDirectory: document.getElementById('setting-temp-dir')?.value.trim() || '',
    useTargetForTemp: document.getElementById('setting-use-target-for-temp')?.checked ?? true,
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

// // Utilities
function calculatePercent(d) {
  if (typeof d.percent === 'number') return d.percent;
  if (!d.totalSize || d.totalSize <= 0) return 0;
  return Math.min(100, Math.floor(((d.downloadedBytes || 0) / d.totalSize) * 100));
}

function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  const formatted = (val % 1 === 0 || val >= 100) ? val.toFixed(0) : val.toFixed(1);
  return `${formatted} ${units[i]}`;
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
  const name = (filename || '').toLowerCase();
  if (name.includes('ubuntu') || name.endsWith('.iso')) {
    return `<div class="file-icon-wrap file-icon-doc" title="ISO Image">
      <svg width="18" height="22" viewBox="0 0 20 24" fill="none">
        <path d="M2 2C2 0.895431 2.89543 0 4 0H14L20 6V22C20 23.1046 19.1046 24 18 24H4C2.89543 24 2 23.1046 2 22V2Z" fill="#E2E8F0"/>
        <path d="M14 0V6H20L14 0Z" fill="#94A3B8"/>
      </svg>
    </div>`;
  }
  if (name.includes('photoshop') || name.endsWith('.psd')) {
    return `<div class="file-icon-wrap file-icon-ps" title="Adobe Photoshop">Ps</div>`;
  }
  if (name.includes('interstellar')) {
    return `<div class="file-icon-wrap file-icon-interstellar" title="Interstellar">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
        <rect width="24" height="24" rx="4" fill="#090d16"/>
        <circle cx="12" cy="11" r="7" fill="#1e293b"/>
        <ellipse cx="12" cy="11" rx="5" ry="5" fill="#0f172a"/>
        <path d="M7 11C7 8.2 9.2 6 12 6C14.8 6 17 8.2 17 11C17 12.5 15 15 12 15C9 15 7 12.5 7 11Z" fill="#fbbf24"/>
        <circle cx="12" cy="11" r="2.5" fill="#f59e0b"/>
      </svg>
    </div>`;
  }
  if (name.includes('figma')) {
    return `<div class="file-icon-wrap file-icon-figma" title="Figma">
      <svg width="15" height="22" viewBox="0 0 38 57" fill="none">
        <path d="M19 28.5C13.7533 28.5 9.5 24.2467 9.5 19C9.5 13.7533 13.7533 9.5 19 9.5H28.5V28.5H19Z" fill="#F24E1E"/>
        <path d="M9.5 9.5C9.5 4.25329 13.7533 0 19 0H28.5V9.5H19C13.7533 9.5 9.5 9.5 9.5 9.5Z" fill="#FF7262"/>
        <path d="M28.5 0V9.5H38C38 4.25329 33.7467 0 28.5 0Z" fill="#1ABCFE"/>
        <path d="M38 19C38 24.2467 33.7467 28.5 28.5 28.5H19V9.5H28.5C33.7467 9.5 38 13.7533 38 19Z" fill="#0ACF83"/>
        <path d="M9.5 38C9.5 32.7533 13.7533 28.5 19 28.5V47.5C13.7533 47.5 9.5 43.2467 9.5 38Z" fill="#A259FF"/>
      </svg>
    </div>`;
  }
  if (name.endsWith('.pdf')) {
    return `<div class="file-icon-wrap file-icon-pdf" title="PDF Document">
      <svg width="18" height="22" viewBox="0 0 20 24" fill="none">
        <path d="M2 2C2 0.895431 2.89543 0 4 0H14L20 6V22C20 23.1046 19.1046 24 18 24H4C2.89543 24 2 23.1046 2 22V2Z" fill="#EF4444"/>
        <path d="M14 0V6H20L14 0Z" fill="#DC2626"/>
        <path d="M6 13C6 11.5 7.5 11 9 11C11.5 11 14 15 14 15C14 15 11 16 9 16C7 16 6 14.5 6 13Z" stroke="#FFFFFF" stroke-width="1.2" fill="none"/>
      </svg>
    </div>`;
  }
  if (name.includes('wallpapers') || name.includes('collection') || category === 'Compressed' || name.endsWith('.zip') || name.endsWith('.rar') || name.endsWith('.7z')) {
    return `<div class="file-icon-wrap file-icon-archive" title="Archive">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
        <path d="M3 4C3 2.89543 3.89543 2 5 2H9.58579C10.1162 2 10.6249 2.21071 11 2.58579L12.4142 4H19C20.1046 4 21 4.89543 21 6V19C21 20.1046 20.1046 21 19 21H5C3.89543 21 3 20.1046 3 19V4Z" fill="#F59E0B"/>
        <rect x="10.5" y="6" width="3" height="10" rx="1" fill="#D97706"/>
        <rect x="10" y="13" width="4" height="4" rx="1" fill="#FEF3C7"/>
      </svg>
    </div>`;
  }
  if (category === 'Videos' || name.endsWith('.mp4') || name.endsWith('.mkv') || name.endsWith('.avi')) {
    return `<div class="file-icon-wrap file-icon-video" title="Video">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
        <rect width="24" height="24" rx="5" fill="#0284C7"/>
        <polygon points="10 8 16 12 10 16 10 8" fill="#FFFFFF"/>
      </svg>
    </div>`;
  }
  if (category === 'Music' || name.endsWith('.mp3') || name.endsWith('.wav') || name.endsWith('.flac')) {
    return `<div class="file-icon-wrap" style="color: #10b981;" title="Audio">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18V5l12-2v13"></path><circle cx="6" cy="18" r="3"></circle><circle cx="18" cy="16" r="3"></circle></svg>
    </div>`;
  }
  if (category === 'Programs' || name.endsWith('.exe') || name.endsWith('.msi')) {
    return `<div class="file-icon-wrap" style="color: #38bdf8;" title="Application">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect></svg>
    </div>`;
  }
  return `<div class="file-icon-wrap file-icon-doc" title="File">
    <svg width="18" height="22" viewBox="0 0 20 24" fill="none">
      <path d="M2 2C2 0.895431 2.89543 0 4 0H14L20 6V22C20 23.1046 19.1046 24 18 24H4C2.89543 24 2 23.1046 2 22V2Z" fill="#E2E8F0"/>
      <path d="M14 0V6H20L14 0Z" fill="#94A3B8"/>
    </svg>
  </div>`;
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


// --- File Collision Resolution System ---
async function checkFileCollision(url, filename, saveDir) {
  try {
    const res = await fetch('/api/file/check_exists', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, filename, saveDir })
    });
    if (res.ok) {
      return await res.json();
    }
  } catch (e) {
    console.error('File collision check error:', e);
  }
  return { exists: false };
}

function promptFileConflict(conflictData) {
  return new Promise((resolve) => {
    const nameEl = document.getElementById('conflict-filename');
    const pathEl = document.getElementById('conflict-filepath');
    const sizeEl = document.getElementById('conflict-filesize');
    const dateEl = document.getElementById('conflict-filedate');
    const renameTextEl = document.getElementById('conflict-suggested-name');
    const rememberEl = document.getElementById('conflict-remember-choice');

    if (nameEl) nameEl.textContent = conflictData.filename || 'File';
    if (pathEl) pathEl.textContent = conflictData.fullPath || '';
    if (sizeEl) sizeEl.textContent = `Size: ${formatSize(conflictData.existingFileSize || 0)}`;
    if (dateEl) {
      const dt = conflictData.existingFileModTime ? new Date(conflictData.existingFileModTime).toLocaleString() : 'Unknown';
      dateEl.textContent = `Modified: ${dt}`;
    }
    if (renameTextEl) renameTextEl.textContent = conflictData.suggestedRename || `${conflictData.filename} (1)`;
    if (rememberEl) rememberEl.checked = false;

    const btnCancel = document.getElementById('btn-conflict-cancel');
    const btnOverwrite = document.getElementById('btn-conflict-overwrite');
    const btnRename = document.getElementById('btn-conflict-rename');

    const cleanup = () => {
      closeModal('modal-file-conflict');
      if (btnCancel) btnCancel.removeEventListener('click', onCancel);
      if (btnOverwrite) btnOverwrite.removeEventListener('click', onOverwrite);
      if (btnRename) btnRename.removeEventListener('click', onRename);
    };

    const onCancel = () => {
      cleanup();
      resolve({ action: 'cancel' });
    };

    const onOverwrite = async () => {
      const remember = rememberEl && rememberEl.checked;
      cleanup();
      if (remember) {
        await updateSettingDuplicateAction('overwrite');
      }
      resolve({ action: 'overwrite', filename: conflictData.filename });
    };

    const onRename = async () => {
      const remember = rememberEl && rememberEl.checked;
      cleanup();
      if (remember) {
        await updateSettingDuplicateAction('rename');
      }
      resolve({ action: 'rename', filename: conflictData.suggestedRename || conflictData.filename });
    };

    if (btnCancel) btnCancel.addEventListener('click', onCancel, { once: true });
    if (btnOverwrite) btnOverwrite.addEventListener('click', onOverwrite, { once: true });
    if (btnRename) btnRename.addEventListener('click', onRename, { once: true });

    openModal('modal-file-conflict');
  });
}

async function updateSettingDuplicateAction(action) {
  try {
    const sResp = await fetch('/api/settings');
    if (sResp.ok) {
      const settings = await sResp.json();
      settings.duplicateFileAction = action;
      await fetch('/api/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      });
      const el = document.getElementById('setting-duplicate-action');
      if (el) el.value = action;
      if (window.currentSettings) window.currentSettings.duplicateFileAction = action;
    }
  } catch (e) {
    console.error('Failed to update duplicateFileAction setting:', e);
  }
}

async function resolveFileConflictIfNeeded(url, filename, saveDir) {
  try {
    let pref = 'prompt';
    if (window.currentSettings && window.currentSettings.duplicateFileAction) {
      pref = window.currentSettings.duplicateFileAction;
    } else {
      const sResp = await fetch('/api/settings');
      if (sResp.ok) {
        const settings = await sResp.json();
        window.currentSettings = settings;
        pref = settings.duplicateFileAction || 'prompt';
      }
    }

    if (pref === 'overwrite') {
      return { action: 'overwrite', filename: filename };
    }
    if (pref === 'rename') {
      return { action: 'rename', filename: filename };
    }

    // pref === 'prompt': probe backend
    const check = await checkFileCollision(url, filename, saveDir);
    if (!check.exists) {
      return { action: 'proceed', filename: filename };
    }

    // Prompt user via modal
    return await promptFileConflict(check);
  } catch (err) {
    console.error('resolveFileConflictIfNeeded error:', err);
    return { action: 'proceed', filename: filename };
  }
}
