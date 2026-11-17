/* 轻量 Markdown 渲染器：离线可用、容错、为中文笔记优化 */
(function () {
  const escape = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

  function safeUrl(u) {
    const v = String(u || '').trim();
    if (/^(javascript|vbscript|data:text\/html)/i.test(v)) return '';
    return v.replace(/"/g, '%22');
  }

  function inline(text) {
    if (!text) return '';
    let s = escape(text);
    const codes = [];
    s = s.replace(/`([^`\n]+)`/g, (m, c) => {
      codes.push(c);
      return '\u0000' + (codes.length - 1) + '\u0000';
    });
    // images
    s = s.replace(/!\[([^\]]*)\]\(\s*([^)\s]+)(?:\s+&quot;[^&]*&quot;)?\s*\)/g, (m, alt, src) => {
      const u = safeUrl(src);
      if (!u) return alt;
      return '<img src="' + u + '" alt="' + alt + '" loading="lazy">';
    });
    // links
    s = s.replace(/\[([^\]]+)\]\(\s*([^)\s]+)(?:\s+&quot;[^&]*&quot;)?\s*\)/g, (m, label, href) => {
      const u = safeUrl(href);
      if (!u) return label;
      if (u.startsWith('#/note/')) {
        return '<a href="' + u + '" class="note-link" data-note="' + u.slice(7) + '">' + label + '</a>';
      }
      const ext = /^https?:/i.test(u) ? ' target="_blank" rel="noopener"' : '';
      return '<a href="' + u + '"' + ext + '>' + label + '</a>';
    });
    // wikilinks
    s = s.replace(/\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g, (m, target, label) => '<span class="wiki-link">' + (label || target) + '</span>');
    // bold / italic / strike / highlight
    s = s.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>').replace(/__([^_\n]+)__/g, '<strong>$1</strong>');
    s = s.replace(/(^|[\s(（])\*([^*\n]+)\*(?=$|[\s)）,.，。；;:：])/g, '$1<em>$2</em>');
    s = s.replace(/(^|[\s(（])_([^_\n]+)_(?=$|[\s)）,.，。；;:：])/g, '$1<em>$2</em>');
    s = s.replace(/~~([^~\n]+)~~/g, '<del>$1</del>');
    s = s.replace(/==([^=\n]+)==/g, '<mark>$1</mark>');
    // bare urls
    s = s.replace(/(^|[\s(])(https?:\/\/[^\s<)]+)/g, (m, pre, u) => pre + '<a href="' + u + '" target="_blank" rel="noopener">' + u + '</a>');
    s = s.replace(/\u0000(\d+)\u0000/g, (m, i) => '<code>' + codes[+i] + '</code>');
    return s;
  }

  function isBlockStart(line) {
    return /^\s*(#{1,6}\s|```|>\s?|([-*+]|\d+[.)])\s|\||---+\s*$|\*\*\*\s*$|___+\s*$)/.test(line);
  }

  function parseList(lines, i) {
    const root = { indent: -1, type: 'ul', items: [], parent: null };
    let list = root;
    const re = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;
    while (i < lines.length) {
      const line = lines[i];
      const m = line.match(re);
      if (!m) {
        if (line.trim() === '') break;
        if (/^\s{2,}/.test(line) && list.items.length) {
          list.items[list.items.length - 1].text += '<br>' + inline(line.trim());
          i++;
          continue;
        }
        break;
      }
      const indent = m[1].replace(/\t/g, '  ').length;
      const type = /\d/.test(m[2]) ? 'ol' : 'ul';
      while (list !== root && indent < list.indent) list = list.parent;
      if (list.items.length === 0) {
        list.indent = indent;
        list.type = type;
      } else if (indent > list.indent) {
        const parentItem = list.items[list.items.length - 1];
        const child = { indent, type, items: [], parent: list };
        parentItem.children.push(child);
        list = child;
      }
      let content = m[3];
      let cls = '';
      const task = content.match(/^\[([ xX])\]\s+(.*)$/);
      if (task) {
        cls = ' class="task"';
        content = '<input type="checkbox" disabled' + (task[1].toLowerCase() === 'x' ? ' checked' : '') + '> ' + inline(task[2]);
      } else {
        content = inline(content);
      }
      list.items.push({ text: content, cls, children: [] });
      i++;
    }
    function render(l) {
      const tag = l.type;
      const hasTask = l.items.some((it) => (it.cls || '').indexOf('task') >= 0);
      let out = '<' + tag + (tag === 'ul' && hasTask ? ' class="task-list"' : '') + '>';
      l.items.forEach((item) => {
        out += '<li' + (item.cls || '') + '>' + item.text;
        (item.children || []).forEach((c) => { out += render(c); });
        out += '</li>';
      });
      return out + '</' + tag + '>';
    }
    return { html: render(root), next: i };
  }

  function parseTable(lines, i) {
    const header = lines[i];
    const sep = lines[i + 1] || '';
    const cells = (l) => l.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
    const align = cells(sep).map((c) => {
      if (/^:-+:$/.test(c)) return 'center';
      if (/^-+:$/.test(c)) return 'right';
      return '';
    });
    const head = cells(header);
    let html = '<table><thead><tr>';
    head.forEach((c, idx) => {
      html += '<th' + (align[idx] ? ' style="text-align:' + align[idx] + '"' : '') + '>' + inline(c) + '</th>';
    });
    html += '</tr></thead><tbody>';
    i += 2;
    while (i < lines.length && /^\s*\|/.test(lines[i])) {
      const row = cells(lines[i]);
      html += '<tr>';
      head.forEach((_, idx) => {
        html += '<td' + (align[idx] ? ' style="text-align:' + align[idx] + '"' : '') + '>' + inline(row[idx] || '') + '</td>';
      });
      html += '</tr>';
      i++;
    }
    return { html: html + '</tbody></table>', next: i };
  }

  function render(md) {
    const lines = String(md || '').replace(/\r\n?/g, '\n').split('\n');
    let html = '';
    let i = 0;
    let headingIndex = 0;
    const headings = [];

    while (i < lines.length) {
      const line = lines[i];
      const trimmed = line.trim();

      if (!trimmed) { i++; continue; }

      const fence = trimmed.match(/^```(.*)$/);
      if (fence) {
        const lang = fence[1].trim();
        i++;
        const buf = [];
        while (i < lines.length && !/^```/.test(lines[i].trim())) { buf.push(lines[i]); i++; }
        i++;
        html += '<pre data-lang="' + escape(lang) + '"><button class="copy-code" type="button">复制</button><code>' + escape(buf.join('\n')) + '</code></pre>';
        continue;
      }

      const h = trimmed.match(/^(#{1,6})\s+(.*)$/);
      if (h) {
        const level = h[1].length;
        const text = h[2].replace(/\s+#+\s*$/, '');
        const id = 'sec-' + (++headingIndex);
        headings.push({ level, text: text.replace(/[*_`~]/g, ''), id });
        html += '<h' + level + ' id="' + id + '">' + inline(text) + '</h' + level + '>';
        i++;
        continue;
      }

      if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) { html += '<hr>'; i++; continue; }

      if (/^\s*\|/.test(line) && /^\s*\|?[\s:-]*-[\s:|-]*\|?\s*$/.test(lines[i + 1] || '')) {
        const t = parseTable(lines, i);
        html += t.html; i = t.next; continue;
      }

      if (/^>\s?/.test(trimmed)) {
        const buf = [];
        while (i < lines.length && /^\s*>/.test(lines[i])) {
          buf.push(lines[i].replace(/^\s*>\s?/, ''));
          i++;
        }
        html += '<blockquote>' + buf.map((l) => inline(l)).join('<br>') + '</blockquote>';
        continue;
      }

      if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
        const l = parseList(lines, i);
        html += l.html; i = l.next; continue;
      }

      // paragraph
      const buf = [];
      while (i < lines.length) {
        const l = lines[i];
        if (!l.trim()) break;
        if (buf.length && (isBlockStart(l) || /^```/.test(l.trim()))) break;
        buf.push(l.trim());
        i++;
      }
      if (buf.length) html += '<p>' + buf.map((l) => inline(l)).join('<br>') + '</p>';
      else i++;
    }
    return { html: html || '<p class="md-empty">还没有内容。开始输入，或从右侧工具栏插入。</p>', headings };
  }

  window.MD = { render, inline, escape };
})();
