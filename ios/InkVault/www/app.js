/* 应用启动、交互与编辑器 */
(function () {
  const S = window.State;
  const app = {
    ready: false,
    saveSeq: 0,
    previewTimer: null,
    searchTimer: null
  };
  window.App = app;
  window.Store = window.LocalStore || window.HTTP;
  S.local = window.Store.kind === 'local';

  /* ------------------------------- helpers -------------------------------- */
  function setTheme(t) {
    S.theme = t || 'light';
    document.documentElement.setAttribute('data-theme', S.theme === 'light' ? '' : S.theme);
    try { localStorage.setItem('iv.theme', S.theme); } catch (e) {}
  }
  function applyFont(px) {
    S.fontSize = px || 15;
    document.documentElement.style.setProperty('--editor-font-size', S.fontSize + 'px');
    const c = $('#content');
    if (c) c.style.fontSize = S.fontSize + 'px';
    try { localStorage.setItem('iv.font', S.fontSize); } catch (e) {}
  }
  function applyWidth(px) {
    S.editorWidth = px || 1080;
    const p = $('#editorPaper');
    if (p) p.style.maxWidth = S.editorWidth + 'px';
    try { localStorage.setItem('iv.width', S.editorWidth); } catch (e) {}
  }
  window.applyTheme = setTheme;
  window.applyFont = applyFont;
  window.applyWidth = applyWidth;

  function showLogin(msg) {
    $('#boot').classList.add('hidden');
    $('#app').classList.add('hidden');
    $('#login').classList.remove('hidden');
    if (msg) $('#loginSub').textContent = msg;
    setTimeout(() => $('#pinInput').focus(), 80);
  }
  window.onAuthRequired = () => { if (!app.loggingIn) showLogin('登录已过期，请重新输入访问码'); };

  async function doLogin(pin) {
    app.loggingIn = true;
    try {
      await Store.login(pin);
      await start();
    } catch (e) {
      Utils.toast(e.message || '访问码不正确', 'error');
      $('#pinInput').select();
    } finally {
      app.loggingIn = false;
    }
  }

  async function start() {
    let boot;
    try {
      boot = await Store.bootstrap();
    } catch (e) {
      if (e.code === 401) { showLogin(); return; }
      showLogin('无法连接服务端');
      $('#loginSub').textContent = e.message;
      return;
    }
    S.boot = boot;
    S.notebooks = boot.notebooks || [];
    S.tags = boot.tags || [];
    S.fields = boot.fields || [];
    S.templates = boot.templates || [];
    S.searches = boot.searches || [];
    S.counts = boot.counts || {};
    S.settings = Object.assign({}, boot.settings, { vaultName: boot.vaultName, publicUrl: boot.publicUrl });
    S.publicUrl = boot.publicUrl || '';
    $('#boot').classList.add('hidden');
    $('#login').classList.add('hidden');
    $('#app').classList.remove('hidden');
    $('#vaultName').textContent = boot.vaultName || '我的知识库';
    document.title = (boot.vaultName || '墨库') + ' · InkVault';
    UI.renderSidebar();
    UI.renderNotebookSelect();
    await refreshList();
    app.ready = true;

    const hash = location.hash || '';
    const m = hash.match(/^#\/note\/(.+)$/);
    if (m) {
      try { await openNote(m[1], true); } catch (e) {}
    } else if (window.innerWidth > 980 && S.notes.length) {
      const last = (() => { try { return localStorage.getItem('iv.lastNote'); } catch (e) { return ''; } })();
      const pick = S.notes.find((n) => n.uid === last) || S.notes[0];
      try { await openNote(pick.uid, true); } catch (e) {}
    }
    if (window.innerWidth <= 980) document.body.classList.remove('editing');

    // deep links: ?panel=stats|toc|import|settings|backup|sync
    const panel = window.__deepPanel || new URLSearchParams(location.search).get('panel');
    if (panel) {
      setTimeout(() => {
        const fn = { stats: 'openStats', toc: 'openTOC', import: 'openImport', settings: 'openSettings', backup: 'openBackup', sync: 'openSync' }[panel];
        if (fn && Panels[fn]) Panels[fn]();
      }, 250);
    }
  }

  async function refreshAll() {
    try {
      const boot = await Store.bootstrap();
      S.boot = boot;
      S.notebooks = boot.notebooks || [];
      S.tags = boot.tags || [];
      S.fields = boot.fields || [];
      S.templates = boot.templates || [];
      S.searches = boot.searches || [];
      S.counts = boot.counts || {};
      S.settings = Object.assign(S.settings, { vaultName: boot.vaultName, publicUrl: boot.publicUrl });
      $('#vaultName').textContent = boot.vaultName || '我的知识库';
    } catch (e) { /* keep old data */ }
    UI.renderSidebar();
    UI.renderNotebookSelect();
    await refreshList();
  }
  window.refreshAll = refreshAll;

  function filterParams() {
    return {
      notebook: S.notebook || '', tag: S.tag || '', q: S.query || '',
      starred: S.starred ? 1 : '', pinned: S.pinned ? 1 : '',
      trashed: S.trashed ? 1 : '', archived: S.archived ? 1 : '',
      type: S.type || '', sort: S.sort || (S.query ? 'relevance' : ''),
      limit: S.limit, offset: S.offset
    };
  }

  async function refreshList(append) {
    const seq = ++S.listSeq;
    S.loading = true;
    UI.renderListHeader();
    if (!append) { S.offset = 0; }
    try {
      const r = await Store.listNotes(filterParams());
      if (seq !== S.listSeq) return;
      S.notes = append ? S.notes.concat(r.notes) : r.notes;
      S.total = r.total;
      S.loading = false;
      UI.renderNotes(false);
      UI.renderListHeader();
      if (S.current) {
        const still = S.notes.find((n) => n.uid === S.current.uid);
        if (!still && !append && S.view !== 'recent') {
          /* keep editor open even when filtered out */
        }
      }
      document.querySelectorAll('.nav-item').forEach((b) => b.classList.toggle('active',
        (b.getAttribute('data-view') === 'all' && !S.notebook && !S.tag && !S.query && !S.starred && !S.pinned && !S.trashed && !S.archived) ||
        (b.getAttribute('data-view') === 'starred' && S.starred) ||
        (b.getAttribute('data-view') === 'pinned' && S.pinned) ||
        (b.getAttribute('data-view') === 'trash' && S.trashed) ||
        (b.getAttribute('data-view') === 'archived' && S.archived)));
    } catch (e) {
      S.loading = false;
      UI.renderListHeader();
      Utils.toast(e.message, 'error');
    }
  }
  window.refreshList = refreshList;

  /* -------------------------------- routing -------------------------------- */
  async function openNote(uid, silent) {
    if (!uid) return;
    if (S.current && S.dirty) await saveCurrent(true);
    try {
      const n = await Store.getNote(uid);
      S.current = n;
      S.dirty = false;
      UI.renderEditor(n);
      UI.renderNotes(false);
      try { localStorage.setItem('iv.lastNote', uid); } catch (e) {}
      if (!silent) {
        try { history.replaceState(null, '', '#/note/' + uid); } catch (e) {}
      }
      document.body.classList.add('editing');
      setTimeout(() => { $('#content').scrollTop = 0; $('#editorScroll').scrollTop = 0; }, 0);
    } catch (e) {
      Utils.toast(e.message, 'error');
    }
  }
  window.openNote = openNote;

  async function createNote(opts) {
    const o = opts || {};
    try {
      const n = await Store.createNote({
        title: o.title || '', content: o.content || '', notebookUid: o.notebookUid || S.notebook || S.notebooks[0] && S.notebooks[0].uid,
        tags: o.tags || []
      });
      await refreshAll();
      await openNote(n.uid);
      setTimeout(() => $('#noteTitle').focus(), 80);
      Utils.toast('已创建笔记', 'success');
      return n;
    } catch (e) { Utils.toast(e.message, 'error'); }
  }
  window.createNote = createNote;

  async function setView(view) {
    if (S.current && S.dirty) await saveCurrent(true);
    S.view = view || 'all';
    S.notebook = null; S.tag = null; S.query = ''; S.type = '';
    S.starred = view === 'starred';
    S.pinned = view === 'pinned';
    S.trashed = view === 'trash';
    S.archived = view === 'archived';
    S.sort = '';
    S.offset = 0;
    UI.renderSidebar();
    await refreshList();
    document.body.classList.remove('editing');
    if (window.innerWidth <= 980) closeSidebar();
  }
  window.setView = setView;

  async function selectNotebook(uid) {
    if (S.current && S.dirty) await saveCurrent(true);
    S.notebook = uid; S.tag = null; S.query = ''; S.view = 'all';
    S.starred = false; S.pinned = false; S.trashed = false; S.archived = false;
    UI.renderSidebar();
    await refreshList();
    if (window.innerWidth <= 980) closeSidebar();
  }

  async function selectTag(name) {
    if (S.current && S.dirty) await saveCurrent(true);
    S.tag = S.tag === name ? null : name;
    S.notebook = null; S.query = ''; S.view = 'all';
    S.starred = false; S.pinned = false; S.trashed = false; S.archived = false;
    UI.renderSidebar();
    await refreshList();
  }

  window.selectTag = selectTag;

  function closeSidebar() {
    document.body.classList.remove('sidebar-open');
  }

  /* -------------------------------- saving --------------------------------- */
  async function saveCurrent(force) {
    if (!S.current || !S.dirty) return;
    const uid = S.current.uid;
    const seq = ++app.saveSeq;
    setSaveState('保存中…', 'dirty');
    const payload = {
      title: $('#noteTitle').value,
      content: $('#content').value,
      notebookUid: $('#notebookSelect').value,
      tags: S.current.tags || [],
      starred: S.current.starred,
      pinned: S.current.pinned,
      status: S.current.status || 'active',
      type: S.current.type || 'note'
    };
    try {
      const n = await Store.updateNote(uid, payload);
      if (seq !== app.saveSeq) return;
      S.current = n;
      S.dirty = false;
      setSaveState('已保存', 'saved');
      $('#saveTime').textContent = '更新于 刚刚';
      $('#metaDates').textContent = '创建于 ' + Utils.fmtDate(n.createdAt) + ' · 更新于 刚刚';
      const i = S.notes.findIndex((x) => x.uid === uid);
      if (i >= 0) {
        S.notes[i] = Object.assign({}, S.notes[i], {
          title: n.title, excerpt: n.excerpt, updatedAt: n.updatedAt, wordCount: n.wordCount,
          notebookName: n.notebookName, tags: n.tags, starred: n.starred, pinned: n.pinned
        });
        UI.renderNotes(false);
      }
      if (S.local) { /* local store persists itself */ }
    } catch (e) {
      setSaveState('保存失败', 'error');
      Utils.toast(e.message, 'error');
    }
  }
  window.saveCurrent = saveCurrent;

  const scheduleSave = Utils.debounce(() => saveCurrent(), 800);

  function markDirty() {
    S.dirty = true;
    setSaveState('未保存', 'dirty');
    scheduleSave();
  }

  /* ------------------------------- editor ---------------------------------- */
  function insertText(before, after, placeholder) {
    const ta = $('#content');
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const value = ta.value;
    const selected = value.slice(start, end) || placeholder || '';
    ta.value = value.slice(0, start) + before + selected + after + value.slice(end);
    ta.selectionStart = start + before.length;
    ta.selectionEnd = start + before.length + selected.length;
    ta.focus();
    markDirty();
    schedulePreview();
    updateEditorStats();
  }

  function toggleLinePrefix(prefix) {
    const ta = $('#content');
    const start = ta.selectionStart;
    const value = ta.value;
    const lineStart = value.lastIndexOf('\n', start - 1) + 1;
    const lineEnd = value.indexOf('\n', start);
    const end = lineEnd === -1 ? value.length : lineEnd;
    const line = value.slice(lineStart, end);
    let replaced;
    if (line.startsWith(prefix)) replaced = line.slice(prefix.length);
    else if (prefix === '- [ ] ' && /^- \[[ x]\] /.test(line)) replaced = line.replace(/^- \[[ x]\] /, '');
    else replaced = prefix + line;
    ta.value = value.slice(0, lineStart) + replaced + value.slice(end);
    ta.selectionStart = lineStart + replaced.length;
    ta.selectionEnd = ta.selectionStart;
    ta.focus();
    markDirty();
    schedulePreview();
  }

  function applyCommand(cmd) {
    const ta = $('#content');
    switch (cmd) {
      case 'bold': return insertText('**', '**', '加粗文字');
      case 'italic': return insertText('*', '*', '斜体');
      case 'strike': return insertText('~~', '~~', '删除线');
      case 'code': return insertText('`', '`', 'code');
      case 'h1': return toggleLinePrefix('# ');
      case 'h2': return toggleLinePrefix('## ');
      case 'h3': return toggleLinePrefix('### ');
      case 'ul': return toggleLinePrefix('- ');
      case 'ol': return toggleLinePrefix('1. ');
      case 'task': return toggleLinePrefix('- [ ] ');
      case 'quote': return toggleLinePrefix('> ');
      case 'divider': return insertText('\n---\n', '', '');
      case 'link': {
        const url = prompt('链接地址', 'https://');
        if (url) insertText('[', '](' + url + ')', '链接文字');
        return;
      }
      case 'image': {
        const input = document.createElement('input');
        input.type = 'file'; input.accept = 'image/*';
        input.onchange = () => { if (input.files[0]) uploadImage(input.files[0]); };
        input.click();
        return;
      }
      case 'table':
        return insertText('\n| 列一 | 列二 |\n| --- | --- |\n| 内容 | 内容 |\n', '', '');
    }
  }

  async function uploadImage(file) {
    if (!S.current) { Utils.toast('请先创建笔记', 'error'); return; }
    try {
      Utils.toast('正在上传图片…');
      const prepared = await Utils.prepareImage(file, 2000);
      const att = await Store.uploadAttachment(S.current.uid, prepared);
      const url = att.url || att.data;
      const ta = $('#content');
      const pos = ta.selectionStart;
      const snippet = `\n![${att.filename || '图片'}](${url})\n`;
      ta.value = ta.value.slice(0, pos) + snippet + ta.value.slice(pos);
      ta.selectionStart = ta.selectionEnd = pos + snippet.length;
      markDirty();
      schedulePreview();
      Utils.toast('图片已插入', 'success');
      if (S.current.attachments) S.current.attachments.push(att);
    } catch (e) { Utils.toast(e.message, 'error'); }
  }

  function schedulePreview() {
    clearTimeout(app.previewTimer);
    app.previewTimer = setTimeout(() => { UI.renderPreview(); }, 180);
  }
  function updateEditorStats() { UI.updateStats(); }

  /* -------------------------------- events --------------------------------- */
  function bind() {
    $('#loginForm').addEventListener('submit', (e) => {
      e.preventDefault();
      doLogin($('#pinInput').value.trim());
    });

    // sidebar navigation
    document.querySelectorAll('[data-view]').forEach((b) => b.onclick = () => setView(b.getAttribute('data-view')));
    $('#notebookTree').addEventListener('click', (e) => {
      const caret = e.target.closest('[data-caret]');
      if (caret) {
        e.stopPropagation();
        const uid = caret.getAttribute('data-caret');
        S.expanded[uid] = S.expanded[uid] === false;
        UI.renderNotebookTree();
        return;
      }
      const menu = e.target.closest('[data-nbmenu]');
      if (menu) {
        e.stopPropagation();
        const uid = menu.getAttribute('data-nbmenu');
        const nb = flattenNotebooks(S.notebooks).find((n) => n.uid === uid);
        Panels.openNotebookMenu(uid, nb ? nb.name : '');
        return;
      }
      const row = e.target.closest('[data-notebook]');
      if (row) selectNotebook(row.getAttribute('data-notebook'));
    });
    $('#tagCloud').addEventListener('contextmenu', (e) => {
      const b = e.target.closest('[data-tag]');
      if (!b) return;
      e.preventDefault();
      const name = b.getAttribute('data-tag');
      const tag = (S.tags || []).find((t) => t.name === name);
      if (tag && window.Panels) Panels.openTagMenu(tag);
    });
    $('#tagCloud').addEventListener('click', (e) => {
      if (e.target.closest('[data-tags-more]')) {
        S.tagsExpanded = !S.tagsExpanded;
        UI.renderTags();
        return;
      }
      const b = e.target.closest('[data-tag]');
      if (b) selectTag(b.getAttribute('data-tag'));
    });
    $('#savedSearches').addEventListener('click', (e) => {
      const b = e.target.closest('[data-search]');
      if (!b) return;
      const s = (S.searches || []).find((x) => x.uid === b.getAttribute('data-search'));
      if (s) { S.query = s.query; S.notebook = null; S.tag = null; S.view = 'all'; S.starred = false; S.pinned = false; S.trashed = false; S.archived = false; refreshList(); }
    });

    $('#addNotebook').onclick = async () => {
      const name = await Dialogs.prompt({ title: '新建笔记本', label: '名称', placeholder: '例如：读书笔记' });
      if (!name) return;
      try {
        await Store.createNotebook({ name, parentUid: S.notebook || '' });
        await refreshAll();
        Utils.toast('已创建笔记本', 'success');
      } catch (e) { Utils.toast(e.message, 'error'); }
    };
    $('#addTag').onclick = async () => {
      const name = await Dialogs.prompt({ title: '新建标签', label: '标签名', placeholder: '例如：灵感' });
      if (!name) return;
      try { await Store.createTag({ name }); await refreshAll(); Utils.toast('已创建标签', 'success'); } catch (e) { Utils.toast(e.message, 'error'); }
    };

    $('#newNoteBtn').onclick = () => createNote();
    $('#emptyNewBtn').onclick = () => createNote();
    $('#mobileNew').onclick = () => createNote();
    $('#sortBtn').onclick = () => Panels.openSort();
    $('#searchTrigger').onclick = () => Panels.openPalette();
    $('#openSidebar').onclick = () => document.body.classList.add('sidebar-open');
    $('#sidebarClose').onclick = closeSidebar;
    $('#backToList').onclick = () => {
      document.body.classList.remove('editing');
      if (history.length > 1) history.back();
    };
    $('#backupBtn').onclick = () => Panels.openBackup();
    $('#syncBtn').onclick = () => Panels.openSync();

    // list
    $('#noteList').addEventListener('click', (e) => {
      const card = e.target.closest('[data-uid]');
      if (card) openNote(card.getAttribute('data-uid'));
    });
    $('#loadMore').onclick = async () => {
      S.offset = S.notes.length;
      const seq = ++S.listSeq;
      const r = await Store.listNotes(filterParams());
      if (seq !== S.listSeq) return;
      S.notes = S.notes.concat(r.notes);
      S.total = r.total;
      UI.renderNotes(false);
      $('#loadMore').classList.toggle('hidden', S.notes.length >= S.total);
    };
    $('#filterChips').addEventListener('click', async (e) => {
      if (e.target.closest('[data-save-search]')) {
        const name = await Dialogs.prompt({ title: '保存搜索', label: '名称', value: S.query || UI.viewLabel(), placeholder: '例如：待读的架构文章' });
        if (!name) return;
        const parts = [];
        if (S.query) parts.push(S.query);
        if (S.tag) parts.push('tag:' + S.tag);
        if (S.starred) parts.push('is:starred');
        if (S.pinned) parts.push('is:pinned');
        try {
          await Store.saveSearch({ name, query: parts.join(' ') || 'is:starred', icon: 'search' });
          await refreshAll();
          Utils.toast('已保存搜索', 'success');
        } catch (err) { Utils.toast(err.message, 'error'); }
        return;
      }
      const b = e.target.closest('[data-clear]');
      if (!b) return;
      const k = b.getAttribute('data-clear');
      if (k === 'notebook') { S.notebook = null; }
      if (k === 'tag') { S.tag = null; }
      if (k === 'query') { S.query = ''; }
      if (k === 'starred') { S.starred = false; }
      if (k === 'pinned') { S.pinned = false; }
      if (k === 'type') { S.type = ''; }
      UI.renderSidebar();
      refreshList();
    });

    // editor
    $('#content').addEventListener('input', () => {
      markDirty();
      updateEditorStats();
      schedulePreview();
    });
    $('#noteTitle').addEventListener('input', markDirty);
    $('#noteTitle').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); $('#content').focus(); }
    });
    $('#content').addEventListener('keydown', (e) => {
      const meta = e.metaKey || e.ctrlKey;
      if (meta && e.key.toLowerCase() === 'b') { e.preventDefault(); applyCommand('bold'); return; }
      if (meta && e.key.toLowerCase() === 'i') { e.preventDefault(); applyCommand('italic'); return; }
      if (meta && e.key.toLowerCase() === 's') { e.preventDefault(); saveCurrent(true); return; }
      if (meta && e.key.toLowerCase() === 'k') { e.preventDefault(); applyCommand('link'); return; }
      if (e.key === 'Tab') {
        e.preventDefault();
        const ta = e.currentTarget;
        if (e.shiftKey) {
          const before = ta.value.slice(0, ta.selectionStart);
          if (before.endsWith('  ')) {
            ta.value = ta.value.slice(0, ta.selectionStart - 2) + ta.value.slice(ta.selectionEnd);
            ta.selectionStart = ta.selectionEnd = ta.selectionStart - 2;
          }
        } else {
          const pos = ta.selectionStart;
          ta.value = ta.value.slice(0, pos) + '  ' + ta.value.slice(ta.selectionEnd);
          ta.selectionStart = ta.selectionEnd = pos + 2;
        }
        markDirty();
        return;
      }
      if (e.key === 'Enter') {
        const ta = e.currentTarget;
        const start = ta.selectionStart;
        const line = ta.value.slice(ta.value.lastIndexOf('\n', start - 1) + 1, start);
        const m = line.match(/^(\s*)([-*+]|\d+[.)])\s+(\[[ xX]\]\s+)?(.*)$/);
        if (m && m[4].trim() !== '') {
          e.preventDefault();
          let marker = m[2];
          if (/^\d/.test(marker)) marker = (parseInt(marker, 10) + 1) + '.';
          const prefix = '\n' + m[1] + marker + ' ' + (m[3] ? '[ ] ' : '');
          ta.value = ta.value.slice(0, start) + prefix + ta.value.slice(ta.selectionEnd);
          ta.selectionStart = ta.selectionEnd = start + prefix.length;
          markDirty();
          schedulePreview();
          updateEditorStats();
        } else if (m && m[4].trim() === '') {
          e.preventDefault();
          ta.value = ta.value.slice(0, start - line.length) + '\n' + ta.value.slice(ta.selectionEnd);
          ta.selectionStart = ta.selectionEnd = start - line.length + 1;
          markDirty();
        }
      }
    });
    $('#content').addEventListener('paste', async (e) => {
      const items = Array.from(e.clipboardData.items || []);
      const img = items.find((i) => i.type && i.type.startsWith('image/'));
      if (img) { e.preventDefault(); const f = img.getAsFile(); if (f) uploadImage(f); }
    });
    $('#content').addEventListener('dragover', (e) => e.preventDefault());
    $('#content').addEventListener('drop', (e) => {
      const f = e.dataTransfer.files && e.dataTransfer.files[0];
      if (f && /^image\//.test(f.type)) { e.preventDefault(); uploadImage(f); }
    });

    document.querySelectorAll('[data-md]').forEach((b) => b.onclick = () => applyCommand(b.getAttribute('data-md')));
    document.querySelectorAll('[data-viewmode]').forEach((b) => b.onclick = () => { UI.setViewMode(b.getAttribute('data-viewmode')); localStorage.setItem('iv.viewMode', b.getAttribute('data-viewmode')); });
    $('#outlineBtn').onclick = () => {
      S.outline = !S.outline;
      $('#outlineBtn').classList.toggle('active', S.outline);
      UI.renderOutline();
      localStorage.setItem('iv.outline', S.outline ? '1' : '0');
    };
    $('#outline').addEventListener('click', (e) => {
      const a = e.target.closest('[data-outline]');
      if (!a) return;
      e.preventDefault();
      const target = document.getElementById(a.getAttribute('data-outline'));
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
    $('#preview').addEventListener('click', (e) => {
      const copy = e.target.closest('.copy-code');
      if (copy) {
        const code = copy.parentElement.querySelector('code').innerText;
        Utils.copy(code).then(() => Utils.toast('代码已复制', 'success'));
        return;
      }
      const link = e.target.closest('.note-link');
      if (link) { e.preventDefault(); openNote(link.getAttribute('data-note')); }
    });

    $('#notebookSelect').onchange = () => {
      if (!S.current) return;
      S.current.notebookUid = $('#notebookSelect').value;
      markDirty();
      saveCurrent(true);
    };

    // tags in editor
    $('#tagEditor').addEventListener('click', async (e) => {
      const rm = e.target.closest('[data-remove-tag]');
      if (rm && S.current) {
        const t = rm.getAttribute('data-remove-tag');
        S.current.tags = (S.current.tags || []).filter((x) => x !== t);
        $('#tagEditor').innerHTML = UI.tagChipsHtml(S.current);
        markDirty(); saveCurrent(true);
        return;
      }
      if (e.target.id === 'tagInput') return;
      const input = $('#tagInput');
      if (input) input.focus();
    });
    $('#tagEditor').addEventListener('keydown', async (e) => {
      if (!e.target.classList.contains('tag-input')) return;
      if (e.key === 'Enter' || e.key === ',' || e.key === '，') {
        e.preventDefault();
        const name = e.target.value.trim().replace(/^#/, '');
        if (!name || !S.current) return;
        S.current.tags = Array.from(new Set([...(S.current.tags || []), name]));
        $('#tagEditor').innerHTML = UI.tagChipsHtml(S.current);
        markDirty();
        try { await saveCurrent(true); await refreshAll(); } catch (err) {}
        setTimeout(() => { const i = $('#tagInput'); if (i) i.focus(); }, 50);
      }
      if (e.key === 'Backspace' && !e.target.value && S.current.tags && S.current.tags.length) {
        S.current.tags.pop();
        $('#tagEditor').innerHTML = UI.tagChipsHtml(S.current);
        markDirty(); saveCurrent(true);
      }
    });

    $('#fieldEditor').addEventListener('change', async () => {
      if (!S.current) return;
      const values = {};
      $$('#fieldEditor [data-field]').forEach((el) => {
        values[el.getAttribute('data-field')] = el.type === 'checkbox' ? String(el.checked) : el.value;
      });
      try {
        await Store.setFields(S.current.uid, values);
        S.current.fields = values;
        Utils.toast('属性已保存', 'success');
      } catch (e) { Utils.toast(e.message, 'error'); }
    });

    $('#starBtn').onclick = async () => {
      if (!S.current) return;
      S.current.starred = !S.current.starred;
      $('#starBtn').classList.toggle('active', S.current.starred);
      await Store.patchNote(S.current.uid, { starred: S.current.starred });
      await refreshAll();
    };
    $('#pinBtn').onclick = async () => {
      if (!S.current) return;
      S.current.pinned = !S.current.pinned;
      $('#pinBtn').classList.toggle('active', S.current.pinned);
      await Store.patchNote(S.current.uid, { pinned: S.current.pinned });
      await refreshAll();
    };
    $('#versionsBtn').onclick = () => Panels.openVersions();
    $('#exportNoteBtn').onclick = () => Panels.openNoteMenu();
    $('#moreBtn').onclick = () => Panels.openNoteMenu();

    // mobile nav
    document.querySelectorAll('#mobileBar [data-nav]').forEach((b) => b.onclick = () => {
      const nav = b.getAttribute('data-nav');
      if (nav === 'notes') setView('all');
      if (nav === 'search') Panels.openPalette();
      if (nav === 'stats') Panels.openStats();
      if (nav === 'settings') Panels.openSettings();
    });
    document.querySelectorAll('.rail-btn[data-nav]').forEach((b) => b.onclick = () => {
      const nav = b.getAttribute('data-nav');
      if (nav === 'notes') setView('all');
      if (nav === 'search') Panels.openPalette();
      if (nav === 'stats') Panels.openStats();
      if (nav === 'import') Panels.openImport();
      if (nav === 'settings') Panels.openSettings();
    });

    // overlay scrim for mobile sidebar
    const scrim = document.createElement('div');
    scrim.className = 'scrim';
    document.body.appendChild(scrim);
    scrim.onclick = closeSidebar;

    // global keys
    document.addEventListener('keydown', (e) => {
      const meta = e.metaKey || e.ctrlKey;
      const inEditor = document.activeElement && ['TEXTAREA', 'INPUT'].includes(document.activeElement.tagName);
      if (meta && e.key.toLowerCase() === 'k') { e.preventDefault(); Panels.openPalette(); return; }
      if (meta && e.key.toLowerCase() === 'n') { e.preventDefault(); createNote(); return; }
      if (meta && e.key.toLowerCase() === 's') { e.preventDefault(); saveCurrent(true); return; }
      if (meta && e.key.toLowerCase() === 'e') { e.preventDefault(); Panels.openTOC(); return; }
      if (e.key === 'Escape') {
        if (!$('#modalRoot').classList.contains('hidden')) Modal.close();
        else if (!$('#palette').classList.contains('hidden')) Panels.closePalette();
        else if (!$('#drawer').classList.contains('hidden')) Modal.closeDrawer();
        else if (document.body.classList.contains('sidebar-open')) closeSidebar();
        else if (document.body.classList.contains('editing') && window.innerWidth <= 980) document.body.classList.remove('editing');
      }
    });

    window.addEventListener('hashchange', () => {
      const m = location.hash.match(/^#\/note\/(.+)$/);
      if (m) openNote(m[1], true);
    });

    window.addEventListener('beforeunload', (e) => {
      if (S.dirty) { saveCurrent(true); e.preventDefault(); e.returnValue = ''; }
    });

    window.addEventListener('online', () => { S.online = true; });
    window.addEventListener('offline', () => { S.online = false; });
  }

  /* --------------------------------- init ---------------------------------- */
  async function init() {
    hydrateIcons();
    const mark = () => {
      document.documentElement.setAttribute('data-viewport', window.innerWidth + 'x' + window.innerHeight);
      const lp = document.getElementById('listPane');
      if (lp) document.documentElement.setAttribute('data-listwidth', String(Math.round(lp.getBoundingClientRect().width)));
      const app = document.getElementById('app');
      if (app) document.documentElement.setAttribute('data-appwidth', String(Math.round(app.getBoundingClientRect().width)));
    };
    mark(); setTimeout(mark, 500); setTimeout(mark, 1500);
    setTheme(localStorage.getItem('iv.theme') || 'light');
    applyFont(+localStorage.getItem('iv.font') || 15);
    applyWidth(+localStorage.getItem('iv.width') || 1080);
    S.outline = localStorage.getItem('iv.outline') === '1';
    $('#outlineBtn').classList.toggle('active', S.outline);
    let savedMode = localStorage.getItem('iv.viewMode');
    if (!savedMode && window.innerWidth <= 980) savedMode = 'edit';
    if (savedMode) UI.setViewMode(savedMode);
    bind();

    if (S.local) {
      $('#loginSub').textContent = '离线模式 · 数据保存在本机';
      $('#login').classList.add('hidden');
      try {
        await start();
      } catch (e) {
        document.querySelector('.boot-text').textContent = '本地存储初始化失败：' + e.message;
        Utils.toast(e.message, 'error');
      }
      return;
    }

    // PIN passed from the desktop launcher: log in transparently
    const params = new URLSearchParams(location.search);
    window.__deepPanel = params.get('panel');
    const pin = params.get('pin');
    if (pin) {
      history.replaceState(null, '', location.pathname + location.hash);
      await doLogin(pin);
      return;
    }
    try {
      const ping = await fetch('/api/ping').then((r) => r.json());
      if (ping && ping.app) {
        await start();
        return;
      }
    } catch (e) {}
    showLogin();
  }

  document.addEventListener('DOMContentLoaded', init);
})();
