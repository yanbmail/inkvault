/* Inline stroke icons — no external icon font, works fully offline. */
(function () {
  const P = {
    'file-text': '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M9 13h6M9 17h4"/>',
    'search': '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/>',
    'chart': '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
    'download': '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 19h16"/>',
    'upload': '<path d="M12 15V3"/><path d="m7 8 5-5 5 5"/><path d="M4 19h16"/>',
    'settings': '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
    'x': '<path d="M18 6 6 18M6 6l12 12"/>',
    'layers': '<path d="m12 2 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5"/><path d="m3 17 9 5 9-5"/>',
    'star': '<path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 9.5l6.1-.9L12 3Z"/>',
    'pin': '<path d="M12 17v5"/><path d="M9 10.8V4h6v6.8l2 3.2H7l2-3.2Z"/>',
    'clock': '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    'archive': '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/>',
    'trash': '<path d="M4 7h16"/><path d="M9 7V4h6v3"/><path d="M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13"/><path d="M10 11v6M14 11v6"/>',
    'plus': '<path d="M12 5v14M5 12h14"/>',
    'minus': '<path d="M5 12h14"/>',
    'database': '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>',
    'refresh': '<path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v6h-6"/>',
    'menu': '<path d="M4 6h16M4 12h16M4 18h16"/>',
    'sort': '<path d="M7 4v16M3 8l4-4 4 4"/><path d="M17 20V4M13 16l4 4 4-4"/>',
    'feather': '<path d="M20.2 3.8a6 6 0 0 0-8.5 0L4 11.5V20h8.5l7.7-7.7a6 6 0 0 0 0-8.5Z"/><path d="M16 8 4 20M14 4l6 6"/>',
    'arrow-left': '<path d="M19 12H5"/><path d="m11 18-6-6 6-6"/>',
    'history': '<path d="M3 12a9 9 0 1 0 2.6-6.4"/><path d="M3 4v5h5"/><path d="M12 8v4l3 2"/>',
    'more': '<circle cx="12" cy="5" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="12" cy="19" r="1.4"/>',
    'bold': '<path d="M6 4h7a3.5 3.5 0 0 1 0 7H6z"/><path d="M6 11h8a4 4 0 0 1 0 9H6z"/>',
    'italic': '<path d="M19 4h-9M14 20H5M15 4 9 20"/>',
    'strike': '<path d="M4 12h16"/><path d="M17 6.5C16.3 5 14.6 4 12.5 4 9.8 4 8 5.4 8 7.5c0 1.6 1 2.7 2.7 3.5"/><path d="M8 17c.8 1.7 2.5 3 5 3 2.9 0 4.6-1.5 4.6-3.6 0-1.4-.7-2.4-2-3.1"/>',
    'code': '<path d="m8 8-4 4 4 4M16 8l4 4-4 4M14 4l-4 16"/>',
    'list': '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>',
    'list-ordered': '<path d="M10 6h10M10 12h10M10 18h10"/><path d="M4 5h1v4M3 9h3M3 14h2.2a1 1 0 0 1 0 2H3l2.5 3H3"/>',
    'check-square': '<rect x="3" y="3" width="18" height="18" rx="3"/><path d="m8 12 3 3 5-6"/>',
    'check': '<path d="m4 12 5 5L20 6"/>',
    'quote': '<path d="M7 7H4v6h5V9c0-3-2-4-4-4"/><path d="M17 7h-3v6h5V9c0-3-2-4-4-4"/>',
    'link': '<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>',
    'image': '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m21 16-5-5-6 7"/>',
    'table': '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M9 10v10M15 10v10"/>',
    'columns': '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>',
    'edit': '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    'eye': '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
    'list-tree': '<path d="M8 6h13M8 12h13M8 18h13"/><path d="M3 6h.01M3 12h.01M3 18h.01"/>',
    'chevron-right': '<path d="m9 6 6 6-6 6"/>',
    'chevron-down': '<path d="m6 9 6 6 6-6"/>',
    'folder': '<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    'tag': '<path d="M20 12 12 4H4v8l8 8 8-8Z"/><circle cx="8" cy="8" r="1.2"/>',
    'info': '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
    'copy': '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h8"/>',
    'external': '<path d="M14 4h6v6M20 4l-9 9"/><path d="M20 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h5"/>',
    'sun': '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    'moon': '<path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z"/>',
    'cloud': '<path d="M18 17H7a4 4 0 0 1-.8-7.9A6 6 0 0 1 18 10a4 4 0 0 1 0 7Z"/>',
    'shield': '<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6l-8-3Z"/><path d="m9 12 2 2 4-4"/>',
    'zap': '<path d="M13 2 4 14h7l-1 8 9-12h-7l1-8Z"/>',
    'book': '<path d="M4 4h6a3 3 0 0 1 3 3v13a2.5 2.5 0 0 0-2.5-2H4z"/><path d="M20 4h-6a3 3 0 0 0-3 3v13a2.5 2.5 0 0 1 2.5-2H20z"/>',
    'grid': '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    'filter': '<path d="M3 5h18l-7 8v6l-4-2v-4z"/>',
    'sparkles': '<path d="m12 3 1.8 4.2L18 9l-4.2 1.8L12 15l-1.8-4.2L6 9l4.2-1.8L12 3Z"/><path d="m18 15 .9 2.1L21 18l-2.1.9L18 21l-.9-2.1L15 18l2.1-.9L18 15Z"/>',
    'save': '<path d="M19 21H5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h11l4 4v13a1 1 0 0 1-1 1Z"/><path d="M8 3v6h7V3M8 21v-7h8v7"/>',
    'rotate': '<path d="M3 12a9 9 0 1 0 3-6.7"/><path d="M3 4v5h5"/>',
    'file-down': '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M12 11v6M9.5 14.5 12 17l2.5-2.5"/>',
    'inbox': '<path d="M4 13h4l2 3h4l2-3h4"/><path d="M5 5h14l2 8v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-6z"/>',
    'globe': '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 3 2.5 15 0 18M12 3c-2.5 3-2.5 15 0 18"/>',
    'smartphone': '<rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/>',
    'monitor': '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
    'merge': '<path d="M8 3v6a4 4 0 0 0 4 4h8"/><path d="m17 9 4 4-4 4"/><path d="M8 21v-6a4 4 0 0 1 4-4"/>',
    'key': '<circle cx="8" cy="15" r="4"/><path d="m11 12 8-8M17 4l3 3M15 6l2 2"/>',
    'type': '<path d="M4 6V4h16v2M12 4v16M8 20h8"/>',
    'hash': '<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18"/>',
    'bookmark': '<path d="M6 3h12v18l-6-4-6 4z"/>'
  };
  function svg(name, size) {
    const d = P[name] || P['file-text'];
    return '<svg viewBox="0 0 24 24" width="' + (size || 18) + '" height="' + (size || 18) +
      '" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + d + '</svg>';
  }
  window.Icons = { paths: P, svg };
  window.hydrateIcons = function (root) {
    (root || document).querySelectorAll('i[data-icon]').forEach(function (el) {
      const name = el.getAttribute('data-icon');
      el.innerHTML = svg(name, el.dataset.size || 18);
      el.classList.add('i');
    });
  };
})();
