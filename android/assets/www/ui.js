/* 界面渲染 */
(function () {
  const S = window.State;

  function icon(name, size) { return window.Icons.svg(name, size || 16); }

  function flattenNotebooks(list, depth, out) {
    out = out || [];
    (list || []).forEach((n) => {
      out.push({ uid: n.uid, name: n.name, depth: depth || 0, count: n.count, total: n.total, color: n.color, hasChildren: !!(n.children && n.children.length) });
      if (n.children) flattenNotebooks(n.children, (depth || 0) + 1, out);
    });
    return out;
  }
  window.flattenNotebooks = flattenNotebooks;

  function renderSidebar() {
    const b = S.boot || {};
    $('#vaultName').textContent = S.settings.vaultName || b.vaultName || '我的知识库';
    $('#vaultVersion').textContent = 'v' + (b.version || '1.0');
    const counts = S.counts || {};
    document.querySelectorAll('[data-count]').forEach((el) => {
      const k = el.getAttribute('data-count');
      const v = counts[k] || 0;
      el.textContent = v ? Utils.fmtNum(v) : '';
    });
    renderNotebookTree();
    renderTags();
    renderSavedSearches();
  }

  function renderNotebookTree() {
    const root = $('#notebookTree');
    const tree = S.notebooks || [];
    if (!tree.length) {
      root.innerHTML = '<div style="padding:6px 9px;font-size:11.5px;color:var(--faint)">还没有笔记本</div>';
      return;
    }
    const expanded = S.expanded || {};
    function node(n, depth) {
      const kids = n.children || [];
      const isOpen = expanded[n.uid] !== false;
      const active = S.notebook === n.uid;
      let html = `<button class="tree-row ${active ? 'active' : ''}" data-notebook="${n.uid}" style="padding-left:${8 + depth * 12}px">
        <span class="caret ${kids.length ? '' : 'empty'} ${isOpen ? 'open' : ''}" data-caret="${n.uid}">${kids.length ? icon('chevron-right', 10) : ''}</span>
        <span class="nb-icon">${icon(kids.length ? 'folder' : 'book', 14)}</span>
        <span class="nb-name">${Utils.esc(n.name)}</span>
        ${n.total ? `<span class="nb-count">${n.total}</span>` : ''}
      </button>`;
      if (kids.length && isOpen) {
        html += '<div class="tree-children">' + kids.map((c) => node(c, depth + 1)).join('') + '</div>';
      }
      return html;
    }
    root.innerHTML = tree.map((n) => node(n, 0)).join('');
  }

  function renderTags() {
    const root = $('#tagCloud');
    const all = (S.tags || []).filter((t) => t.count > 0);
    const tags = all.slice(0, S.tagsExpanded ? 60 : 10);
    if (!tags.length) {
      root.innerHTML = '<span style="font-size:11.5px;color:var(--faint);padding-left:3px">标签会随笔记自动出现</span>';
    } else {
      root.innerHTML = tags.map((t) => `<button class="tag-chip ${S.tag === t.name ? 'active' : ''}" data-tag="${Utils.esc(t.name)}">#${Utils.esc(t.name)}<b>${t.count}</b></button>`).join('') +
        (all.length > tags.length ? `<button class="tag-chip" data-tags-more>+${all.length - tags.length}</button>` : (S.tagsExpanded && all.length > 10 ? '<button class="tag-chip" data-tags-more>收起</button>' : ''));
    }
    const savedRoot = $('#savedSearches');
    const section = $('#savedSection');
    const searches = S.searches || [];
    section.style.display = searches.length ? '' : 'none';
    savedRoot.innerHTML = searches.map((s) => `<button class="saved-item" data-search="${s.uid}">${icon('search', 13)}<span>${Utils.esc(s.name)}</span></button>`).join('');
  }

  function renderSavedSearches() { /* handled in renderTags for compactness */ }

  function viewLabel() {
    if (S.query) return '搜索结果';
    if (S.notebook) {
      const nb = flattenNotebooks(S.notebooks).find((n) => n.uid === S.notebook);
      return nb ? nb.name : '笔记本';
    }
    if (S.tag) return '#' + S.tag;
    const map = { all: '全部笔记', starred: '星标', pinned: '置顶', recent: '最近编辑', archived: '归档', trash: '回收站' };
    return map[S.view] || '全部笔记';
  }

  function renderListHeader() {
    $('#listTitle').textContent = viewLabel();
    const total = S.total || 0;
    let sub = Utils.fmtNum(total) + ' 篇';
    if (S.loading) sub = '正在查找…';
    if (S.view === 'trash' && total) sub += ' · 可恢复';
    $('#listSubtitle').textContent = sub;
    const chips = [];
    if (S.query) chips.push(`<span class="filter-chip">搜索：${Utils.esc(S.query)}<button data-clear="query">${icon('x', 11)}</button></span>`);
    if (S.notebook) chips.push(`<span class="filter-chip">笔记本：${Utils.esc(viewLabel())}<button data-clear="notebook">${icon('x', 11)}</button></span>`);
    if (S.tag) chips.push(`<span class="filter-chip">#${Utils.esc(S.tag)}<button data-clear="tag">${icon('x', 11)}</button></span>`);
    if (S.starred) chips.push(`<span class="filter-chip">星标<button data-clear="starred">${icon('x', 11)}</button></span>`);
    if (S.pinned) chips.push(`<span class="filter-chip">置顶<button data-clear="pinned">${icon('x', 11)}</button></span>`);
    if (S.type) chips.push(`<span class="filter-chip">类型：${Utils.esc(S.type)}<button data-clear="type">${icon('x', 11)}</button></span>`);
    if (S.query || S.tag || S.notebook || S.starred || S.pinned) {
      chips.push(`<button class="filter-chip" data-save-search style="background:var(--panel);color:var(--muted);border-color:var(--line)">${icon('bookmark', 11)} 保存搜索</button>`);
    }
    $('#filterChips').innerHTML = chips.join('');
    $('#sortBtn').title = '排序：' + sortLabel(S.sort);
  }

  function sortLabel(s) {
    return ({ '': '智能排序', relevance: '相关度', updated_desc: '最近更新', created_desc: '最近创建', title: '标题', words: '字数最多', oldest: '最早更新' })[S.query ? (s || 'relevance') : (s || 'updated_desc')] || '最近更新';
  }
  window.sortLabel = sortLabel;

  function noteCard(n) {
    const tags = (n.tags || []).slice(0, 2).map((t) => `<span class="nc-tag">#${Utils.esc(t)}</span>`).join('');
    return `<article class="note-card ${S.current && S.current.uid === n.uid ? 'active' : ''}" data-uid="${n.uid}">
      <div class="nc-top">
        ${n.pinned ? `<span class="nc-pin">${icon('pin', 12)}</span>` : ''}
        <h3 class="nc-title">${Utils.highlight(n.title || '无标题笔记', S.query)}</h3>
        ${n.starred ? `<span class="nc-star">${icon('star', 12)}</span>` : ''}
      </div>
      ${n.excerpt ? `<p class="nc-excerpt">${Utils.highlight(n.excerpt, S.query)}</p>` : ''}
      <div class="nc-meta">
        <span class="nc-nb"><span class="dot"></span>${Utils.esc(n.notebookName || '未分类')}</span>
        ${tags}
        <span class="nc-date"><span class="nc-time">${Utils.fmtRelative(n.updatedAt)}</span></span>
      </div>
    </article>`;
  }

  function renderNotes(append) {
    const root = $('#noteList');
    if (!S.notes.length) {
      root.innerHTML = '';
      $('#listEmpty').classList.toggle('hidden', !!S.query || S.view === 'trash');
      $('#loadMore').classList.add('hidden');
      if (S.query || S.view === 'trash') {
        root.innerHTML = `<div style="padding:36px 18px;text-align:center;color:var(--faint);font-size:12.5px">
          ${S.query ? '没有找到匹配的笔记。<br>试试更短的关键词，或用 tag: / notebook: 语法。' : '回收站是空的。'}</div>`;
      }
      return;
    }
    $('#listEmpty').classList.add('hidden');
    const html = S.notes.map(noteCard).join('');
    if (append) root.insertAdjacentHTML('beforeend', html);
    else root.innerHTML = html;
    $('#loadMore').classList.toggle('hidden', S.notes.length >= S.total);
  }

  function renderNotebookSelect() {
    const sel = $('#notebookSelect');
    const opts = flattenNotebooks(S.notebooks);
    sel.innerHTML = opts.map((n) => `<option value="${n.uid}">${'\u00a0\u00a0'.repeat(n.depth)}${Utils.esc(n.name)}</option>`).join('') || '<option value="">未分类</option>';
    if (S.current) sel.value = S.current.notebookUid || '';
  }

  function tagChipsHtml(note) {
    const tags = note.tags || [];
    return tags.map((t) => `<span class="note-tag">#${Utils.esc(t)}<button data-remove-tag="${Utils.esc(t)}" title="移除">${icon('x', 10)}</button></span>`).join('') +
      `<input class="tag-input" id="tagInput" placeholder="${tags.length ? '+ 标签' : '添加标签'}" autocomplete="off">`;
  }

  function renderFields(note) {
    const box = $('#fieldEditor');
    if (!box) return;
    const fields = (S.fields || []).filter((f) => !f.scope || f.scope === note.notebookUid);
    if (!fields.length || S.local) { box.classList.add('hidden'); box.innerHTML = ''; return; }
    box.classList.remove('hidden');
    box.innerHTML = fields.map((f) => {
      const val = (note.fields && note.fields[f.uid]) || '';
      if (f.type === 'checkbox') {
        return `<label class="cf"><input type="checkbox" data-field="${f.uid}" ${val === 'true' ? 'checked' : ''}><span>${Utils.esc(f.name)}</span></label>`;
      }
      if (f.type === 'select' || f.type === 'multiselect') {
        return `<label class="cf"><span>${Utils.esc(f.name)}</span><select data-field="${f.uid}"><option value="">—</option>${(f.options || []).map((o) => `<option ${o === val ? 'selected' : ''}>${Utils.esc(o)}</option>`).join('')}</select></label>`;
      }
      return `<label class="cf"><span>${Utils.esc(f.name)}</span><input data-field="${f.uid}" type="${f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text'}" value="${Utils.esc(val)}" placeholder="—"></label>`;
    }).join('');
  }

  function renderEditor(note) {
    if (!note) {
      $('#editorEmpty').classList.remove('hidden');
      $('#editorWrap').classList.add('hidden');
      return;
    }
    $('#editorEmpty').classList.add('hidden');
    $('#editorWrap').classList.remove('hidden');
    $('#noteTitle').value = note.title || '';
    $('#content').value = note.content || '';
    $('#notebookSelect').value = note.notebookUid || '';
    $('#tagEditor').innerHTML = tagChipsHtml(note);
    renderFields(note);
    $('#metaDates').textContent = '创建于 ' + Utils.fmtDate(note.createdAt) + ' · 更新于 ' + Utils.fmtRelative(note.updatedAt);
    $('#starBtn').classList.toggle('active', !!note.starred);
    $('#pinBtn').classList.toggle('active', !!note.pinned);
    const nb = flattenNotebooks(S.notebooks).find((n) => n.uid === note.notebookUid);
    $('#nbDot').style.background = (nb && nb.color) || 'var(--accent)';
    $('#attachCount').textContent = (note.attachments && note.attachments.length) ? note.attachments.length + ' 个附件' : '';
    updateStats();
    renderPreview();
    setSaveState(note.updatedAt ? '已保存' : '', 'saved');
    $('#saveTime').textContent = note.updatedAt ? '更新于 ' + Utils.fmtRelative(note.updatedAt) : '';
  }

  function updateStats() {
    const text = $('#content').value || '';
    const words = Utils.countWords(text);
    $('#wordCount').textContent = Utils.fmtNum(words) + ' 字';
    $('#readTime').textContent = '约 ' + Math.max(1, Math.ceil(words / 400)) + ' 分钟';
    $('#charCount').textContent = Utils.fmtNum(text.length) + ' 字符';
    return words;
  }

  function renderPreview() {
    const text = $('#content').value || '';
    const out = window.MD.render(text);
    $('#preview').innerHTML = out.html;
    S.headings = out.headings;
    renderOutline();
  }

  function renderOutline() {
    const panel = $('#outline');
    const hs = (S.headings || []);
    if (!S.outline) { panel.classList.add('hidden'); $('#editorSplit').classList.remove('with-outline'); return; }
    $('#editorSplit').classList.add('with-outline');
    panel.classList.remove('hidden');
    panel.innerHTML = '<div class="outline-title">大纲</div>' + (hs.length ? hs.map((h) =>
      `<a href="#${h.id}" data-outline="${h.id}" class="lv${Math.min(3, h.level)}">${Utils.esc(h.text)}</a>`).join('') : '<div style="font-size:11px;color:var(--faint)">标题会显示在这里</div>');
  }

  function setSaveState(text, cls) {
    const el = $('#saveState');
    el.textContent = text || '';
    el.className = 'save-state ' + (cls || '');
  }
  window.setSaveState = setSaveState;

  function setViewMode(mode) {
    S.viewMode = mode;
    const split = $('#editorSplit');
    split.classList.remove('mode-edit', 'mode-preview', 'mode-split');
    if (mode === 'edit') split.classList.add('mode-edit');
    else if (mode === 'preview') split.classList.add('mode-preview');
    else if (window.innerWidth <= 980) split.classList.add('mode-split');
    document.querySelectorAll('[data-viewmode]').forEach((b) => b.classList.toggle('active', b.getAttribute('data-viewmode') === mode));
    if (mode === 'split') renderPreview();
    try { localStorage.setItem('iv.viewMode', mode); } catch (e) {}
  }
  window.setViewMode = setViewMode;

  function renderStats(st) {
    S.stats = st;
    const heatMax = Math.max(1, ...st.daily.map((d) => d.edits));
    const colors = ['var(--panel-3)', '#e3d5c8', '#c9a795', '#b1735f', '#8f4433'];
    const heat = st.daily.map((d) => {
      let lv = 0;
      if (d.edits > 0) lv = Math.min(4, 1 + Math.floor((d.edits / heatMax) * 3.99));
      return `<i style="background:${colors[lv]}" title="${d.date} · ${d.edits} 次编辑 · 新增 ${d.notes} 篇"></i>`;
    }).join('');
    // fill the last 18 calendar months so the chart keeps a stable rhythm
    const byMonth = {};
    (st.byMonth || []).forEach((m) => { byMonth[m.month] = m; });
    const series = [];
    const cur = new Date();
    cur.setDate(1);
    for (let i = 17; i >= 0; i--) {
      const d = new Date(cur.getFullYear(), cur.getMonth() - i, 1);
      const key = d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
      const m = byMonth[key] || { month: key, notes: 0, words: 0 };
      series.push({ m, d });
    }
    const maxMonth = Math.max(1, ...series.map((x) => x.m.notes));
    const months = series.map(({ m, d }) => {
      const h = m.notes ? 8 + Math.round((m.notes / maxMonth) * 116) : 2;
      const label = (d.getMonth() + 1) + '月';
      return `<div class="bar-col" title="${d.getFullYear()}年${label}：${m.notes} 篇 / ${Utils.fmtNum(m.words)} 字"><i style="height:${h}px" class="${m.notes ? '' : 'empty'}"></i><span>${label}</span></div>`;
    }).join('');
    const maxNb = Math.max(1, ...st.byNotebook.map((n) => n.count));
    const nbRows = st.byNotebook.slice(0, 10).map((n) =>
      `<div class="dist-row"><span>${Utils.esc(n.name)}</span><div class="track"><div class="fill" style="width:${Math.round(n.count / maxNb * 100)}%"></div></div><span class="n">${n.count}</span></div>`).join('');
    const maxTag = Math.max(1, ...st.byTag.map((t) => t.count));
    const tagRows = st.byTag.slice(0, 12).map((t) =>
      `<div class="dist-row"><span>#${Utils.esc(t.name)}</span><div class="track"><div class="fill" style="width:${Math.round(t.count / maxTag * 100)}%;background:var(--accent)"></div></div><span class="n">${t.count}</span></div>`).join('');
    const topRows = st.topNotes.map((n, i) =>
      `<tr><td style="width:24px;color:var(--faint)">${i + 1}</td><td><a href="#/note/${n.uid}" class="stat-note" data-note="${n.uid}">${Utils.esc(n.title)}</a></td><td>${Utils.esc(n.notebook)}</td><td class="num">${Utils.fmtNum(n.words)}</td></tr>`).join('');
    $('#sheetBody').innerHTML = `<div class="sheet-inner">
      <div style="display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;margin-bottom:18px">
        <p style="margin:0;color:var(--muted);font-size:12.5px">共 ${Utils.fmtNum(st.activeNotes)} 篇笔记 · ${Utils.fmtNum(st.totalWords)} 字 · 连续记录 ${st.streak} 天 · ${st.busiestWeekday}最活跃</p>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <button class="btn small" id="statsNoteBtn">${icon('book', 14)}生成统计笔记</button>
          <button class="btn small" id="statsExportBtn">${icon('download', 14)}导出报告</button>
          <button class="btn small primary" id="statsRefresh">${icon('refresh', 14)}刷新</button>
        </div>
      </div>
      <div class="stat-grid">
        ${[
          [Utils.fmtNum(st.activeNotes), '笔记'], [Utils.fmtNum(st.totalWords), '总字数'],
          [Utils.fmtNum(st.avgWords), '篇均字数'], [String(st.notebookCount), '笔记本'],
          [String(st.tagCount), '标签'], [st.streak + ' 天', '连续记录'],
          [Utils.fmtNum(st.createdMonth), '本月新增'], [Utils.fmtNum(st.attachmentCount), '附件']
        ].map((k) => `<div class="stat-card"><div class="v">${k[0]}</div><div class="l">${k[1]}</div></div>`).join('')}
      </div>
      <div class="stat-section">
        <h3>每月新增 <span class="sub">近 18 个月</span></h3>
        <div class="bars">${months || '<div style="color:var(--faint);font-size:12px">还没有数据</div>'}</div>
      </div>
      <div class="stat-section">
        <h3>记录热度 <span class="sub">近一年 · 按编辑次数</span></h3>
        <div class="heat">${heat}</div>
        <div class="heat-legend">少 ${colors.map((c) => `<i style="background:${c}"></i>`).join('')} 多</div>
      </div>
      <div class="stat-cols">
        <div class="stat-section"><h3>笔记本分布</h3>${nbRows || '<div style="color:var(--faint);font-size:12px">暂无</div>'}</div>
        <div class="stat-section"><h3>高频标签</h3>${tagRows || '<div style="color:var(--faint);font-size:12px">暂无</div>'}</div>
      </div>
      <div class="stat-section">
        <h3>最长的笔记 <span class="sub">按字数</span></h3>
        <table class="table"><thead><tr><th></th><th>标题</th><th>笔记本</th><th class="num">字数</th></tr></thead><tbody>${topRows}</tbody></table>
      </div>
    </div>`;
    hydrateIcons($('#sheetBody'));
    if (window.Panels && Panels.bindStats) Panels.bindStats();
  }
  window.renderStats = renderStats;

  window.UI = {
    renderSidebar, renderNotes, renderEditor, renderListHeader, renderNotebookSelect,
    updateStats, renderPreview, renderOutline, setSaveState, setViewMode, renderStats, renderFields,
    renderNotebookTree, renderTags, viewLabel, tagChipsHtml, flattenNotebooks, icon
  };
})();
