/* 移动端离线模式：IndexedDB 本地库 + 与桌面端合并同步 */
(function () {
  const isLocal = location.protocol === 'file:' || /(\?|&)local=1/.test(location.search) || window.INKVAULT_LOCAL === true;
  if (!isLocal) { window.LocalStore = null; return; }

  const DB_NAME = 'inkvault';
  const STORE = 'vault';
  const KEY = 'data';
  let dbp = null;
  let vault = null;
  let saveTimer = null;

  const now = () => new Date().toISOString().replace('Z', 'Z');
  const uid = (p) => p + '-' + Math.random().toString(36).slice(2, 11);

  function openDB() {
    if (dbp) return dbp;
    dbp = new Promise((resolve, reject) => {
      if (!window.indexedDB) { reject(new Error('no idb')); return; }
      const req = indexedDB.open(DB_NAME, 1);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE);
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
    return dbp;
  }

  function withTimeout(promise, ms, fallback) {
    return Promise.race([
      promise,
      new Promise((resolve) => setTimeout(() => resolve(fallback), ms))
    ]);
  }

  async function idbGet() {
    try {
      const db = await withTimeout(openDB(), 1500, null);
      if (!db) throw new Error('idb timeout');
      return await withTimeout(new Promise((resolve) => {
        const tx = db.transaction(STORE, 'readonly');
        const req = tx.objectStore(STORE).get(KEY);
        req.onsuccess = () => resolve(req.result || null);
        req.onerror = () => resolve(null);
      }), 1500, null);
    } catch (e) {
      const raw = localStorage.getItem('inkvault.data');
      return raw ? JSON.parse(raw) : null;
    }
  }

  async function idbSet(data) {
    try {
      const db = await withTimeout(openDB(), 1500, null);
      if (!db) throw new Error('idb timeout');
      await withTimeout(new Promise((resolve, reject) => {
        const tx = db.transaction(STORE, 'readwrite');
        tx.objectStore(STORE).put(data, KEY);
        tx.oncomplete = resolve;
        tx.onerror = () => reject(tx.error);
      }), 1500, null);
    } catch (e) {
      try { localStorage.setItem('inkvault.data', JSON.stringify(data)); } catch (err) { Utils.toast('本地存储空间不足', 'error'); }
    }
  }

  function seed() {
    const t = now();
    const welcome = `# 欢迎使用墨库

这是手机上的**离线知识库**。所有内容保存在本机，不需要网络也能写。

## 你可以

- 新建笔记，用 Markdown 记录
- 用笔记本和标签整理
- 在「统计」里查看写作数据
- 在「设置 → 设备同步」连回电脑上的墨库，双向同步

## 搜索语法

- \`tag:想法\` 按标签
- \`notebook:工作\` 按笔记本
- \`is:starred\` 星标笔记

> 数据能带走，才是真的属于你。
`;
    return {
      version: 1,
      notebooks: [{ uid: 'nb-inbox', name: '收集箱', parentUid: '', icon: 'inbox', color: '#a8442f', sort: 0, createdAt: t, updatedAt: t }],
      notes: [{
        uid: uid('nt'), title: '欢迎使用墨库', content: welcome, excerpt: storeExcerpt(welcome), notebookUid: 'nb-inbox',
        type: 'note', status: 'active', pinned: false, starred: false, wordCount: countWords(welcome),
        createdAt: t, updatedAt: t, deletedAt: '', tags: ['开始'], attachments: [], versions: []
      }],
      tags: [{ uid: uid('tg'), name: '开始', color: '' }],
      fields: [], templates: [], searches: [],
      settings: { vaultName: '我的笔记', publicUrl: '' },
      snapshots: []
    };
  }

  function storeExcerpt(md) {
    const s = String(md || '')
      .replace(/```[\s\S]*?```/g, ' ')
      .replace(/^#{1,6}\s.*$/gm, ' ')
      .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
      .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/[*_~`>#|]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
    return s.length > 180 ? s.slice(0, 180) + '…' : s;
  }
  function countWords(text) {
    const s = String(text || '');
    const cjk = (s.match(/[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]/g) || []).length;
    const latin = (s.replace(/[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]/g, ' ').match(/[A-Za-z0-9]+/g) || []).length;
    return cjk + latin;
  }

  async function load() {
    if (vault) return vault;
    vault = await idbGet();
    if (!vault || !vault.notes) {
      vault = seed();
      await idbSet(vault);
    }
    vault.snapshots = vault.snapshots || [];
    return vault;
  }

  function save() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => { if (vault) idbSet(vault); }, 120);
  }
  async function saveNow() { if (vault) await idbSet(vault); }

  function notebookPath(uidVal) {
    const map = {};
    vault.notebooks.forEach((n) => { map[n.uid] = n; });
    const parts = [];
    let cur = map[uidVal];
    let guard = 0;
    while (cur && guard++ < 20) { parts.unshift(cur.name); cur = map[cur.parentUid]; }
    return parts.join(' / ');
  }

  function noteTags(n) { return n.tags || []; }

  function ensureTag(name) {
    name = String(name || '').trim().replace(/^#/, '');
    if (!name) return null;
    let t = vault.tags.find((x) => x.name === name);
    if (!t) { t = { uid: uid('tg'), name, color: '' }; vault.tags.push(t); }
    return t;
  }

  function visibleNotes() { return vault.notes.filter((n) => !n.deletedAt); }

  function notebookTree() {
    const map = {};
    vault.notebooks.forEach((n) => { map[n.uid] = Object.assign({}, n, { children: [], count: 0, total: 0 }); });
    const roots = [];
    Object.values(map).forEach((n) => {
      if (n.parentUid && map[n.parentUid]) map[n.parentUid].children.push(n);
      else roots.push(n);
    });
    visibleNotes().forEach((n) => { if (map[n.notebookUid]) map[n.notebookUid].count++; });
    const walk = (n) => {
      let total = n.count;
      n.children.forEach((c) => { total += walk(c); });
      n.total = total;
      return total;
    };
    roots.forEach(walk);
    return roots;
  }

  function parseQuery(q) {
    let text = [], tag = null, notebook = null, starred = false, trashed = false;
    String(q || '').split(/\s+/).forEach((w) => {
      if (/^tag:/i.test(w)) tag = w.slice(4);
      else if (/^#/.test(w)) tag = w.slice(1);
      else if (/^notebook:/i.test(w)) notebook = w.slice(9);
      else if (w === 'is:starred') starred = true;
      else if (w === 'is:trashed') trashed = true;
      else if (w) text.push(w);
    });
    return { text: text.join(' '), tag, notebook, starred, trashed };
  }

  function listNotes(params) {
    const p = params || {};
    const q = parseQuery(p.q);
    let list = vault.notes.slice();
    const trashed = p.trashed || q.trashed;
    list = list.filter((n) => (trashed ? !!n.deletedAt : !n.deletedAt));
    if (!trashed) {
      if (p.archived) list = list.filter((n) => n.status === 'archived');
      else list = list.filter((n) => n.status !== 'archived');
    }
    if (p.starred) list = list.filter((n) => n.starred);
    if (p.pinned) list = list.filter((n) => n.pinned);
    if (p.notebook) {
      const kids = new Set([p.notebook]);
      let changed = true;
      while (changed) {
        changed = false;
        vault.notebooks.forEach((n) => { if (kids.has(n.parentUid) && !kids.has(n.uid)) { kids.add(n.uid); changed = true; } });
      }
      list = list.filter((n) => kids.has(n.notebookUid));
    }
    if (p.tag || q.tag) {
      const tag = p.tag || q.tag;
      list = list.filter((n) => noteTags(n).includes(tag));
    }
    if (q.notebook) {
      const nb = vault.notebooks.find((n) => n.name === q.notebook);
      if (nb) list = list.filter((n) => n.notebookUid === nb.uid);
    }
    if (p.starred || q.starred) list = list.filter((n) => n.starred);
    if (p.type) list = list.filter((n) => n.type === p.type);
    const terms = (p.q && !q.text ? [] : q.text.split(/\s+/)).filter(Boolean);
    if (terms.length) {
      list = list.filter((n) => terms.every((t) => (n.title + '\n' + n.content).toLowerCase().includes(t.toLowerCase())));
    }
    const term = q.text;
    if (term) {
      const re = new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
      list.sort((a, b) => {
        const score = (n) => ((n.title.match(re) || []).length * 4 + (n.content.match(re) || []).length);
        return score(b) - score(a) || b.updatedAt.localeCompare(a.updatedAt);
      });
    } else if (p.sort === 'title') list.sort((a, b) => a.title.localeCompare(b.title, 'zh'));
    else if (p.sort === 'words') list.sort((a, b) => b.wordCount - a.wordCount);
    else if (p.sort === 'created_desc') list.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    else if (p.sort === 'oldest') list.sort((a, b) => a.updatedAt.localeCompare(b.updatedAt));
    else list.sort((a, b) => (b.pinned ? 1 : 0) - (a.pinned ? 1 : 0) || b.updatedAt.localeCompare(a.updatedAt));

    const total = list.length;
    const offset = p.offset || 0;
    const limit = p.limit || 60;
    const page = list.slice(offset, offset + limit).map((n) => ({
      uid: n.uid, title: n.title, excerpt: n.excerpt, notebookUid: n.notebookUid,
      notebookName: notebookPath(n.notebookUid), type: n.type, status: n.status,
      pinned: !!n.pinned, starred: !!n.starred, wordCount: n.wordCount,
      tags: noteTags(n), createdAt: n.createdAt, updatedAt: n.updatedAt, deletedAt: n.deletedAt || ''
    }));
    return { notes: page, total, limit, offset };
  }

  function noteDetail(n) {
    return Object.assign({}, n, {
      notebookName: notebookPath(n.notebookUid),
      tags: noteTags(n),
      attachments: (n.attachments || []).map((a) => ({ uid: a.uid, filename: a.filename, mime: a.mime, size: a.size || 0, url: a.url || '' })),
      versionCount: (n.versions || []).length
    });
  }

  function countWordsNow(n) { n.wordCount = countWords(n.content); n.excerpt = storeExcerpt(n.content); }

  const LocalStore = {
    kind: 'local',
    async bootstrap() {
      await load();
      const counts = { all: 0, starred: 0, trash: 0, archived: 0, pinned: 0 };
      visibleNotes().forEach((n) => {
        if (n.status === 'archived') counts.archived++;
        else counts.all++;
        if (n.starred) counts.starred++;
        if (n.pinned) counts.pinned++;
      });
      counts.trash = vault.notes.filter((n) => n.deletedAt).length;
      return {
        app: 'InkVault', version: '1.0.0', vaultName: vault.settings.vaultName || '我的笔记',
        publicUrl: vault.settings.publicUrl || '', baseUrl: '本地', notebooks: notebookTree(),
        notebookTree: flatten(notebookTree()), tags: vault.tags.map((t) => Object.assign({}, t, { count: visibleNotes().filter((n) => noteTags(n).includes(t.name)).length })).filter((t) => t.count),
        fields: vault.fields, templates: vault.templates, searches: vault.searches,
        counts, stats: computeStats(), settings: vault.settings, auth: false,
        backupKeep: 3, autoBackup: false, maxUploadMb: 20, now: now(), lanUrls: [], pinSet: false,
        fts: true, local: true
      };
    },
    async login() { return { ok: true }; },
    async logout() { location.reload(); },
    async listNotes(p) { await load(); return listNotes(p); },
    async getNote(uidVal) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      if (!n) throw new Error('笔记不存在');
      return noteDetail(n);
    },
    async createNote(data) {
      await load();
      const t = now();
      const n = {
        uid: uid('nt'), title: data.title || '', content: data.content || '',
        notebookUid: data.notebookUid || vault.notebooks[0].uid, type: data.type || 'note', status: 'active',
        pinned: !!data.pinned, starred: !!data.starred, tags: data.tags || [],
        attachments: [], versions: [], createdAt: t, updatedAt: t, deletedAt: ''
      };
      if (!n.title) n.title = (n.content.split('\n').find((l) => l.trim()) || '无标题笔记').replace(/^#+\s*/, '').slice(0, 60);
      (n.tags || []).forEach(ensureTag);
      countWordsNow(n);
      vault.notes.unshift(n);
      save();
      return noteDetail(n);
    },
    async updateNote(uidVal, data) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      if (!n) throw new Error('笔记不存在');
      if (data.content !== undefined && data.content !== n.content) {
        n.versions = n.versions || [];
        n.versions.unshift({ id: Date.now(), title: n.title, content: n.content, wordCount: n.wordCount, reason: 'edit', createdAt: now() });
        n.versions = n.versions.slice(0, 12);
      }
      ['title', 'content', 'notebookUid', 'type', 'status'].forEach((k) => { if (data[k] !== undefined) n[k] = data[k]; });
      ['pinned', 'starred'].forEach((k) => { if (data[k] !== undefined) n[k] = !!data[k]; });
      if (data.tags) { n.tags = data.tags; data.tags.forEach(ensureTag); }
      n.updatedAt = now();
      countWordsNow(n);
      save();
      return noteDetail(n);
    },
    async patchNote(uidVal, patch) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      if (!n) throw new Error('笔记不存在');
      Object.assign(n, patch);
      n.updatedAt = now();
      save();
      return noteDetail(n);
    },
    async deleteNote(uidVal) { await load(); const n = vault.notes.find((x) => x.uid === uidVal); if (n) { n.deletedAt = now(); save(); } return { ok: true }; },
    async restoreNote(uidVal) { await load(); const n = vault.notes.find((x) => x.uid === uidVal); if (n) { n.deletedAt = ''; save(); } return { ok: true }; },
    async purgeNote(uidVal) { await load(); vault.notes = vault.notes.filter((x) => x.uid !== uidVal); save(); return { ok: true }; },
    async duplicateNote(uidVal) {
      await load();
      const src = vault.notes.find((x) => x.uid === uidVal);
      const copy = JSON.parse(JSON.stringify(src));
      copy.uid = uid('nt'); copy.title = src.title + ' 副本'; copy.createdAt = now(); copy.updatedAt = now();
      copy.versions = []; vault.notes.unshift(copy); save();
      return noteDetail(copy);
    },
    async versions(uidVal, withContent) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      return ((n && n.versions) || []).map((v) => ({ id: v.id, title: v.title, content: withContent ? v.content : undefined, wordCount: v.wordCount, reason: v.reason, createdAt: v.createdAt }));
    },
    async restoreVersion(uidVal, id) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      const v = (n.versions || []).find((x) => String(x.id) === String(id));
      if (!v) throw new Error('版本不存在');
      n.versions.unshift({ id: Date.now(), title: n.title, content: n.content, wordCount: n.wordCount, reason: 'before-restore', createdAt: now() });
      n.title = v.title; n.content = v.content; n.updatedAt = now(); countWordsNow(n); save();
      return noteDetail(n);
    },
    async setFields() { return { ok: true }; },
    async uploadAttachment(uidVal, file) {
      await load();
      const n = vault.notes.find((x) => x.uid === uidVal);
      if (!n) throw new Error('请先保存笔记');
      const data = await new Promise((res) => { const r = new FileReader(); r.onload = () => res(r.result); r.readAsDataURL(file); });
      const att = { uid: uid('at'), filename: file.name || '图片', mime: file.type || 'image/png', size: file.size, url: data, data };
      n.attachments = n.attachments || [];
      n.attachments.push(att);
      save();
      return att;
    },
    async listNotebooks() { await load(); return { notebooks: notebookTree(), options: flatten(notebookTree()) }; },
    async createNotebook(data) {
      await load();
      const t = now();
      const nb = { uid: uid('nb'), name: data.name || '未命名笔记本', parentUid: data.parentUid || '', icon: '', color: data.color || '', sort: 0, createdAt: t, updatedAt: t };
      vault.notebooks.push(nb); save(); return nb;
    },
    async updateNotebook(uidVal, data) {
      await load();
      const nb = vault.notebooks.find((x) => x.uid === uidVal);
      if (nb) { Object.assign(nb, data, { updatedAt: now() }); save(); }
      return nb;
    },
    async deleteNotebook(uidVal) {
      await load();
      const nb = vault.notebooks.find((x) => x.uid === uidVal);
      const parent = nb ? nb.parentUid || vault.notebooks[0].uid : vault.notebooks[0].uid;
      vault.notes.forEach((n) => { if (n.notebookUid === uidVal) n.notebookUid = parent; });
      vault.notebooks = vault.notebooks.filter((x) => x.uid !== uidVal);
      save(); return { ok: true };
    },
    async listTags() {
      await load();
      return vault.tags.map((t) => Object.assign({}, t, { count: visibleNotes().filter((n) => noteTags(n).includes(t.name)).length })).sort((a, b) => b.count - a.count);
    },
    async createTag(data) { await load(); const t = ensureTag(data.name); save(); return t || {}; },
    async updateTag(uidVal, data) {
      await load();
      const t = vault.tags.find((x) => x.uid === uidVal);
      if (t && data.name) {
        const old = t.name;
        t.name = data.name;
        vault.notes.forEach((n) => { n.tags = (n.tags || []).map((x) => x === old ? data.name : x); });
      }
      save(); return { ok: true };
    },
    async deleteTag(uidVal) {
      await load();
      const t = vault.tags.find((x) => x.uid === uidVal);
      if (t) vault.notes.forEach((n) => { n.tags = (n.tags || []).filter((x) => x !== t.name); });
      vault.tags = vault.tags.filter((x) => x.uid !== uidVal);
      save(); return { ok: true };
    },
    async listFields() { await load(); return vault.fields; },
    async saveField(f) { await load(); vault.fields.push(f); save(); return f; },
    async deleteField(uidVal) { await load(); vault.fields = vault.fields.filter((x) => x.uid !== uidVal); save(); return { ok: true }; },
    async listTemplates() { await load(); return vault.templates; },
    async saveTemplate(t) { await load(); vault.templates.push(t); save(); return t; },
    async deleteTemplate(uidVal) { await load(); vault.templates = vault.templates.filter((x) => x.uid !== uidVal); save(); return { ok: true }; },
    async listSearches() { await load(); return vault.searches; },
    async saveSearch(s) { await load(); vault.searches.push(s); save(); return s; },
    async deleteSearch(uidVal) { await load(); vault.searches = vault.searches.filter((x) => x.uid !== uidVal); save(); return { ok: true }; },
    async emptyTrash() { await load(); vault.notes = vault.notes.filter((n) => !n.deletedAt); save(); return { ok: true, deleted: 0 }; },
    async stats() { await load(); return computeStats(); },
    async statsNote() {
      await load();
      const st = computeStats();
      const md = `# 数据概览 · ${new Date().toISOString().slice(0, 7)}\n\n> 由墨库自动生成于 ${new Date().toLocaleString('zh-CN')}\n\n` +
        `- 笔记：${st.activeNotes}\n- 总字数：${st.totalWords}\n- 笔记本：${st.notebookCount}\n- 标签：${st.tagCount}\n- 连续记录：${st.streak} 天\n\n## 笔记本分布\n\n` +
        st.byNotebook.map((n) => `- ${n.name}：${n.count} 篇`).join('\n') + '\n\n## 高频标签\n\n' +
        st.byTag.slice(0, 12).map((t) => `- #${t.name}：${t.count} 篇`).join('\n');
      return LocalStore.createNote({ title: '数据概览 · ' + new Date().toISOString().slice(0, 7), content: md, tags: ['统计'] });
    },
    async makeIndexNote(notebookUid) {
      await load();
      const notes = listNotes({ notebook: notebookUid, limit: 500, sort: 'title' }).notes;
      const content = `# 目录 · ${notebookPath(notebookUid)}\n\n> 自动汇总 · 共 ${notes.length} 篇 · ${new Date().toLocaleDateString('zh-CN')}\n\n` +
        notes.map((n) => `- [${n.title}](#/note/${n.uid}) · ${n.wordCount} 字`).join('\n');
      return LocalStore.createNote({ title: '目录 · ' + notebookPath(notebookUid), content, notebookUid, tags: ['目录'] });
    },
    async settings() {
      await load();
      return Object.assign({}, vault.settings, { version: '1.0.0', port: 0, lanUrls: [], pin: '', token: '', requireAuth: false, fts: true, dbPath: '本机 IndexedDB', filesDir: '本机 IndexedDB' });
    },
    async saveSettings(data) {
      await load();
      ['vaultName', 'publicUrl'].forEach((k) => { if (data[k] !== undefined) vault.settings[k] = data[k]; });
      save(); return { ok: true };
    },
    async audit() { return []; },
    async reindex() { return { ok: true }; },
    async backups() {
      await load();
      return { backups: vault.snapshots.map((s) => ({ name: s.name, size: s.size, createdAt: s.createdAt, reason: s.reason })), dir: '本机快照' };
    },
    async createBackup() {
      await load();
      const data = JSON.stringify(vault);
      const snap = { name: 'local-' + new Date().toISOString().slice(0, 16).replace(/[:T]/g, ''), createdAt: now(), reason: 'manual', size: data.length, data };
      vault.snapshots.unshift(snap);
      vault.snapshots = vault.snapshots.slice(0, 3);
      await saveNow();
      return snap;
    },
    async deleteBackup(name) { await load(); vault.snapshots = vault.snapshots.filter((s) => s.name !== name); save(); return { ok: true }; },
    async restoreBackup(name) {
      await load();
      const s = vault.snapshots.find((x) => x.name === name);
      if (!s) throw new Error('快照不存在');
      vault = JSON.parse(s.data); await saveNow(); return { ok: true };
    },
    downloadBackup(name) {
      const s = (vault.snapshots || []).find((x) => x.name === name);
      if (s) Utils.downloadBlob(new Blob([s.data], { type: 'application/json' }), name + '.json');
    },
    async tocPreview(params) {
      await load();
      const data = buildTOC(params);
      return { doc: data, total: data.totalNotes, preview: tocMarkdown(data) };
    },
    exportTOC(params) {
      const data = buildTOC(params);
      const fmt = params.format || 'md';
      if (fmt === 'md') return Utils.downloadBlob(new Blob([tocMarkdown(data)], { type: 'text/markdown' }), '目录.md');
      if (fmt === 'html') return Utils.downloadBlob(new Blob([tocHTML(data)], { type: 'text/html' }), '目录.html');
      if (fmt === 'csv') return Utils.downloadBlob(new Blob(['\ufeff' + tocCSV(data)], { type: 'text/csv' }), '目录.csv');
      if (fmt === 'json') return Utils.downloadBlob(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }), '目录.json');
      Utils.toast('手机端暂不支持 ' + fmt + '，请连接电脑导出', 'error');
    },
    exportStats(format) {
      const st = computeStats();
      if (format === 'json') return Utils.downloadBlob(new Blob([JSON.stringify(st, null, 2)], { type: 'application/json' }), 'stats.json');
      if (format === 'csv') {
        const rows = [['指标', '数值'], ['笔记', st.activeNotes], ['总字数', st.totalWords], ['笔记本', st.notebookCount], ['标签', st.tagCount], ['连续记录', st.streak]];
        return Utils.downloadBlob(new Blob(['\ufeff' + rows.map((r) => r.join(',')).join('\r\n')], { type: 'text/csv' }), '统计.csv');
      }
      const md = `# 数据概览\n\n- 笔记：${st.activeNotes}\n- 总字数：${st.totalWords}\n- 笔记本：${st.notebookCount}\n- 标签：${st.tagCount}\n- 连续记录：${st.streak} 天\n`;
      Utils.downloadBlob(new Blob([format === 'html' ? tocHTML({ title: '数据概览', totalNotes: st.activeNotes, totalWords: st.totalWords, groups: [], generatedAt: now() }) : md], { type: 'text/plain' }), '统计.' + format);
    },
    exportNote(uidVal, format) {
      const n = vault.notes.find((x) => x.uid === uidVal);
      if (!n) return;
      if (format === 'md') return Utils.downloadBlob(new Blob([n.content], { type: 'text/markdown' }), (n.title || 'note') + '.md');
      if (format === 'txt') return Utils.downloadBlob(new Blob([n.content.replace(/[*_`#>]/g, '')], { type: 'text/plain' }), (n.title || 'note') + '.txt');
      if (format === 'json') return Utils.downloadBlob(new Blob([JSON.stringify(noteDetail(n), null, 2)], { type: 'application/json' }), (n.title || 'note') + '.json');
      if (format === 'html') return Utils.downloadBlob(new Blob([localNoteHTML(n)], { type: 'text/html' }), (n.title || 'note') + '.html');
      Utils.toast('该格式请连接电脑端导出', 'error');
    },
    exportVault() {
      return Utils.downloadBlob(new Blob([JSON.stringify(vault, null, 2)], { type: 'application/json' }), 'inkvault-local.json');
    },
    async importFile(file, fields) {
      await load();
      const text = await file.text();
      const name = file.name || '';
      let added = 0;
      if (/\.json$/i.test(name)) {
        const data = JSON.parse(text);
        const notes = Array.isArray(data) ? data : (data.notes || []);
        notes.forEach((n) => {
          if (!n.title && !n.content) return;
          vault.notes.unshift({
            uid: uid('nt'), title: n.title || '导入的笔记', content: n.content || '', excerpt: storeExcerpt(n.content || ''),
            notebookUid: vault.notebooks[0].uid, type: 'note', status: 'active', pinned: false, starred: false,
            tags: n.tags || [], attachments: [], versions: [], wordCount: countWords(n.content || ''),
            createdAt: n.createdAt || now(), updatedAt: n.updatedAt || now(), deletedAt: ''
          });
          added++;
        });
      } else if (/\.(md|markdown|txt)$/i.test(name)) {
        const title = (text.match(/^#\s+(.+)$/m) || [])[1] || name.replace(/\.[^.]+$/, '');
        vault.notes.unshift({
          uid: uid('nt'), title, content: text, excerpt: storeExcerpt(text), notebookUid: vault.notebooks[0].uid,
          type: 'note', status: 'active', pinned: false, starred: false, tags: (fields && fields.tagPrefix) ? [fields.tagPrefix] : [],
          attachments: [], versions: [], wordCount: countWords(text), createdAt: now(), updatedAt: now(), deletedAt: ''
        });
        added = 1;
      } else if (/\.enex$/i.test(name)) {
        const doc = new DOMParser().parseFromString(text, 'text/xml');
        const notes = Array.from(doc.getElementsByTagName('note'));
        notes.forEach((el) => {
          const title = (el.getElementsByTagName('title')[0] || {}).textContent || '导入的笔记';
          const html = (el.getElementsByTagName('content')[0] || {}).textContent || '';
          const body = html.replace(/<[^>]+>/g, '\n').replace(/\n{3,}/g, '\n\n').trim();
          const tags = Array.from(el.getElementsByTagName('tag')).map((t) => t.textContent).filter(Boolean);
          vault.notes.unshift({
            uid: uid('nt'), title, content: body, excerpt: storeExcerpt(body), notebookUid: vault.notebooks[0].uid,
            type: 'note', status: 'active', pinned: false, starred: false, tags,
            attachments: [], versions: [], wordCount: countWords(body),
            createdAt: (el.getElementsByTagName('created')[0] || {}).textContent || now(),
            updatedAt: (el.getElementsByTagName('updated')[0] || {}).textContent || now(), deletedAt: ''
          });
          added++;
        });
      } else {
        throw new Error('手机端支持导入 Markdown、TXT、JSON、ENEX；压缩包请在电脑端导入');
      }
      saveNow();
      return { ok: true, created: added, failed: 0, format: name.split('.').pop() };
    },
    async importPreview(file) {
      await load();
      const name = file.name || '';
      const text = await file.text();
      let count = 0, titles = [];
      if (/\.json$/i.test(name)) { const d = JSON.parse(text); const ns = Array.isArray(d) ? d : d.notes || []; count = ns.length; titles = ns.slice(0, 8).map((n) => n.title || '无标题'); }
      else if (/\.enex$/i.test(name)) { const doc = new DOMParser().parseFromString(text, 'text/xml'); count = doc.getElementsByTagName('note').length; titles = Array.from(doc.getElementsByTagName('title')).slice(0, 8).map((t) => t.textContent); }
      else { count = 1; titles = [name.replace(/\.[^.]+$/, '')]; }
      return { format: name.split('.').pop(), filename: name, notes: count, attachments: 0, titles, notebooks: [{ name: '当前笔记本', count }], warnings: [] };
    },
    async uploadImage(file) { return LocalStore.uploadAttachment(State.current ? State.current.uid : '', file); },
    async sync(url, pin) {
      await load();
      const base = url.replace(/\/+$/, '');
      let token = '';
      try {
        const login = await fetch(base + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pin }) });
        if (!login.ok) throw new Error('访问码不正确');
        token = (await login.json()).token;
      } catch (e) { throw new Error('无法连接电脑端：' + e.message); }
      const pull = await fetch(base + '/api/sync/pull', { headers: { 'X-Ink-Token': token } });
      if (!pull.ok) throw new Error('拉取失败');
      const remote = (await pull.json()).vault;
      let created = 0, updated = 0;
      const map = {};
      vault.notes.forEach((n) => { map[n.uid] = n; });
      (remote.notes || []).forEach((rn) => {
        const local = map[rn.uid];
        if (!local) {
          vault.notes.push(Object.assign({}, rn, { tags: rn.tags || [], attachments: [], versions: [], deletedAt: rn.deletedAt || '' }));
          created++;
        } else if ((rn.updatedAt || '') > (local.updatedAt || '')) {
          Object.assign(local, { title: rn.title, content: rn.content, updatedAt: rn.updatedAt, deletedAt: rn.deletedAt || '', tags: rn.tags || [] });
          countWordsNow(local); updated++;
        }
      });
      (remote.notebooks || []).forEach((rn) => { if (!vault.notebooks.find((x) => x.uid === rn.uid)) vault.notebooks.push(Object.assign({}, rn, { children: undefined })); });
      (remote.tags || []).forEach((rt) => { if (!vault.tags.find((x) => x.name === rt.name)) vault.tags.push({ uid: rt.uid || uid('tg'), name: rt.name, color: rt.color || '' }); });
      await saveNow();
      const push = await fetch(base + '/api/sync/push', {
        method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Ink-Token': token },
        body: JSON.stringify({ notes: vault.notes, notebooks: vault.notebooks, tags: vault.tags })
      });
      if (!push.ok) throw new Error('推送失败');
      const pr = await push.json();
      return { created, updated, pushed: pr.updated };
    }
  };

  function flatten(list, depth, out) {
    out = out || [];
    (list || []).forEach((n) => {
      out.push({ uid: n.uid, name: n.name, depth: depth || 0, path: n.name });
      flatten(n.children, (depth || 0) + 1, out);
    });
    return out;
  }

  function computeStats() {
    const notes = visibleNotes();
    const totalWords = notes.reduce((s, n) => s + (n.wordCount || 0), 0);
    const byNotebookMap = {};
    notes.forEach((n) => { const k = notebookPath(n.notebookUid) || '未分类'; byNotebookMap[k] = (byNotebookMap[k] || 0) + 1; });
    const tagMap = {};
    notes.forEach((n) => (n.tags || []).forEach((t) => { tagMap[t] = (tagMap[t] || 0) + 1; }));
    const byMonthMap = {};
    notes.forEach((n) => { const k = (n.createdAt || '').slice(0, 7); if (k) byMonthMap[k] = (byMonthMap[k] || 0) + 1; });
    const days = {};
    for (let i = 364; i >= 0; i--) {
      const d = new Date(); d.setDate(d.getDate() - i);
      days[d.toISOString().slice(0, 10)] = { date: d.toISOString().slice(0, 10), notes: 0, words: 0, edits: 0 };
    }
    notes.forEach((n) => {
      const d = (n.updatedAt || '').slice(0, 10);
      if (days[d]) days[d].edits++;
      const c = (n.createdAt || '').slice(0, 10);
      if (days[c]) days[c].notes++;
    });
    let streak = 0;
    const cursor = new Date();
    while (days[cursor.toISOString().slice(0, 10)] && days[cursor.toISOString().slice(0, 10)].edits > 0) {
      streak++; cursor.setDate(cursor.getDate() - 1);
    }
    const today = new Date().toISOString().slice(0, 10);
    return {
      vaultName: vault.settings.vaultName || '我的笔记', totalNotes: notes.length, activeNotes: notes.length,
      archivedNotes: 0, trashedNotes: vault.notes.filter((n) => n.deletedAt).length,
      starredNotes: notes.filter((n) => n.starred).length, pinnedNotes: notes.filter((n) => n.pinned).length,
      totalWords, avgWords: notes.length ? Math.round(totalWords / notes.length) : 0,
      notebookCount: vault.notebooks.length, tagCount: vault.tags.length, attachmentCount: 0, attachmentSize: 0,
      createdToday: notes.filter((n) => (n.createdAt || '').slice(0, 10) === today).length, updatedToday: 0,
      createdWeek: 0, createdMonth: notes.filter((n) => (n.createdAt || '').slice(0, 7) === today.slice(0, 7)).length,
      streak, firstNoteAt: notes.length ? notes[notes.length - 1].createdAt : '', lastNoteAt: now(),
      byNotebook: Object.entries(byNotebookMap).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count),
      byTag: Object.entries(tagMap).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count),
      byMonth: Object.entries(byMonthMap).map(([month, notesCount]) => ({ month, notes: notesCount, words: 0 })).sort((a, b) => a.month.localeCompare(b.month)).slice(-18),
      byWeekday: [0, 0, 0, 0, 0, 0, 0], daily: Object.values(days),
      topNotes: notes.slice().sort((a, b) => b.wordCount - a.wordCount).slice(0, 10).map((n) => ({ uid: n.uid, title: n.title, words: n.wordCount, notebook: notebookPath(n.notebookUid), updatedAt: n.updatedAt, createdAt: n.createdAt })),
      recentNotes: notes.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 10).map((n) => ({ uid: n.uid, title: n.title, words: n.wordCount, notebook: notebookPath(n.notebookUid), updatedAt: n.updatedAt, createdAt: n.createdAt })),
      types: [], oldestDrafts: 0, unfiledNotes: 0, importBatches: 0, dbBytes: 0,
      generatedAt: now(), longestNote: notes.length ? notes.slice().sort((a, b) => b.wordCount - a.wordCount)[0].title : '',
      busiestWeekday: '—'
    };
  }

  function buildTOC(params) {
    const p = params || {};
    let list = listNotes({ notebook: p.notebook, tag: p.tag, q: p.q, starred: p.starred, trashed: p.trashed, limit: 5000, sort: 'updated_desc' }).notes;
    const groups = {};
    const order = [];
    list.forEach((n) => {
      let key = '';
      if (p.groupBy === 'notebook') key = n.notebookName || '未分类';
      else if (p.groupBy === 'tag') key = (n.tags && n.tags[0]) || '无标签';
      else if (p.groupBy === 'month') key = (n.createdAt || '').slice(0, 7);
      if (!groups[key]) { groups[key] = []; order.push(key); }
      groups[key].push({
        title: n.title, uid: n.uid, tags: n.tags || [], words: n.wordCount, updated: n.updatedAt, created: n.createdAt,
        summary: (n.excerpt || '').slice(0, p.summaryLen || 110), notebook: n.notebookName
      });
    });
    if (p.groupBy === 'month') order.sort().reverse();
    let idx = 0;
    const groupArr = (p.groupBy === 'none' ? [''] : order).map((k) => ({
      name: k, entries: (groups[k] || []).map((e) => Object.assign(e, { number: ++idx }))
    }));
    return {
      title: p.title || '知识库目录', generatedAt: now(), totalNotes: list.length,
      totalWords: list.reduce((s, n) => s + n.wordCount, 0), groups: groupArr,
      numbered: p.numbered !== 0, includeSummary: p.summary !== 0, includeTags: p.tags !== 0,
      includeDates: p.dates !== 0, includeWords: p.words !== 0
    };
  }

  function tocMarkdown(d) {
    let out = `# ${d.title}\n\n> 生成于 ${new Date().toLocaleString('zh-CN')} · 共 ${d.totalNotes} 篇 · ${d.totalWords} 字\n\n`;
    d.groups.forEach((g) => {
      if (g.name) out += `## ${g.name}\n\n`;
      g.entries.forEach((e) => {
        out += `${d.numbered ? e.number + '. ' : '- '}**${e.title}**\n`;
        if (d.includeSummary && e.summary) out += `  ${e.summary}\n`;
        const bits = [];
        if (d.includeTags && e.tags.length) bits.push('#' + e.tags.join(' #'));
        if (d.includeWords && e.words) bits.push(e.words + ' 字');
        if (d.includeDates) bits.push('更新于 ' + (e.updated || '').slice(0, 10));
        if (bits.length) out += `  *${bits.join(' · ')}*\n`;
        out += '\n';
      });
    });
    return out + '\n---\n\n*由墨库 InkVault 生成*\n';
  }

  function tocHTML(d) {
    const esc = (s) => String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(d.title)}</title>
<style>body{background:#fbf9f4;color:#23211e;font-family:-apple-system,"PingFang SC",sans-serif;line-height:1.8;margin:0}
.page{max-width:820px;margin:0 auto;padding:56px 24px}h1{font-family:Georgia,"Songti SC",serif;font-size:36px;margin:0 0 8px}
.sub{color:#7d766a;font-size:13px;margin-bottom:30px}h2{font-family:Georgia,"Songti SC",serif;font-size:21px;border-bottom:1px solid #e4dfd5;padding-bottom:8px;margin-top:40px}
ol{padding-left:22px}li{margin-bottom:14px}.t{font-weight:600}.m{color:#7d766a;font-size:12px}.s{color:#5a554d;font-size:13.5px}
.foot{margin-top:50px;border-top:1px solid #e4dfd5;padding-top:12px;color:#7d766a;font-size:11px}</style></head><body><div class="page">
<h1>${esc(d.title)}</h1><div class="sub">生成于 ${new Date().toLocaleString('zh-CN')} · 共 ${d.totalNotes} 篇 · ${d.totalWords} 字</div>
${d.groups.map((g) => `<h2>${esc(g.name || '全部')}</h2><ol>${g.entries.map((e) => `<li><div class="t">${esc(e.title)}</div>${d.includeSummary && e.summary ? '<div class="s">' + esc(e.summary) + '</div>' : ''}<div class="m">${[d.includeTags && e.tags.length ? '#' + esc(e.tags.join(' #')) : '', d.includeWords ? e.words + ' 字' : '', d.includeDates ? '更新于 ' + esc((e.updated || '').slice(0, 10)) : ''].filter(Boolean).join(' · ')}</div></li>`).join('')}</ol>`).join('')}
<div class="foot">由墨库 InkVault 生成</div></div></body></html>`;
  }

  function tocCSV(d) {
    const rows = [['序号', '标题', '笔记本', '标签', '摘要', '字数', '更新时间']];
    d.groups.forEach((g) => g.entries.forEach((e) => rows.push([e.number, e.title, e.notebook, (e.tags || []).join(' '), (e.summary || '').replace(/"/g, '""'), e.words, (e.updated || '').slice(0, 10)])));
    return rows.map((r) => r.map((c) => /[",\n]/.test(String(c)) ? '"' + String(c).replace(/"/g, '""') + '"' : c).join(',')).join('\r\n');
  }

  function localNoteHTML(n) {
    const body = window.MD.render(n.content || '').html;
    return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>${Utils.esc(n.title)}</title>
<style>body{max-width:760px;margin:0 auto;padding:48px 22px;font-family:-apple-system,"PingFang SC",sans-serif;line-height:1.9;color:#23211e;background:#fbf9f4}
h1{font-family:Georgia,"Songti SC",serif}h2,h3{font-family:Georgia,"Songti SC",serif}img{max-width:100%;border-radius:8px}
blockquote{border-left:3px solid #a8442f;margin:1em 0;padding:.3em 1em;color:#6b6459}code{background:#f1ece1;padding:2px 5px;border-radius:4px}
pre{background:#f1ece1;padding:14px;border-radius:8px;overflow:auto}</style></head><body><h1>${Utils.esc(n.title)}</h1>${body}</body></html>`;
  }

  window.LocalStore = LocalStore;
  window.INKVAULT_LOCAL_MODE = true;
})();
