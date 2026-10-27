// NeoDM Media Stream Sniffer (Content Script)

(function () {
  const detectedMedia = new Set();

  function scanMediaElements() {
    const mediaEls = document.querySelectorAll('video, audio, source');
    mediaEls.forEach(el => {
      let src = el.src || el.currentSrc;
      if (!src && el.tagName.toLowerCase() === 'source') {
        src = el.getAttribute('src');
      }

      if (src && !src.startsWith('blob:') && !src.startsWith('data:') && !detectedMedia.has(src)) {
        detectedMedia.add(src);
        injectDownloadPill(el, src);
      }
    });
  }

  function injectDownloadPill(targetEl, mediaUrl) {
    if (document.getElementById(`neodm-pill-${hash(mediaUrl)}`)) return;

    const pill = document.createElement('div');
    pill.id = `neodm-pill-${hash(mediaUrl)}`;
    pill.className = 'neodm-media-pill';
    pill.innerHTML = `
      <div class="neodm-pill-btn">
        <span class="neodm-pill-icon">⚡</span>
        <span class="neodm-pill-text">Download with NeoDM</span>
      </div>
    `;

    pill.addEventListener('click', (e) => {
      e.stopPropagation();
      e.preventDefault();
      chrome.runtime.sendMessage({
        type: 'SNIFFED_MEDIA',
        url: mediaUrl,
        title: document.title || 'media_stream'
      }, () => {
        pill.innerHTML = `<span class="neodm-pill-text">✓ Sent to NeoDM</span>`;
        setTimeout(() => pill.remove(), 2000);
      });
    });

    // Style the injected pill
    Object.assign(pill.style, {
      position: 'absolute',
      zIndex: '2147483647',
      cursor: 'pointer',
      backgroundColor: '#0f172a',
      color: '#38bdf8',
      border: '1px solid #334155',
      borderRadius: '20px',
      padding: '4px 10px',
      fontSize: '12px',
      fontWeight: '600',
      display: 'flex',
      alignItems: 'center',
      gap: '6px',
      boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
      transition: 'all 0.2s ease',
      fontFamily: 'Segoe UI, sans-serif'
    });

    // Position relative to video or top-right of page
    try {
      const rect = targetEl.getBoundingClientRect();
      if (rect.width > 120 && rect.height > 80) {
        pill.style.top = `${Math.max(10, window.scrollY + rect.top + 10)}px`;
        pill.style.left = `${Math.max(10, window.scrollX + rect.right - 180)}px`;
      } else {
        pill.style.position = 'fixed';
        pill.style.top = '16px';
        pill.style.right = '16px';
      }
    } catch (e) {
      pill.style.position = 'fixed';
      pill.style.top = '16px';
      pill.style.right = '16px';
    }

    document.body.appendChild(pill);
  }

  function hash(str) {
    let h = 0;
    for (let i = 0; i < str.length; i++) {
      h = (Math.imul(31, h) + str.charCodeAt(i)) | 0;
    }
    return Math.abs(h);
  }

  // Periodic scan & DOM mutation observer
  setInterval(scanMediaElements, 3000);
  const observer = new MutationObserver(() => scanMediaElements());
  observer.observe(document.body, { childList: true, subtree: true });
})();
