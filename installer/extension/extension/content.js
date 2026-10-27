// NeoDM Media Stream Sniffer (Content Script)

(function () {
  let lastInjectedHref = '';

  function isMediaPlatform() {
    const host = window.location.hostname.toLowerCase();
    return host.includes('youtube.com') ||
           host.includes('youtu.be') ||
           host.includes('vimeo.com') ||
           host.includes('twitch.tv') ||
           host.includes('tiktok.com') ||
           host.includes('twitter.com') ||
           host.includes('x.com') ||
           host.includes('facebook.com') ||
           host.includes('instagram.com') ||
           host.includes('dailymotion.com');
  }

  function scanMediaElements() {
    const mediaEls = document.querySelectorAll('video, audio, source');
    const isPlatform = isMediaPlatform();

    // Check if on a video watch page (e.g. YouTube watch page)
    if (isPlatform && (window.location.href.includes('watch') || window.location.href.includes('/shorts/') || window.location.href.includes('/video/'))) {
      const mainVideo = document.querySelector('video');
      if (mainVideo) {
        injectOrUpdateDownloadPill(mainVideo, window.location.href, true);
        return;
      }
    }

    mediaEls.forEach(el => {
      let src = el.src || el.currentSrc;
      if (!src && el.tagName.toLowerCase() === 'source') {
        src = el.getAttribute('src');
      }

      if (src) {
        if (!src.startsWith('blob:') && !src.startsWith('data:')) {
          injectOrUpdateDownloadPill(el, src, false);
        } else if (isPlatform || el.tagName.toLowerCase() === 'video') {
          // Stream using MediaSource / blob URL - target the page URL for yt-dlp muxing
          injectOrUpdateDownloadPill(el, window.location.href, true);
        }
      }
    });
  }

  function injectOrUpdateDownloadPill(targetEl, mediaUrl, isStream) {
    const pillId = 'neodm-media-pill-overlay';
    let pill = document.getElementById(pillId);

    if (pill) {
      if (pill.dataset.targetUrl === mediaUrl) {
        return; // Already up to date
      }
      pill.dataset.targetUrl = mediaUrl;
      pill.dataset.isStream = isStream ? 'true' : 'false';
      return;
    }

    pill = document.createElement('div');
    pill.id = pillId;
    pill.className = 'neodm-media-pill';
    pill.dataset.targetUrl = mediaUrl;
    pill.dataset.isStream = isStream ? 'true' : 'false';

    pill.innerHTML = `
      <div style="display:flex;align-items:center;gap:6px;padding:6px 12px;background:#0f172a;color:#38bdf8;border:1px solid #334155;border-radius:20px;font-family:system-ui,-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;font-size:12px;font-weight:600;box-shadow:0 4px 16px rgba(0,0,0,0.6);cursor:pointer;user-select:none;transition:transform 0.15s ease,background 0.15s ease;">
        <span style="font-size:14px;color:#38bdf8;">⚡</span>
        <span id="neodm-pill-text">Download with NeoDM</span>
      </div>
    `;

    pill.addEventListener('mouseenter', () => {
      const inner = pill.firstElementChild;
      if (inner) {
        inner.style.background = '#1e293b';
        inner.style.transform = 'scale(1.04)';
      }
    });

    pill.addEventListener('mouseleave', () => {
      const inner = pill.firstElementChild;
      if (inner) {
        inner.style.background = '#0f172a';
        inner.style.transform = 'scale(1.0)';
      }
    });

    pill.addEventListener('click', (e) => {
      e.stopPropagation();
      e.preventDefault();
      const currentTargetUrl = pill.dataset.targetUrl || window.location.href;
      const textSpan = pill.querySelector('#neodm-pill-text');
      if (textSpan) textSpan.textContent = 'Sending to NeoDM...';

      chrome.runtime.sendMessage({
        type: 'SNIFFED_MEDIA',
        url: currentTargetUrl,
        title: document.title || 'media_download'
      }, (resp) => {
        if (textSpan) textSpan.textContent = '✓ Sent to NeoDM';
        setTimeout(() => {
          if (textSpan) textSpan.textContent = 'Download with NeoDM';
        }, 3000);
      });
    });

    // Style the container
    Object.assign(pill.style, {
      position: 'fixed',
      top: '20px',
      right: '24px',
      zIndex: '2147483647',
      display: 'block'
    });

    document.body.appendChild(pill);
  }

  // Periodic scan & DOM mutation observer
  setInterval(scanMediaElements, 2000);
  const observer = new MutationObserver(() => scanMediaElements());
  if (document.body) {
    observer.observe(document.body, { childList: true, subtree: true });
  } else {
    document.addEventListener('DOMContentLoaded', () => {
      observer.observe(document.body, { childList: true, subtree: true });
    });
  }
})();
