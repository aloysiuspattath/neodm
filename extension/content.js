// NeoDM Browser Integration - Context Menu & Right-Click Media Helper
// Floating onscreen badges are disabled; downloads trigger strictly via right-click ("Download with NeoDM").

(function () {
  function removeLegacyPills() {
    const pill = document.getElementById('neodm-media-pill-overlay');
    if (pill) {
      pill.remove();
    }
  }

  // Capture right-clicked media target so background context menu gets the exact stream URL
  document.addEventListener('contextmenu', (e) => {
    removeLegacyPills();
    let target = e.target;
    if (!target) return;

    let mediaUrl = null;
    const tagName = target.tagName ? target.tagName.toLowerCase() : '';

    if (tagName === 'video' || tagName === 'audio') {
      mediaUrl = target.src || target.currentSrc;
    } else {
      const parentMedia = target.closest ? target.closest('video, audio') : null;
      if (parentMedia) {
        mediaUrl = parentMedia.src || parentMedia.currentSrc;
      } else {
        const nestedMedia = target.querySelector ? target.querySelector('video, audio') : null;
        if (nestedMedia) {
          mediaUrl = nestedMedia.src || nestedMedia.currentSrc;
        }
      }
    }

    if (mediaUrl && !mediaUrl.startsWith('blob:') && !mediaUrl.startsWith('data:')) {
      try {
        chrome.runtime.sendMessage({
          type: 'MEDIA_CONTEXT_CLICK',
          url: mediaUrl,
          pageUrl: window.location.href,
          title: document.title || ''
        });
      } catch (err) {}
    }
  }, true);

  removeLegacyPills();
})();
