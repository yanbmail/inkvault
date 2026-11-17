/* 核心：状态、工具、HTTP 适配层 */
(function () {
  const $ = (s, r) => (r || document).querySelector(s);
  const $$ = (s, r) => Array.from((r || document).querySelectorAll(s));
  const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

  const State = {
    boot: null,
    notes: [], total: 0, offset: 0, limit: 60, loading: false, listSeq: 0,
    view: 'all', notebook: null, tag: null, query: '', sort: '', type: '',
    starred: false, pinned: false, trashed: false, archived: false,
    current: null, dirty: false, saving: false, saveTimer: null, lastSavedAt: 0,
    viewMode: 'split', outline: false, mobileEdit: false, sidebarOpen: false,
    notebooks: [], tags: [], fields: [], templates: [], searches: [], counts: {},
    expanded: {}, stats: null, settings: {}, theme: 'light', editorWidth: 1080,
    local: false, publicUrl: '', online: navigator.onLine !== false,
    paletteIndex: 0, paletteItems: [], focusMode: false
  };
  window.State = State;

  const Utils = {
    isMac,
    esc(s) {
      return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    },
    fmtNum(n) { return (n || 0).toLocaleString('zh-CN'); },
    fmtDate(ts, withTime) {
      if (!ts) return '';
      const d = new Date(ts);
      if (isNaN(d)) return '';
      const pad = (x) => String(x).padStart(2, '0');
      const s = d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
      return withTime ? s + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) : s;
    },
    fmtRelative(ts) {
      if (!ts) return '';
      const d = new Date(ts);
      if (isNaN(d)) return '';
      const now = new Date();
      const diff = (now - d) / 1000;
      if (diff < 60) return '刚刚';
      if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前';
      const sameDay = d.toDateString() === now.toDateString();
      if (sameDay) return '今天 ' + String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
      const y = new Date(now); y.setDate(now.getDate() - 1);
      if (d.toDateString() === y.toDateString()) return '昨天';
      if (d.getFullYear() === now.getFullYear()) return (d.getMonth() + 1) + '月' + d.getDate() + '日';
      return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
    },
    fmtBytes(n) {
      if (!n) return '0 B';
      const u = ['B', 'KB', 'MB', 'GB'];
      let i = 0; let v = n;
      while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
      return (i === 0 ? v : v.toFixed(1)) + ' ' + u[i];
    },
    debounce(fn, ms) {
      let t;
      return function (...args) { clearTimeout(t); t = setTimeout(() => fn.apply(this, args), ms); };
    },
    sleep(ms) { return new Promise((r) => setTimeout(r, ms)); },
    uid(p) { return (p || 'id') + '-' + Math.random().toString(36).slice(2, 11); },
    download(url, filename) {
      if (window.AndroidBridge && window.AndroidBridge.download && /^https?:/.test(url)) {
        window.AndroidBridge.download(url, filename || '');
        return;
      }
      const a = document.createElement('a');
      a.href = url;
      if (filename) a.download = filename;
      a.rel = 'noopener';
      document.body.appendChild(a);
      a.click();
      setTimeout(() => a.remove(), 100);
    },
    downloadBlob(blob, filename) {
      // Native shells can save blobs directly (file:// pages cannot download).
      const native = window.AndroidBridge && window.AndroidBridge.saveBase64
        ? (name, mime, b64) => window.AndroidBridge.saveBase64(name, mime, b64)
        : (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.inkvault
          ? (name, mime, b64) => window.webkit.messageHandlers.inkvault.postMessage({ action: 'save', name, mime, b64 })
          : null);
      if (native) {
        const reader = new FileReader();
        reader.onload = () => {
          const b64 = String(reader.result).split(',')[1] || '';
          native(filename || 'inkvault-export', blob.type || 'application/octet-stream', b64);
        };
        reader.readAsDataURL(blob);
        return;
      }
      const url = URL.createObjectURL(blob);
      Utils.download(url, filename);
      setTimeout(() => URL.revokeObjectURL(url), 4000);
    },
    async copy(text) {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch (e) {
        const ta = document.createElement('textarea');
        ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.select();
        try { document.execCommand('copy'); } catch (_) {}
        ta.remove();
        return true;
      }
    },
    highlight(text, query) {
      const q = (query || '').trim();
      if (!q) return Utils.esc(text);
      const terms = q.split(/\s+/).filter((t) => t && !/[:#]/.test(t)).slice(0, 4);
      let out = Utils.esc(text);
      terms.forEach((term) => {
        const safe = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        out = out.replace(new RegExp('(' + safe + ')', 'gi'), '<mark class="nc-mark">$1</mark>');
      });
      return out;
    },
    toast(msg, type) {
      const root = $('#toasts');
      const el = document.createElement('div');
      el.className = 'toast ' + (type || '');
      el.innerHTML = '<i data-icon="' + (type === 'error' ? 'info' : type === 'success' ? 'check' : 'sparkles') + '"></i><span>' + Utils.esc(msg) + '</span>';
      root.appendChild(el);
      hydrateIcons(el);
      setTimeout(() => { el.style.opacity = '0'; el.style.transform = 'translateY(4px)'; setTimeout(() => el.remove(), 250); }, type === 'error' ? 5200 : 3000);
    },
    // Downscale large pasted images so the vault stays light.
    async prepareImage(file, maxDim) {
      if (!file || !/^image\//.test(file.type)) return file;
      if (file.type === 'image/gif' || file.size < 900 * 1024) return file;
      try {
        const bitmap = await createImageBitmap(file);
        const max = maxDim || 2000;
        let { width, height } = bitmap;
        if (width <= max && height <= max) return file;
        const ratio = Math.min(max / width, max / height);
        width = Math.round(width * ratio); height = Math.round(height * ratio);
        const canvas = document.createElement('canvas');
        canvas.width = width; canvas.height = height;
        canvas.getContext('2d').drawImage(bitmap, 0, 0, width, height);
        const blob = await new Promise((res) => canvas.toBlob(res, 'image/jpeg', 0.88));
        if (!blob) return file;
        return new File([blob], (file.name || 'image').replace(/\.[^.]+$/, '') + '.jpg', { type: 'image/jpeg' });
      } catch (e) { return file; }
    },
    countWords(text) {
      const s = String(text || '');
      const cjk = (s.match(/[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]/g) || []).length;
      const latin = (s.replace(/[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]/g, ' ').match(/[A-Za-z0-9]+/g) || []).length;
      return cjk + latin;
    }
  };
  window.$ = $; window.$$ = $$; window.Utils = Utils;

  /* ------------------------------ modal helpers ----------------------------- */
  function closeModal() {
    const root = $('#modalRoot');
    root.classList.add('hidden');
    root.innerHTML = '';
    document.body.style.overflow = '';
  }
  function openModal(opts) {
    const root = $('#modalRoot');
    const card = document.createElement('div');
    card.className = 'modal-card' + (opts.wide ? ' wide' : '') + (opts.narrow ? ' narrow' : '');
    card.innerHTML = `
      <div class="modal-head">
        <div><h3>${Utils.esc(opts.title || '')}</h3>${opts.subtitle ? '<p>' + opts.subtitle + '</p>' : ''}</div>
        <button class="icon-btn" data-close><i data-icon="x"></i></button>
      </div>
      <div class="modal-body">${opts.body || ''}</div>
      ${opts.footer ? '<div class="modal-foot">' + opts.footer + '</div>' : ''}`;
    root.innerHTML = '';
    root.appendChild(card);
    root.classList.remove('hidden');
    hydrateIcons(card);
    card.querySelectorAll('[data-close]').forEach((b) => b.addEventListener('click', closeModal));
    root.onclick = (e) => { if (e.target === root) closeModal(); };
    if (opts.onMount) opts.onMount(card);
    return card;
  }
  function closeDrawer() {
    const d = $('#drawer');
    d.classList.add('hidden'); d.innerHTML = '';
    $('#overlay').classList.add('hidden');
  }
  function openDrawer(html, onMount) {
    const d = $('#drawer');
    d.innerHTML = html;
    d.classList.remove('hidden');
    $('#overlay').classList.remove('hidden');
    hydrateIcons(d);
    $('#overlay').onclick = closeDrawer;
    if (onMount) onMount(d);
    return d;
  }
  window.Modal = { open: openModal, close: closeModal, drawer: openDrawer, closeDrawer };

  async function confirmDialog(opts) {
    return new Promise((resolve) => {
      const card = openModal({
        title: opts.title || '确认',
        subtitle: opts.subtitle || '',
        narrow: true,
        body: `<p style="font-size:13px;color:var(--muted);line-height:1.8;margin:0">${opts.message || ''}</p>`,
        footer: `<button class="btn ghost" data-no>取消</button><div class="right"><button class="btn ${opts.danger ? 'danger' : 'primary'}" data-yes>${Utils.esc(opts.okText || '确定')}</button></div>`
      });
      card.querySelector('[data-no]').onclick = () => { closeModal(); resolve(false); };
      card.querySelector('[data-yes]').onclick = () => { closeModal(); resolve(true); };
    });
  }
  async function promptDialog(opts) {
    return new Promise((resolve) => {
      const field = opts.multiline
        ? `<textarea class="textarea" data-input style="min-height:180px" placeholder="${Utils.esc(opts.placeholder || '')}">${Utils.esc(opts.value || '')}</textarea>`
        : `<input class="input" data-input value="${Utils.esc(opts.value || '')}" placeholder="${Utils.esc(opts.placeholder || '')}">`;
      const card = openModal({
        title: opts.title || '输入',
        narrow: true,
        body: `<div class="field"><label>${Utils.esc(opts.label || '')}</label>${field}
          ${opts.hint ? '<div class="hint">' + opts.hint + '</div>' : ''}</div>`,
        footer: `<button class="btn ghost" data-no>取消</button><div class="right"><button class="btn primary" data-yes>${Utils.esc(opts.okText || '确定')}</button></div>`
      });
      const input = card.querySelector('[data-input]');
      setTimeout(() => { input.focus(); if (!opts.multiline) input.select(); }, 50);
      const done = (v) => { closeModal(); resolve(v); };
      card.querySelector('[data-no]').onclick = () => done(null);
      card.querySelector('[data-yes]').onclick = () => done(input.value.trim());
      input.onkeydown = (e) => {
        if (e.key === 'Enter' && (opts.multiline ? (e.metaKey || e.ctrlKey) : true)) done(input.value.trim());
        if (opts.multiline && e.key === 'Enter' && (e.metaKey || e.ctrlKey)) e.preventDefault();
      };
    });
  }
  window.Dialogs = { confirm: confirmDialog, prompt: promptDialog };

  /* --------------------------------- HTTP ---------------------------------- */
  async function request(path, opts) {
    opts = opts || {};
    const init = { method: opts.method || 'GET', credentials: 'same-origin', headers: {} };
    if (opts.body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(opts.body);
    }
    if (opts.headers) Object.assign(init.headers, opts.headers);
    let res;
    try {
      res = await fetch(path, init);
    } catch (e) {
      State.online = false;
      throw new Error('无法连接到服务端，请检查是否已启动');
    }
    State.online = true;
    if (res.status === 401) {
      if (window.onAuthRequired) window.onAuthRequired();
      const err = new Error('AUTH'); err.code = 401; throw err;
    }
    if (!res.ok) {
      let msg = '请求失败 (' + res.status + ')';
      try { const j = await res.json(); if (j.error) msg = j.error; } catch (e) {}
      throw new Error(msg);
    }
    if (opts.raw) return res;
    const ct = res.headers.get('Content-Type') || '';
    if (ct.includes('application/json')) return res.json();
    return res.text();
  }

  function query(params) {
    const q = new URLSearchParams();
    Object.keys(params || {}).forEach((k) => {
      const v = params[k];
      if (v === undefined || v === null || v === '' || v === false) return;
      q.set(k, v === true ? '1' : v);
    });
    return q.toString();
  }

  const HTTP = {
    kind: 'http',
    base: '',
    async ping() { return request('/api/ping'); },
    async login(pin) { return request('/api/login', { method: 'POST', body: { pin } }); },
    async logout() { return request('/api/logout', { method: 'POST' }); },
    async bootstrap() { return request('/api/bootstrap'); },
    async listNotes(p) { return request('/api/notes?' + query(p)); },
    async getNote(uid) { return request('/api/notes/' + uid); },
    async createNote(data) { return request('/api/notes', { method: 'POST', body: data }); },
    async updateNote(uid, data) { return request('/api/notes/' + uid, { method: 'PUT', body: data }); },
    async patchNote(uid, patch) { return request('/api/notes/' + uid, { method: 'PATCH', body: patch }); },
    async deleteNote(uid) { return request('/api/notes/' + uid, { method: 'DELETE' }); },
    async restoreNote(uid) { return request('/api/notes/' + uid + '/restore', { method: 'POST' }); },
    async purgeNote(uid) { return request('/api/notes/' + uid + '/purge', { method: 'DELETE' }); },
    async duplicateNote(uid) { return request('/api/notes/' + uid + '/duplicate', { method: 'POST' }); },
    async versions(uid, withContent) { return request('/api/notes/' + uid + '/versions' + (withContent ? '?content=1' : '')); },
    async restoreVersion(uid, id) { return request('/api/notes/' + uid + '/versions/' + id + '/restore', { method: 'POST' }); },
    async setFields(uid, values) { return request('/api/notes/' + uid + '/fields', { method: 'POST', body: values }); },
    async uploadAttachment(uid, file) {
      const fd = new FormData();
      fd.append('file', file, file.name || 'image.png');
      const res = await fetch('/api/notes/' + uid + '/attachments', { method: 'POST', body: fd, credentials: 'same-origin' });
      if (!res.ok) throw new Error('上传失败');
      return res.json();
    },
    async listNotebooks() { return request('/api/notebooks'); },
    async createNotebook(data) { return request('/api/notebooks', { method: 'POST', body: data }); },
    async updateNotebook(uid, data) { return request('/api/notebooks/' + uid, { method: 'PUT', body: data }); },
    async deleteNotebook(uid) { return request('/api/notebooks/' + uid, { method: 'DELETE' }); },
    async listTags() { return request('/api/tags'); },
    async createTag(data) { return request('/api/tags', { method: 'POST', body: data }); },
    async updateTag(uid, data) { return request('/api/tags/' + uid, { method: 'PUT', body: data }); },
    async deleteTag(uid) { return request('/api/tags/' + uid, { method: 'DELETE' }); },
    async listFields() { return request('/api/fields'); },
    async saveField(data) { return request('/api/fields', { method: 'POST', body: data }); },
    async deleteField(uid) { return request('/api/fields/' + uid, { method: 'DELETE' }); },
    async listTemplates() { return request('/api/templates'); },
    async saveTemplate(data) { return request('/api/templates', { method: 'POST', body: data }); },
    async deleteTemplate(uid) { return request('/api/templates/' + uid, { method: 'DELETE' }); },
    async listSearches() { return request('/api/searches'); },
    async saveSearch(data) { return request('/api/searches', { method: 'POST', body: data }); },
    async deleteSearch(uid) { return request('/api/searches/' + uid, { method: 'DELETE' }); },
    async emptyTrash() { return request('/api/trash/empty', { method: 'POST' }); },
    async stats() { return request('/api/stats'); },
    async statsNote(payload) { return request('/api/stats/note', { method: 'POST', body: payload || {} }); },
    async makeIndexNote(notebookUid) { return request('/api/notes/' + notebookUid + '/index', { method: 'POST', body: {} }); },
    async settings() { return request('/api/settings'); },
    async saveSettings(data) { return request('/api/settings', { method: 'PUT', body: data }); },
    async audit() { return request('/api/audit'); },
    async reindex() { return request('/api/tools/reindex', { method: 'POST' }); },
    async backups() { return request('/api/backups'); },
    async createBackup() { return request('/api/backups', { method: 'POST' }); },
    async deleteBackup(name) { return request('/api/backups/' + encodeURIComponent(name), { method: 'DELETE' }); },
    async restoreBackup(name) { return request('/api/backups/restore', { method: 'POST', body: { name } }); },
    async tocPreview(params) { return request('/api/toc/preview?' + query(params)); },
    async syncPull() { return request('/api/sync/pull'); },
    async syncPush(vault) { return request('/api/sync/push', { method: 'POST', body: vault }); },

    exportTOC(params) {
      Utils.download('/api/export/toc?' + query(params));
    },
    exportStats(format) {
      Utils.download('/api/export/stats?' + query({ format }));
    },
    exportNote(uid, format) {
      Utils.download('/api/notes/' + uid + '/export?' + query({ format }));
    },
    exportVault(format) {
      Utils.download('/api/export/vault?' + query({ format: format || 'zip' }));
    },
    downloadBackup(name) {
      Utils.download('/api/backups/download/' + encodeURIComponent(name));
    },
    async importFile(file, fields) {
      const fd = new FormData();
      fd.append('file', file, file.name);
      Object.keys(fields || {}).forEach((k) => fd.append(k, fields[k]));
      const res = await fetch('/api/import', { method: 'POST', body: fd, credentials: 'same-origin' });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json.error || '导入失败');
      return json;
    },
    async importPreview(file) {
      const fd = new FormData();
      fd.append('file', file, file.name);
      const res = await fetch('/api/import/preview', { method: 'POST', body: fd, credentials: 'same-origin' });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json.error || '无法识别文件');
      return json;
    },
    async uploadImage(file) {
      // server-side attachments require a note; callers use uploadAttachment
      return HTTP.uploadAttachment(State.current ? State.current.uid : '', file);
    }
  };

  window.HTTP = HTTP;
  window.request = request;
})();
