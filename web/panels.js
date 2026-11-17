/* 面板：目录导出、统计、导入、设置、备份、同步、版本、命令面板 */
(function () {
  const S = window.State;
  const P = {};

  function store() { return window.Store; }
  function icon(n, s) { return window.Icons.svg(n, s || 16); }
  function esc(v) { return Utils.esc(v); }
  function currentFilterParams() {
    return {
      notebook: S.notebook || '', tag: S.tag || '', q: S.query || '',
      starred: S.starred ? 1 : '', trashed: S.trashed ? 1 : '', type: S.type || ''
    };
  }

  /* ------------------------------- sheet ---------------------------------- */
  function openSheet(title, subtitle, headExtra) {
    closeSheet();
    const el = document.createElement('div');
    el.className = 'sheet';
    el.id = 'sheet';
    el.innerHTML = `<div class="sheet-head">
        <div style="display:flex;align-items:baseline;gap:8px"><h2>${esc(title)}</h2><span class="sub">${esc(subtitle || '')}</span></div>
        <div style="display:flex;align-items:center;gap:8px">${headExtra || ''}<button class="icon-btn" data-sheet-close>${icon('x', 18)}</button></div>
      </div>
      <div class="sheet-body" id="sheetBody"></div>`;
    document.body.appendChild(el);
    el.querySelector('[data-sheet-close]').onclick = closeSheet;
    hydrateIcons(el);
    return el;
  }
  function closeSheet() {
    const el = document.getElementById('sheet');
    if (el) el.remove();
  }
  P.closeSheet = closeSheet;

  /* ------------------------------- TOC ------------------------------------ */
  P.openTOC = function () {
    const opts = {
      format: 'md', groupBy: S.notebook ? 'none' : 'notebook', summaryLen: 110,
      summary: true, tags: true, dates: true, words: true, numbered: true,
      title: (S.query ? '搜索：' + S.query : S.notebook ? UI.viewLabel() + ' · 目录' : '知识库目录')
    };
    const formats = [
      ['md', 'Markdown', '通用文本'], ['html', '网页', '可打印'], ['csv', 'CSV', '表格'],
      ['xlsx', 'Excel', '可筛选'], ['docx', 'Word', '正式文档'], ['json', 'JSON', '结构化']
    ];
    const body = `<div class="toc-grid">
      <div class="toc-options">
        <div class="field"><label>标题</label><input class="input" id="tocTitle" value="${esc(opts.title)}"></div>
        <div class="field"><label>导出格式</label>
          <div class="toc-formats" id="tocFormats">${formats.map((f) => `<button class="fmt-card ${f[0] === 'md' ? 'active' : ''}" data-format="${f[0]}"><b>${f[1]}</b>${f[2]}</button>`).join('')}</div>
        </div>
        <div class="field"><label>分组方式</label>
          <div class="seg" id="tocGroup">
            ${[['notebook', '按笔记本'], ['tag', '按标签'], ['month', '按月份'], ['none', '不分组']].map((g) => `<button data-group="${g[0]}" class="${g[0] === opts.groupBy ? 'active' : ''}">${g[1]}</button>`).join('')}
          </div>
          <div class="hint">分组只影响排版，目录里仍会保留每篇笔记的笔记本与标签信息。</div>
        </div>
        <div class="field"><label>简介长度</label>
          <div class="seg" id="tocSummaryLen">
            ${[[60, '一句话'], [110, '适中'], [180, '详细'], [0, '不显示']].map((v) => `<button data-len="${v[0]}" class="${v[0] === 110 ? 'active' : ''}">${v[1]}</button>`).join('')}
          </div>
        </div>
        <div>
          ${[['summary', '显示简介', '每篇笔记开头的内容摘要'], ['tags', '显示标签', ''], ['dates', '显示更新时间', ''], ['words', '显示字数', ''], ['numbered', '连续编号', '跨分组统一编号']].map((o) =>
            `<div class="opt-line"><div><div class="lbl">${o[1]}</div>${o[2] ? '<div class="sub">' + o[2] + '</div>' : ''}</div>
              <button class="toggle ${opts[o[0]] ? 'on' : ''}" data-opt="${o[0]}"></button></div>`).join('')}
        </div>
        <div class="hint" style="margin-top:12px">范围：${esc(UI.viewLabel())}${S.query ? '（搜索 "' + esc(S.query) + '"）' : ''}</div>
      </div>
      <div>
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
          <label style="font-size:11.5px;color:var(--muted)">实时预览</label>
          <div style="display:flex;align-items:center;gap:8px">
            <span style="font-size:10.5px;color:var(--faint)" id="tocCount"></span>
            <button class="btn ghost small" id="tocRaw" style="height:24px;padding:0 8px;font-size:10.5px">源码</button>
          </div>
        </div>
        <div class="toc-preview"><div class="md toc-doc" id="tocPreview">正在生成预览…</div></div>
        <div class="hint" style="margin-top:10px">目录会包含标题、简介、标签、字数与时间；HTML 版本可直接打印或分享，Excel 版本可按列筛选。</div>
      </div>
    </div>`;

    let card = null;
    function params() {
      return Object.assign(currentFilterParams(), {
        format: opts.format, groupBy: opts.groupBy, summaryLen: opts.summaryLen,
        summary: opts.summary ? 1 : 0, tags: opts.tags ? 1 : 0, dates: opts.dates ? 1 : 0,
        words: opts.words ? 1 : 0, numbered: opts.numbered ? 1 : 0,
        title: opts.title, filterNote: UI.viewLabel()
      });
    }
    let rawPreview = false;
    let lastPreview = '';
    const refresh = Utils.debounce(async () => {
      const pre = document.querySelector('#tocPreview');
      if (!pre) return;
      pre.textContent = '正在生成预览…';
      try {
        const r = await store().tocPreview(params());
        lastPreview = r.preview;
        renderPreviewBox();
        const cnt = document.querySelector('#tocCount');
        if (cnt) cnt.textContent = '共 ' + (r.total || 0) + ' 篇';
      } catch (e) { pre.textContent = '预览失败：' + e.message; }
    }, 260);
    function renderPreviewBox() {
      const pre = document.querySelector('#tocPreview');
      if (!pre) return;
      if (rawPreview) {
        pre.className = 'toc-source';
        pre.textContent = lastPreview.length > 6000 ? lastPreview.slice(0, 6000) + '\n\n…（预览截断，导出为完整版）' : lastPreview;
      } else {
        pre.className = 'md toc-doc';
        pre.innerHTML = MD.render(lastPreview.slice(0, 9000)).html;
      }
      const btn = document.querySelector('#tocRaw');
      if (btn) btn.textContent = rawPreview ? '排版' : '源码';
    }

    card = Modal.open({
      title: '导出目录与摘要',
      subtitle: '把整个知识库变成可读、可打印、可检索的目录文档',
      wide: true,
      body,
      footer: `<div style="font-size:11px;color:var(--faint)">数据始终留在本机，导出文件由浏览器直接下载</div>
        <div class="right"><button class="btn ghost" data-close>取消</button>
        <button class="btn primary" id="tocDownload">${icon('download', 15)}导出 ${'Markdown'}</button></div>`,
      onMount: mount
    });

    function mount(root) {
      root.querySelectorAll('[data-format]').forEach((b) => b.onclick = () => {
        root.querySelectorAll('[data-format]').forEach((x) => x.classList.remove('active'));
        b.classList.add('active');
        opts.format = b.getAttribute('data-format');
        const label = formats.find((f) => f[0] === opts.format);
        root.querySelector('#tocDownload').innerHTML = icon('download', 15) + '导出 ' + label[1];
        refresh();
      });
      root.querySelectorAll('[data-group]').forEach((b) => b.onclick = () => {
        root.querySelectorAll('[data-group]').forEach((x) => x.classList.remove('active'));
        b.classList.add('active'); opts.groupBy = b.getAttribute('data-group'); refresh();
      });
      root.querySelectorAll('[data-len]').forEach((b) => b.onclick = () => {
        root.querySelectorAll('[data-len]').forEach((x) => x.classList.remove('active'));
        b.classList.add('active'); opts.summaryLen = +b.getAttribute('data-len');
        opts.summary = opts.summaryLen > 0; refresh();
      });
      root.querySelectorAll('[data-opt]').forEach((b) => b.onclick = () => {
        const k = b.getAttribute('data-opt');
        opts[k] = !opts[k];
        b.classList.toggle('on', opts[k]);
        refresh();
      });
      const rawBtn = root.querySelector('#tocRaw');
      if (rawBtn) rawBtn.onclick = () => { rawPreview = !rawPreview; renderPreviewBox(); };
      const titleInput = root.querySelector('#tocTitle');
      titleInput.oninput = Utils.debounce(() => { opts.title = titleInput.value.trim() || '知识库目录'; refresh(); }, 300);
      root.querySelector('#tocDownload').onclick = () => {
        store().exportTOC(params());
        Utils.toast('正在导出目录…', 'success');
      };
      refresh();
    }
  };

  /* ------------------------------ stats ----------------------------------- */
  P.openStats = async function () {
    openSheet('数据与洞察', '统计数据全部在本地计算');
    $('#sheetBody').innerHTML = '<div style="padding:60px;text-align:center;color:var(--faint)">正在统计…</div>';
    try {
      const st = await store().stats();
      UI.renderStats(st);
    } catch (e) {
      $('#sheetBody').innerHTML = '<div style="padding:40px;color:var(--muted)">统计失败：' + esc(e.message) + '</div>';
    }
  };

  P.bindStats = function () {
    const exp = document.getElementById('statsExportBtn');
    if (exp) exp.onclick = () => P.openStatsExport();
    const ref = document.getElementById('statsRefresh');
    if (ref) ref.onclick = async () => { UI.renderStats(await store().stats()); Utils.toast('已刷新', 'success'); };
    const nb = document.getElementById('statsNoteBtn');
    if (nb) nb.onclick = async () => {
      try {
        const n = await store().statsNote({});
        Utils.toast('已写入笔记：' + n.title, 'success');
        closeSheet(); window.refreshAll(); window.openNote(n.uid);
      } catch (e) { Utils.toast(e.message, 'error'); }
    };
    document.querySelectorAll('[data-note]').forEach((a) => {
      if (a.tagName === 'A') a.onclick = (e) => { e.preventDefault(); closeSheet(); window.openNote(a.getAttribute('data-note')); };
    });
  };

  P.openStatsExport = function () {
    Modal.open({
      title: '导出统计报告',
      subtitle: '把数据变成可分享、可归档的报告',
      narrow: true,
      body: `<div class="toc-formats" style="grid-template-columns:1fr 1fr">
        ${[['html', '网页报告', '带图表，可打印'], ['xlsx', 'Excel 工作簿', '6 个工作表'], ['docx', 'Word 文档', '正式报告'], ['csv', 'CSV 数据', '原始指标'], ['md', 'Markdown', '纯文本'], ['json', 'JSON', '程序处理']].map((f) =>
          `<button class="fmt-card" data-stat-format="${f[0]}"><b>${f[1]}</b>${f[2]}</button>`).join('')}
      </div>`,
      footer: `<span style="font-size:11px;color:var(--faint)">统计基于本地数据库实时计算</span><div class="right"><button class="btn ghost" data-close>取消</button></div>`,
      onMount(card) {
        card.querySelectorAll('[data-stat-format]').forEach((b) => b.onclick = () => {
          const f = b.getAttribute('data-stat-format');
          if (f === 'json') {
            store().stats().then((st) => Utils.downloadBlob(new Blob([JSON.stringify(st, null, 2)], { type: 'application/json' }), 'inkvault-stats.json'));
          } else {
            store().exportStats(f);
          }
          Modal.close();
          Utils.toast('正在导出统计报告…', 'success');
        });
      }
    });
  };

  /* ------------------------------ import ---------------------------------- */
  P.openImport = function () {
    const body = `<div class="dropzone" id="dropzone">
        <div class="dz-icon">${icon('upload', 30)}</div>
        <h3>把笔记文件拖到这里</h3>
        <p>或点击选择文件 · 支持批量与压缩包</p>
        <div class="format-badges">
          <span class="fmt-badge">印象笔记 .enex</span><span class="fmt-badge">锤子便签 Markdown</span>
          <span class="fmt-badge">Apple 备忘录 HTML</span><span class="fmt-badge">Obsidian 文件夹</span>
          <span class="fmt-badge">Notion 导出</span><span class="fmt-badge">Joplin .jex</span>
          <span class="fmt-badge">Markdown / TXT</span><span class="fmt-badge">CSV / JSON</span>
        </div>
      </div>
      <input type="file" id="importFile" class="hidden" accept=".zip,.enex,.html,.htm,.md,.markdown,.txt,.csv,.tsv,.json,.jex,.tar,.tgz">
      <div id="importPreview"></div>
      <div class="hint" style="margin-top:16px;line-height:1.9">
        <b style="color:var(--ink-2)">导入规则</b>：所有内容都会新建笔记，不覆盖现有数据；文件夹层级会变成笔记本，标签会保留。
        印象笔记请先在客户端「导出笔记 → ENEX」；锤子便签在网页版用导出助手导出 Markdown 压缩包。
      </div>`;
    const card = Modal.open({
      title: '导入笔记',
      subtitle: '把散落在各处的笔记，收进同一个库',
      wide: true,
      body,
      footer: `<div style="font-size:11px;color:var(--faint)" id="importStatus">尚未选择文件</div>
        <div class="right"><button class="btn ghost" data-close>取消</button><button class="btn primary" id="importConfirm" disabled>${icon('upload', 15)}开始导入</button></div>`,
      onMount(root) {
        const dz = root.querySelector('#dropzone');
        const input = root.querySelector('#importFile');
        const preview = root.querySelector('#importPreview');
        const confirm = root.querySelector('#importConfirm');
        const status = root.querySelector('#importStatus');
        let picked = null;

        dz.onclick = () => input.click();
        ['dragenter', 'dragover'].forEach((ev) => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add('drag'); }));
        ['dragleave', 'drop'].forEach((ev) => dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.remove('drag'); }));
        dz.addEventListener('drop', (e) => {
          if (e.dataTransfer.files && e.dataTransfer.files[0]) handleFile(e.dataTransfer.files[0]);
        });
        input.onchange = () => { if (input.files[0]) handleFile(input.files[0]); };

        async function handleFile(file) {
          picked = file;
          status.textContent = '正在识别 ' + file.name + '…';
          preview.innerHTML = '<div style="padding:18px;color:var(--faint);font-size:12px">正在解析文件…</div>';
          try {
            const r = await store().importPreview(file);
            status.textContent = file.name + ' · ' + (r.format || '').toUpperCase();
            const nb = (r.notebooks || []).slice(0, 6).map((n) => `<span class="fmt-badge">${esc(n.name || '未分类')} · ${n.count}</span>`).join('');
            const warnings = (r.warnings || []).length ? `<div style="margin-top:10px;color:#a9792b;font-size:11.5px">${r.warnings.slice(0, 4).map(esc).join('<br>')}</div>` : '';
            preview.innerHTML = `<div class="import-report">
              <div class="ir-head">${icon('check', 15)} 识别到 ${r.notes} 篇笔记 · ${r.attachments || 0} 个附件</div>
              <div class="ir-body">
                <div style="display:flex;flex-wrap:wrap;gap:6px;margin-bottom:10px">${nb || '<span class="fmt-badge">未分类</span>'}</div>
                <div class="import-list">${(r.titles || []).map((t) => '<li>· ' + esc(t) + '</li>').join('')}</div>
                ${warnings}
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:16px">
                  <div class="field" style="margin:0"><label>放进哪个笔记本（留空则按原结构）</label><input class="input" id="importNotebook" placeholder="例如：导入 · 印象笔记"></div>
                  <div class="field" style="margin:0"><label>额外加上标签（可选）</label><input class="input" id="importTag" placeholder="例如：历史笔记"></div>
                </div>
              </div>
            </div>`;
            confirm.disabled = false;
            confirm.onclick = async () => {
              confirm.disabled = true;
              confirm.innerHTML = '正在导入…';
              try {
                const rr = await store().importFile(picked, {
                  notebook: (root.querySelector('#importNotebook') || {}).value || '',
                  tagPrefix: (root.querySelector('#importTag') || {}).value || ''
                });
                preview.innerHTML = `<div class="import-report"><div class="ir-head">${icon('check', 15)} 导入完成</div>
                  <div class="ir-body"><p style="font-size:13px;margin:0">成功导入 <b>${rr.created}</b> 篇笔记${rr.failed ? '，失败 ' + rr.failed + ' 篇' : ''}。</p>
                  <p style="font-size:11.5px;color:var(--faint);margin:6px 0 0">可以在左侧笔记本中查看，或直接在搜索框里找它们。</p></div></div>`;
                status.textContent = '已导入 ' + rr.created + ' 篇';
                confirm.innerHTML = icon('check', 15) + '完成';
                confirm.disabled = true;
                await window.refreshAll();
                Utils.toast('成功导入 ' + rr.created + ' 篇笔记', 'success');
              } catch (e) {
                confirm.disabled = false;
                confirm.innerHTML = '重试';
                status.textContent = '导入失败：' + e.message;
                Utils.toast(e.message, 'error');
              }
            };
          } catch (e) {
            preview.innerHTML = `<div style="padding:16px;color:#a33b2d;font-size:12px">${esc(e.message)}</div>`;
            status.textContent = '无法识别该文件';
            confirm.disabled = true;
          }
        }
      }
    });
    return card;
  };

  /* ------------------------------ settings -------------------------------- */
  P.openSettings = async function () {
    let s;
    try { s = await store().settings(); } catch (e) { Utils.toast(e.message, 'error'); return; }
    const tab = (name) => `<button class="seg-btn ${name === 'general' ? 'active' : ''}" data-tab="${name}">${{ general: '常规', security: '访问与安全', structure: '分类与模板', backup: '备份', about: '关于' }[name]}</button>`;
    const body = `<div class="seg" style="margin-bottom:20px">${tab('general')}${tab('security')}${tab('structure')}${tab('backup')}${tab('about')}</div>
      <div data-panel="general">
        <div class="row2">
          <div class="field"><label>知识库名称</label><input class="input" id="setVaultName" value="${esc(s.vaultName || '')}"></div>
          <div class="field"><label>公网地址（花生壳 / 反向代理）</label><input class="input" id="setPublicUrl" value="${esc(s.publicUrl || '')}" placeholder="https://notes.example.com"></div>
        </div>
        <div class="field"><label>外观主题</label><div class="seg" id="themeSeg">
          ${[['light', '纸白'], ['sepia', '暖黄'], ['dark', '墨黑']].map((t) => `<button data-theme="${t[0]}" class="${(S.theme || 'light') === t[0] ? 'active' : ''}">${t[1]}</button>`).join('')}
        </div></div>
        <div class="row2">
          <div class="field"><label>正文字号</label><div class="seg" id="fontSeg">${[14, 15, 16, 17].map((n) => `<button data-font="${n}" class="${(+localStorage.getItem('iv.font') || 15) === n ? 'active' : ''}">${n}px</button>`).join('')}</div></div>
          <div class="field"><label>编辑区宽度</label><div class="seg" id="widthSeg">${[900, 1080, 1280].map((n) => `<button data-width="${n}" class="${(+localStorage.getItem('iv.width') || 1080) === n ? 'active' : ''}">${n}</button>`).join('')}</div></div>
        </div>
        <div class="field"><label>默认打开方式</label><div class="seg" id="modeSeg">${[['split', '分屏'], ['edit', '纯编辑'], ['preview', '预览']].map((m) => `<button data-mode="${m[0]}" class="${S.viewMode === m[0] ? 'active' : ''}">${m[1]}</button>`).join('')}</div></div>
        <div class="switch"><div><div class="sw-label">自动保存</div><div class="sw-hint">停止输入 0.8 秒后自动写入本地数据库，无需手动保存</div></div><button class="toggle on" data-locked></button></div>
        <div class="switch"><div><div class="sw-label">显示大纲面板</div><div class="sw-hint">在编辑区右侧显示标题结构</div></div><button class="toggle ${S.outline ? 'on' : ''}" id="setOutline"></button></div>
      </div>
      <div data-panel="security" class="hidden">
        <div class="row2">
          <div class="field"><label>访问码</label><input class="input" id="setPin" value="${esc(s.pin || '')}"><div class="hint">手机、平板和外网访问时使用；建议 6 位以上。</div></div>
          <div class="field"><label>API 令牌</label><input class="input" id="setToken" value="${esc(s.token || '')}"><div class="hint">App 与自动化脚本可用 X-Ink-Token 调用接口。</div></div>
        </div>
        <div class="switch"><div><div class="sw-label">需要登录</div><div class="sw-hint">关闭后同一网络内任何人都能打开；通过花生壳暴露到公网时请务必开启</div></div><button class="toggle ${s.requireAuth ? 'on' : ''}" id="setAuth"></button></div>
        <div class="switch"><div><div class="sw-label">信任反向代理头</div><div class="sw-hint">花生壳、Nginx、Caddy 转发时识别真实协议与域名，生成正确的分享链接</div></div><button class="toggle ${s.trustProxy ? 'on' : ''}" id="setProxy"></button></div>
        <div class="switch"><div><div class="sw-label">强制 HTTPS Cookie</div><div class="sw-hint">当公网地址是 https 时建议开启，防止会话被明文传输</div></div><button class="toggle ${s.forceHttps ? 'on' : ''}" id="setHttps"></button></div>
        <div class="row3">
          <div class="field"><label>登录有效期（天）</label><input class="input" id="setSession" type="number" value="${s.sessionDays || 180}"></div>
          <div class="field"><label>单文件上传上限（MB）</label><input class="input" id="setUpload" type="number" value="${s.maxUploadMb || 64}"></div>
          <div class="field"><label>端口</label><input class="input" value="${s.port || 8790}" disabled></div>
        </div>
        <div class="field"><label>当前访问地址</label>
          <div style="display:flex;flex-wrap:wrap;gap:6px">${(s.lanUrls || []).map((u) => `<span class="fmt-badge">${esc(u)}</span>`).join('')}<span class="fmt-badge">http://127.0.0.1:${s.port || 8790}</span>${s.publicUrl ? '<span class="fmt-badge" style="border-color:var(--accent);color:var(--accent)">' + esc(s.publicUrl) + '</span>' : ''}</div>
          <div class="hint">花生壳：映射本机 TCP ${s.port || 8790} 端口，把分配到的域名填入上方「公网地址」，即可在手机和外地电脑访问同一套数据。</div>
        </div>
      </div>
      <div data-panel="structure" class="hidden" id="structurePanel"></div>
      <div data-panel="backup" class="hidden">
        <div class="switch"><div><div class="sw-label">自动备份</div><div class="sw-hint">在后台定期把数据库与附件打包到 data/backups</div></div><button class="toggle ${s.autoBackup ? 'on' : ''}" id="setAutoBackup"></button></div>
        <div class="row2">
          <div class="field"><label>保留份数</label><input class="input" id="setKeep" type="number" value="${s.backupKeep || 14}"></div>
          <div class="field"><label>备份间隔（小时）</label><input class="input" id="setEvery" type="number" value="${s.backupEveryHours || 24}"></div>
        </div>
        <div class="field"><label>数据位置</label><div class="hint" style="margin:0;font-family:var(--mono)">${esc(s.dbPath || '')}<br>${esc(s.filesDir || '')}</div></div>
        <div style="display:flex;gap:8px;flex-wrap:wrap;margin-top:10px">
          <button class="btn small" id="openBackupPanel">${icon('database', 14)}备份与恢复</button>
          <button class="btn small" id="exportVaultZip">${icon('download', 14)}导出整库 ZIP</button>
          <button class="btn small" id="reindexBtn">${icon('refresh', 14)}重建搜索索引</button>
        </div>
      </div>
      <div data-panel="about" class="hidden">
        <div style="display:flex;gap:14px;align-items:center;margin-bottom:18px">
          <div class="seal" style="margin:0;width:48px;height:48px;font-size:26px">墨</div>
          <div><div style="font-family:var(--serif);font-size:20px">墨库 InkVault</div><div style="font-size:11.5px;color:var(--faint)">v${esc(s.version || '1.0.0')} · 本地优先的个人知识库</div></div>
        </div>
        <p style="font-size:12.5px;color:var(--muted);line-height:1.9">数据完全保存在你自己的电脑上：SQLite 单文件数据库 + 附件目录，可随时复制、备份、迁移。没有云端账号，没有订阅，没有锁定。</p>
        <div class="table" style="margin-top:8px"><table class="table">
          <tr><td>数据库</td><td style="font-family:var(--mono);font-size:11px">${esc(s.dbPath || '')}</td></tr>
          <tr><td>全文检索</td><td>${s.fts ? 'FTS5 · 三元组索引（支持中文）' : 'LIKE 模糊匹配'}</td></tr>
          <tr><td>快捷键</td><td><kbd>⌘K</kbd> 搜索 · <kbd>⌘N</kbd> 新建 · <kbd>⌘S</kbd> 保存 · <kbd>⌘B</kbd> 加粗</td></tr>
        </table></div>
        <div style="display:flex;gap:8px;margin-top:16px"><button class="btn small" id="auditBtn">${icon('shield', 14)}查看访问日志</button><button class="btn small" id="logoutBtn">${icon('key', 14)}退出登录</button></div>
      </div>`;

    const card = Modal.open({
      title: '设置',
      subtitle: '外观、访问、备份与数据',
      wide: true,
      body,
      footer: `<span style="font-size:11px;color:var(--faint)">修改访问码后，其他设备需要重新登录</span>
        <div class="right"><button class="btn ghost" data-close>取消</button><button class="btn primary" id="saveSettings">保存设置</button></div>`,
      onMount(root) {
        root.querySelectorAll('[data-tab]').forEach((b) => b.onclick = () => {
          root.querySelectorAll('[data-tab]').forEach((x) => x.classList.remove('active'));
          b.classList.add('active');
          root.querySelectorAll('[data-panel]').forEach((p) => p.classList.toggle('hidden', p.getAttribute('data-panel') !== b.getAttribute('data-tab')));
        });
        const toggle = (sel) => { const el = root.querySelector(sel); if (el) el.onclick = () => el.classList.toggle('on'); };
        ['#setAuth', '#setProxy', '#setHttps', '#setAutoBackup', '#setOutline'].forEach(toggle);
        root.querySelectorAll('#themeSeg button').forEach((b) => b.onclick = () => {
          root.querySelectorAll('#themeSeg button').forEach((x) => x.classList.remove('active'));
          b.classList.add('active'); window.applyTheme(b.getAttribute('data-theme'));
        });
        root.querySelectorAll('#fontSeg button').forEach((b) => b.onclick = () => {
          root.querySelectorAll('#fontSeg button').forEach((x) => x.classList.remove('active'));
          b.classList.add('active'); window.applyFont(+b.getAttribute('data-font'));
        });
        root.querySelectorAll('#widthSeg button').forEach((b) => b.onclick = () => {
          root.querySelectorAll('#widthSeg button').forEach((x) => x.classList.remove('active'));
          b.classList.add('active'); window.applyWidth(+b.getAttribute('data-width'));
        });
        root.querySelectorAll('#modeSeg button').forEach((b) => b.onclick = () => {
          root.querySelectorAll('#modeSeg button').forEach((x) => x.classList.remove('active'));
          b.classList.add('active'); UI.setViewMode(b.getAttribute('data-mode'));
        });
        root.querySelector('#setOutline').onclick = (e) => {
          e.currentTarget.classList.toggle('on');
          S.outline = e.currentTarget.classList.contains('on');
          UI.renderOutline();
        };
        root.querySelector('#saveSettings').onclick = async () => {
          const payload = {
            vaultName: root.querySelector('#setVaultName').value.trim(),
            publicUrl: root.querySelector('#setPublicUrl').value.trim(),
            pin: root.querySelector('#setPin').value.trim(),
            token: root.querySelector('#setToken').value.trim(),
            requireAuth: root.querySelector('#setAuth').classList.contains('on'),
            trustProxy: root.querySelector('#setProxy').classList.contains('on'),
            forceHttps: root.querySelector('#setHttps').classList.contains('on'),
            sessionDays: +root.querySelector('#setSession').value || 180,
            maxUploadMb: +root.querySelector('#setUpload').value || 64,
            autoBackup: root.querySelector('#setAutoBackup').classList.contains('on'),
            backupKeep: +root.querySelector('#setKeep').value || 14,
            backupEveryHours: +root.querySelector('#setEvery').value || 24,
            theme: S.theme,
            editorWidth: +localStorage.getItem('iv.width') || 1080
          };
          try {
            const r = await store().saveSettings(payload);
            S.settings.vaultName = payload.vaultName;
            $('#vaultName').textContent = payload.vaultName;
            if (r.pin) S.settings.pin = r.pin;
            Utils.toast('设置已保存', 'success');
            Modal.close();
          } catch (e) { Utils.toast(e.message, 'error'); }
        };
        if (root.querySelector('#structurePanel')) buildStructure(root.querySelector('#structurePanel'));
        const bind = (id, fn) => { const el = root.querySelector(id); if (el) el.onclick = fn; };
        bind('#openBackupPanel', () => { Modal.close(); P.openBackup(); });
        bind('#exportVaultZip', () => { store().exportVault('zip'); });
        bind('#reindexBtn', async () => { try { await store().reindex(); Utils.toast('索引已重建', 'success'); } catch (e) { Utils.toast(e.message, 'error'); } });
        bind('#auditBtn', () => P.openAudit());
        bind('#logoutBtn', async () => { await store().logout(); location.reload(); });
        bind('#switchModeBtn', () => { if (window.AndroidBridge) window.AndroidBridge.openSetup(); });
      }
    });
    return card;
  };

  function buildStructure(panel) {
    const esc2 = Utils.esc;
    const fieldRows = (S.fields || []).map((f) => `<div class="opt-line">
      <div><div class="lbl">${esc2(f.name)}</div><div class="sub">${esc2(f.type)}${f.scope ? ' · ' + esc2((flattenNotebooks(S.notebooks).find((n) => n.uid === f.scope) || {}).name || '指定笔记本') : ' · 全局'}</div></div>
      <button class="icon-btn" data-del-field="${f.uid}" title="删除">${icon('trash', 14)}</button></div>`).join('') || '<div style="color:var(--faint);font-size:12px;padding:8px 0">还没有自定义字段。字段可以给笔记增加「来源」「评分」「阅读状态」等结构化属性。</div>';
    const tplRows = (S.templates || []).map((t) => `<div class="opt-line">
      <div><div class="lbl">${esc2(t.name)}</div><div class="sub">${(t.content || '').slice(0, 40).replace(/\n/g, ' ')}…</div></div>
      <div style="display:flex;gap:2px"><button class="icon-btn" data-edit-tpl="${t.uid}" title="编辑">${icon('edit', 14)}</button><button class="icon-btn" data-del-tpl="${t.uid}" title="删除">${icon('trash', 14)}</button></div></div>`).join('') || '<div style="color:var(--faint);font-size:12px;padding:8px 0">还没有模板。模板适合周报、会议记录、读书笔记这类重复结构。</div>';
    panel.innerHTML = `<h3 style="font-family:var(--serif);font-size:16px;margin:0 0 10px">自定义字段</h3>
      <div class="stat-section" style="padding:14px 16px">${fieldRows}</div>
      <div class="row3" style="margin-top:10px">
        <div class="field" style="margin:0"><label>字段名称</label><input class="input" id="fldName" placeholder="例如：来源"></div>
        <div class="field" style="margin:0"><label>类型</label><select class="select" id="fldType"><option value="text">文本</option><option value="number">数字</option><option value="date">日期</option><option value="select">单选</option><option value="multiselect">多选</option><option value="checkbox">勾选</option><option value="url">链接</option></select></div>
        <div class="field" style="margin:0"><label>作用范围</label><select class="select" id="fldScope"><option value="">全部笔记</option>${flattenNotebooks(S.notebooks).map((n) => `<option value="${n.uid}">${esc2(n.name)}</option>`).join('')}</select></div>
      </div>
      <div style="display:flex;gap:10px;align-items:flex-end;margin-bottom:22px">
        <div class="field" style="margin:0;flex:1"><label>选项（单选/多选时用逗号分隔）</label><input class="input" id="fldOptions" placeholder="未读, 在读, 已读"></div>
        <button class="btn small" id="addField">${icon('plus', 14)}添加字段</button>
      </div>
      <h3 style="font-family:var(--serif);font-size:16px;margin:6px 0 10px">笔记模板</h3>
      <div class="stat-section" style="padding:14px 16px">${tplRows}</div>
      <div class="row2" style="margin-top:10px">
        <div class="field" style="margin:0"><label>模板名称</label><input class="input" id="tplName" placeholder="例如：每周复盘"></div>
        <div class="field" style="margin:0"><label>模板内容（Markdown）</label><input class="input" id="tplContent" placeholder="# 本周复盘&#10;&#10;## 做成了什么&#10;&#10;## 下周计划"></div>
      </div>
      <button class="btn small" id="addTpl">${icon('plus', 14)}保存模板</button>
      <div class="hint" style="margin-top:10px">模板可以在命令面板（⌘K）里直接调用，新建一篇带结构的笔记。</div>`;

    panel.querySelectorAll('[data-del-field]').forEach((b) => b.onclick = async () => {
      if (!(await Dialogs.confirm({ title: '删除字段', message: '字段会从所有笔记上移除，笔记正文不受影响。', danger: true, okText: '删除' }))) return;
      await store().deleteField(b.getAttribute('data-del-field'));
      await window.refreshAll();
      const root = panel.closest('.modal-card');
      Modal.close(); P.openSettings();
    });
    panel.querySelectorAll('[data-del-tpl]').forEach((b) => b.onclick = async () => {
      await store().deleteTemplate(b.getAttribute('data-del-tpl'));
      S.templates = await store().listTemplates();
      Modal.close(); P.openSettings();
    });
    panel.querySelectorAll('[data-edit-tpl]').forEach((b) => b.onclick = async () => {
      const t = (S.templates || []).find((x) => x.uid === b.getAttribute('data-edit-tpl'));
      if (!t) return;
      const name = await Dialogs.prompt({ title: '模板名称', label: '名称', value: t.name });
      if (!name) return;
      const content = await Dialogs.prompt({ title: '模板内容', label: 'Markdown', value: t.content, multiline: true });
      if (content === null) return;
      await store().saveTemplate(Object.assign({}, t, { name, content }));
      S.templates = await store().listTemplates();
      Modal.close(); P.openSettings();
    });
    panel.querySelector('#addField').onclick = async () => {
      const name = panel.querySelector('#fldName').value.trim();
      if (!name) { Utils.toast('请填写字段名称', 'error'); return; }
      const options = panel.querySelector('#fldOptions').value.split(/[,，]/).map((x) => x.trim()).filter(Boolean);
      await store().saveField({ name, type: panel.querySelector('#fldType').value, scope: panel.querySelector('#fldScope').value, options, sort: (S.fields || []).length });
      await window.refreshAll();
      Modal.close(); P.openSettings();
      Utils.toast('字段已添加', 'success');
    };
    panel.querySelector('#addTpl').onclick = async () => {
      const name = panel.querySelector('#tplName').value.trim();
      const content = panel.querySelector('#tplContent').value;
      if (!name || !content) { Utils.toast('请填写模板名称和内容', 'error'); return; }
      await store().saveTemplate({ name, content });
      S.templates = await store().listTemplates();
      Modal.close(); P.openSettings();
      Utils.toast('模板已保存', 'success');
    };
  }

  P.openAudit = async function () {
    let rows = [];
    try { rows = await store().audit(); } catch (e) {}
    Modal.open({
      title: '访问日志', subtitle: '最近的登录与写操作', wide: true,
      body: `<table class="table"><thead><tr><th>时间</th><th>动作</th><th>详情</th><th>IP</th></tr></thead><tbody>
        ${rows.slice(0, 80).map((r) => `<tr><td>${Utils.fmtDate(r.createdAt, true)}</td><td>${esc(r.action)}</td><td>${esc(r.detail || '')}</td><td>${esc(r.ip || '')}</td></tr>`).join('') || '<tr><td colspan="4" style="color:var(--faint)">暂无记录</td></tr>'}
      </tbody></table>`,
      footer: `<span style="font-size:11px;color:var(--faint)">日志保存在本地数据库</span><div class="right"><button class="btn ghost" data-close>关闭</button></div>`
    });
  };

  /* ------------------------------- backup --------------------------------- */
  P.openBackup = async function () {
    let data = { backups: [] };
    try { data = await store().backups(); } catch (e) {}
    const list = data.backups || [];
    const rows = list.map((b) => `<tr>
      <td style="font-family:var(--mono);font-size:11px">${esc(b.name)}</td>
      <td>${Utils.fmtBytes(b.size)}</td>
      <td>${Utils.fmtDate(b.createdAt, true)}</td>
      <td>${esc(b.reason || '')}</td>
      <td style="text-align:right;white-space:nowrap">
        <button class="icon-btn" data-dl="${esc(b.name)}" title="下载">${icon('download', 15)}</button>
        <button class="icon-btn" data-restore="${esc(b.name)}" title="恢复">${icon('rotate', 15)}</button>
        <button class="icon-btn" data-del="${esc(b.name)}" title="删除">${icon('trash', 15)}</button>
      </td></tr>`).join('');
    const body = `<div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:16px">
        <button class="btn primary" id="newBackup">${icon('database', 15)}立即备份</button>
        <button class="btn" id="exportVault">${icon('download', 15)}导出整库 ZIP</button>
        <button class="btn" id="exportJson">${icon('file-down', 15)}导出 JSON</button>
      </div>
      <div class="stat-section" style="margin-bottom:16px">
        <h3>备份存档 <span class="sub">${list.length} 份 · ${esc(data.dir || 'data/backups')}</span></h3>
        <table class="table"><thead><tr><th>文件</th><th>大小</th><th>时间</th><th>类型</th><th></th></tr></thead>
        <tbody>${rows || '<tr><td colspan="5" style="color:var(--faint);padding:24px;text-align:center">还没有备份，建议立即创建一份</td></tr>'}</tbody></table>
      </div>
      <div class="hint" style="line-height:1.9">备份包含完整的 SQLite 数据库、全部附件与版本历史，可直接复制到移动硬盘或网盘。
      恢复前系统会自动再备份一次当前数据，避免误操作。自动备份在后台运行，默认每天一次、保留 14 份。</div>`;
    Modal.open({
      title: '备份与恢复', subtitle: '数据安全是最高优先级', wide: true, body,
      footer: `<span style="font-size:11px;color:var(--faint)">SQLite + 附件目录，可离线保存数十年</span><div class="right"><button class="btn ghost" data-close>关闭</button></div>`,
      onMount(root) {
        root.querySelector('#newBackup').onclick = async (e) => {
          const btn = e.currentTarget; btn.disabled = true; btn.textContent = '正在备份…';
          try {
            const r = await store().createBackup();
            Utils.toast('已创建备份 ' + r.name, 'success');
            Modal.close(); P.openBackup();
          } catch (err) { Utils.toast(err.message, 'error'); btn.disabled = false; btn.textContent = '立即备份'; }
        };
        root.querySelector('#exportVault').onclick = () => store().exportVault('zip');
        root.querySelector('#exportJson').onclick = () => store().exportVault('json');
        root.querySelectorAll('[data-dl]').forEach((b) => b.onclick = () => store().downloadBackup(b.getAttribute('data-dl')));
        root.querySelectorAll('[data-del]').forEach((b) => b.onclick = async () => {
          if (!(await Dialogs.confirm({ title: '删除备份', message: '确定删除这份备份文件？此操作不可撤销。', danger: true, okText: '删除' }))) return;
          await store().deleteBackup(b.getAttribute('data-del'));
          Modal.close(); P.openBackup();
        });
        root.querySelectorAll('[data-restore]').forEach((b) => b.onclick = async () => {
          const name = b.getAttribute('data-restore');
          if (!(await Dialogs.confirm({ title: '从备份恢复', message: '将用 <b>' + esc(name) + '</b> 覆盖当前数据库。恢复前会自动备份当前数据。', danger: true, okText: '恢复' }))) return;
          try {
            await store().restoreBackup(name);
            Utils.toast('已恢复，正在重新载入…', 'success');
            setTimeout(() => location.reload(), 900);
          } catch (e) { Utils.toast(e.message, 'error'); }
        });
      }
    });
  };

  /* -------------------------------- sync ---------------------------------- */
  P.openSync = async function () {
    let s = {};
    try { s = await store().settings(); } catch (e) {}
    const qr = s.qr ? `<img src="${s.qr}" alt="访问二维码" style="width:150px;height:150px;border-radius:10px;border:1px solid var(--line);background:#fff;padding:6px">` : '';
    const body = `<div style="display:grid;grid-template-columns:1fr 1fr;gap:20px">
      <div>
        <h3 style="font-family:var(--serif);font-size:16px;margin:0 0 8px">在手机 / 外地电脑访问</h3>
        <p style="font-size:12.5px;color:var(--muted);line-height:1.9;margin:0 0 12px">
          服务端已经监听 <b>0.0.0.0:${s.port || 8790}</b>。同一 Wi-Fi 下直接用局域网地址；在外网用花生壳把本机 TCP ${s.port || 8790} 映射出去，然后把域名填到设置里的「公网地址」。
        </p>
        <div style="display:flex;flex-wrap:wrap;gap:6px;margin-bottom:10px">
          <span class="fmt-badge">http://127.0.0.1:${s.port || 8790}</span>
          ${(s.lanUrls || []).map((u) => `<span class="fmt-badge">${esc(u)}</span>`).join('')}
          ${s.publicUrl ? `<span class="fmt-badge" style="border-color:var(--accent);color:var(--accent)">${esc(s.publicUrl)}</span>` : ''}
        </div>
        <div class="field"><label>访问码</label><input class="input" value="${esc(s.pin || '')}" readonly></div>
        <div class="field"><label>API 令牌（App 同步用）</label><input class="input" value="${esc(s.token || '')}" readonly></div>
        <div class="hint">Android App 支持离线写作，并在连回服务端时按更新时间合并。iOS 版使用同一套协议。</div>
      </div>
      <div style="text-align:center">
        ${qr || '<div style="padding:30px;color:var(--faint);font-size:12px">在设置中填写公网地址后，这里会显示扫码入口</div>'}
        <div class="hint" style="text-align:left;margin-top:14px">
          <b style="color:var(--ink-2)">花生壳映射步骤</b><br>
          1. 花生壳客户端 → 内网穿透 → 添加映射<br>
          2. 应用类型选 TCP 或 HTTP，内网主机 127.0.0.1，端口 ${s.port || 8790}<br>
          3. 把分配到的域名写入「设置 → 公网地址」<br>
          4. 手机浏览器打开域名，输入访问码即可
        </div>
      </div>
    </div>
    ${store().kind === 'local' ? `<div class="stat-section" style="margin-top:18px"><h3>连接电脑端</h3>
      <div class="row2"><div class="field"><label>服务端地址</label><input class="input" id="syncUrl" placeholder="http://192.168.1.10:8790 或花生壳域名"></div>
      <div class="field"><label>访问码</label><input class="input" id="syncPin" placeholder="6 位访问码"></div></div>
      <div style="display:flex;gap:8px"><button class="btn primary" id="syncNow">${icon('refresh', 15)}双向同步</button></div>
      </div>` : ''}`;
    Modal.open({
      title: '设备与同步', subtitle: '一套数据，多端访问', wide: true, body,
      footer: `<span style="font-size:11px;color:var(--faint)">所有流量都走你自己的服务端，不经过第三方</span><div class="right"><button class="btn ghost" data-close>关闭</button></div>`,
      onMount(root) {
        const btn = root.querySelector('#syncNow');
        if (btn) btn.onclick = async () => {
          const url = root.querySelector('#syncUrl').value.trim();
          const pin = root.querySelector('#syncPin').value.trim();
          if (!url || !pin) { Utils.toast('请填写地址和访问码', 'error'); return; }
          btn.disabled = true; btn.textContent = '正在同步…';
          try {
            const r = await store().sync(url, pin);
            Utils.toast('同步完成：+' + r.created + ' 更新 ' + r.updated, 'success');
            await window.refreshAll();
            Modal.close();
          } catch (e) { Utils.toast(e.message, 'error'); btn.disabled = false; btn.innerHTML = '双向同步'; }
        };
      }
    });
  };

  /* ------------------------------ versions -------------------------------- */
  P.openVersions = async function () {
    if (!S.current) return;
    let list = [];
    try { list = await store().versions(S.current.uid); } catch (e) {}
    const body = `<div class="hint" style="margin-bottom:12px">每次内容变化都会自动保存一个版本，最多保留 40 个。</div>
      <div id="versionList">${list.map((v) => `<div class="opt-line" style="cursor:pointer" data-vid="${v.id}">
        <div><div class="lbl">${Utils.fmtDate(v.createdAt, true)}</div><div class="sub">${v.reason === 'before-restore' ? '恢复前快照' : '编辑版本'} · ${Utils.fmtNum(v.wordCount)} 字</div></div>
        <button class="btn small" data-preview="${v.id}">查看</button></div>`).join('') || '<div style="color:var(--faint);font-size:12px;padding:20px">还没有历史版本</div>'}</div>`;
    Modal.open({
      title: '历史版本', subtitle: S.current.title || '', narrow: true, body,
      footer: `<span style="font-size:11px;color:var(--faint)">版本保存在本地数据库</span><div class="right"><button class="btn ghost" data-close>关闭</button></div>`,
      onMount(root) {
        root.querySelectorAll('[data-preview]').forEach((b) => b.onclick = async () => {
          const id = b.getAttribute('data-preview');
          try {
            const full = await store().versions(S.current.uid, true);
            const v = full.find((x) => String(x.id) === String(id));
            if (!v) return;
            Modal.open({
              title: '版本预览', subtitle: Utils.fmtDate(v.createdAt, true) + ' · ' + Utils.fmtNum(v.wordCount) + ' 字',
              wide: true,
              body: `<div class="md" style="max-height:60vh;overflow:auto">${MD.render(v.content || '').html}</div>`,
              footer: `<span style="font-size:11px;color:var(--faint)">恢复后会先保存当前内容为新版本</span>
                <div class="right"><button class="btn ghost" data-close>关闭</button><button class="btn primary" id="doRestore">恢复此版本</button></div>`,
              onMount(card) {
                card.querySelector('#doRestore').onclick = async () => {
                  try {
                    const n = await store().restoreVersion(S.current.uid, v.id);
                    S.current = n; Modal.close(); UI.renderEditor(n); UI.renderNotes(); await window.refreshAll();
                    Utils.toast('已恢复该版本', 'success');
                  } catch (e) { Utils.toast(e.message, 'error'); }
                };
              }
            });
          } catch (e) { Utils.toast(e.message, 'error'); }
        });
      }
    });
  };

  /* ------------------------------- note menu ------------------------------ */
  P.openNoteMenu = function () {
    const n = S.current;
    if (!n) return;
    const html = `<div class="drawer-head"><h3>笔记操作</h3><button class="icon-btn" data-drawer-close>${icon('x', 16)}</button></div>
      <div class="drawer-body">
        <div class="menu-group"><div class="mg-title">导出</div>
          ${[['md', 'Markdown 原文'], ['html', '网页（自包含图片）'], ['docx', 'Word 文档'], ['txt', '纯文本'], ['enex', '印象笔记 ENEX'], ['json', 'JSON 数据']].map((x) =>
            `<button class="menu-item" data-export="${x[0]}">${icon('file-down', 15)}<span>${x[1]}</span><span class="shortcut">.${x[0]}</span></button>`).join('')}
        </div>
        <div class="menu-sep"></div>
        <div class="menu-group"><div class="mg-title">整理</div>
          <button class="menu-item" data-act="duplicate">${icon('copy', 15)}<span>创建副本</span></button>
          <button class="menu-item" data-act="index">${icon('list-tree', 15)}<span>为本笔记本生成目录笔记</span></button>
          <button class="menu-item" data-act="info">${icon('info', 15)}<span>笔记信息</span></button>
        </div>
        <div class="menu-sep"></div>
        <div class="menu-group"><div class="mg-title">危险操作</div>
          ${S.trashed || n.deletedAt
            ? `<button class="menu-item" data-act="restore">${icon('rotate', 15)}<span>从回收站恢复</span></button>
               <button class="menu-item danger" data-act="purge">${icon('trash', 15)}<span>永久删除</span></button>`
            : `<button class="menu-item" data-act="archive">${icon('archive', 15)}<span>${n.status === 'archived' ? '取消归档' : '归档这篇笔记'}</span></button>
               <button class="menu-item danger" data-act="delete">${icon('trash', 15)}<span>移到回收站</span></button>`}
        </div>
      </div>`;
    Modal.drawer(html, (root) => {
      root.querySelector('[data-drawer-close]').onclick = Modal.closeDrawer;
      root.querySelectorAll('[data-export]').forEach((b) => b.onclick = () => { store().exportNote(n.uid, b.getAttribute('data-export')); Modal.closeDrawer(); Utils.toast('正在导出…', 'success'); });
      root.querySelectorAll('[data-act]').forEach((b) => b.onclick = async () => {
        const act = b.getAttribute('data-act');
        try {
          if (act === 'duplicate') { const c = await store().duplicateNote(n.uid); Modal.closeDrawer(); await window.refreshAll(); window.openNote(c.uid); }
          if (act === 'index') { const c = await store().makeIndexNote(n.notebookUid); Modal.closeDrawer(); await window.refreshAll(); window.openNote(c.uid); Utils.toast('目录笔记已生成', 'success'); }
          if (act === 'info') { Modal.closeDrawer(); P.openNoteInfo(); }
          if (act === 'delete') { await store().deleteNote(n.uid); Modal.closeDrawer(); S.current = null; await window.refreshAll(); Utils.toast('已移到回收站', 'success'); }
          if (act === 'restore') { await store().restoreNote(n.uid); Modal.closeDrawer(); await window.refreshAll(); Utils.toast('已恢复', 'success'); }
          if (act === 'purge') {
            Modal.closeDrawer();
            if (await Dialogs.confirm({ title: '永久删除', message: '这篇笔记及其附件、历史版本将被永久删除，无法恢复。', danger: true, okText: '永久删除' })) {
              await store().purgeNote(n.uid); S.current = null; await window.refreshAll(); Utils.toast('已永久删除', 'success');
            }
          }
          if (act === 'archive') {
            await store().patchNote(n.uid, { status: n.status === 'archived' ? 'active' : 'archived' });
            Modal.closeDrawer(); await window.refreshAll(); Utils.toast('已归档', 'success');
          }
        } catch (e) { Utils.toast(e.message, 'error'); }
      });
    });
  };

  P.openNoteInfo = function () {
    const n = S.current;
    if (!n) return;
    Modal.open({
      title: '笔记信息', narrow: true,
      body: `<table class="table">
        <tr><td>标题</td><td>${esc(n.title)}</td></tr>
        <tr><td>笔记本</td><td>${esc(n.notebookName || '')}</td></tr>
        <tr><td>标签</td><td>${(n.tags || []).map((t) => '#' + esc(t)).join(' ') || '—'}</td></tr>
        <tr><td>字数</td><td>${Utils.fmtNum(n.wordCount)}</td></tr>
        <tr><td>创建</td><td>${Utils.fmtDate(n.createdAt, true)}</td></tr>
        <tr><td>更新</td><td>${Utils.fmtDate(n.updatedAt, true)}</td></tr>
        <tr><td>附件</td><td>${(n.attachments || []).length} 个</td></tr>
        <tr><td>历史版本</td><td>${n.versionCount || 0} 个</td></tr>
        <tr><td>UID</td><td style="font-family:var(--mono);font-size:10.5px">${esc(n.uid)}</td></tr>
      </table>`,
      footer: `<div class="right"><button class="btn ghost" data-close>关闭</button></div>`
    });
  };

  /* -------------------------------- sort ---------------------------------- */
  P.openSort = function () {
    const opts = S.query
      ? [['relevance', '相关度'], ['updated_desc', '最近更新'], ['created_desc', '最近创建'], ['title', '标题'], ['words', '字数最多']]
      : [['updated_desc', '最近更新'], ['created_desc', '最近创建'], ['title', '标题'], ['words', '字数最多'], ['oldest', '最早更新']];
    Modal.open({
      title: '排序方式', narrow: true,
      body: `<div style="display:flex;flex-direction:column;gap:2px">${opts.map((o) =>
        `<button class="menu-item" data-sort="${o[0]}">${icon('sort', 15)}<span>${o[1]}</span>${(S.sort || '') === o[0] ? '<span class="shortcut">当前</span>' : ''}</button>`).join('')}</div>`,
      footer: `<div class="right"><button class="btn ghost" data-close>关闭</button></div>`,
      onMount(root) {
        root.querySelectorAll('[data-sort]').forEach((b) => b.onclick = () => {
          S.sort = b.getAttribute('data-sort'); Modal.close(); window.refreshList();
        });
      }
    });
  };

  /* ------------------------------- palette -------------------------------- */
  let palTimer = null;
  P.openPalette = function () {
    const el = $('#palette');
    el.classList.remove('hidden');
    const input = $('#paletteInput');
    input.value = '';
    input.focus();
    S.paletteIndex = 0;
    renderPalette('');
    input.oninput = () => {
      clearTimeout(palTimer);
      palTimer = setTimeout(() => renderPalette(input.value.trim()), 160);
    };
    input.onkeydown = (e) => {
      if (e.key === 'Escape') { P.closePalette(); return; }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const items = $$('#paletteResults .pal-item');
        if (!items.length) return;
        S.paletteIndex = (S.paletteIndex + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length;
        items.forEach((x, i) => x.classList.toggle('active', i === S.paletteIndex));
        items[S.paletteIndex].scrollIntoView({ block: 'nearest' });
      }
      if (e.key === 'Enter') {
        const items = $$('#paletteResults .pal-item');
        if (items[S.paletteIndex]) items[S.paletteIndex].click();
      }
    };
    el.onclick = (e) => { if (e.target === el) P.closePalette(); };
  };
  P.closePalette = function () { $('#palette').classList.add('hidden'); };

  function renderPalette(q) {
    const results = $('#paletteResults');
    const actions = [
      { icon: 'plus', title: '新建笔记', sub: '⌘N', run: () => window.createNote() },
      ...(S.templates || []).map((t) => ({ icon: 'file-text', title: '模板：' + t.name, sub: '新建', run: () => window.createNote({ title: t.name, content: t.content, notebookUid: t.notebookUid || undefined }) })),
      { icon: 'download', title: '导出目录 / 摘要', sub: '', run: () => P.openTOC() },
      { icon: 'chart', title: '数据与统计', sub: '', run: () => P.openStats() },
      { icon: 'upload', title: '导入笔记', sub: '', run: () => P.openImport() },
      { icon: 'database', title: '备份与恢复', sub: '', run: () => P.openBackup() },
      { icon: 'settings', title: '设置', sub: '', run: () => P.openSettings() },
      { icon: 'star', title: '查看星标', sub: '', run: () => window.setView('starred') },
      { icon: 'trash', title: '打开回收站', sub: '', run: () => window.setView('trash') }
    ].filter((a) => !q || a.title.includes(q) || a.title.toLowerCase().includes(q.toLowerCase()));

    let html = '';
    if (actions.length) {
      html += '<div class="pal-group">操作</div>' + actions.slice(0, 5).map((a) =>
        `<button class="pal-item" data-act="${a.title}"><span class="pi-icon">${icon(a.icon, 16)}</span><span class="pi-title">${a.title}</span><span class="pi-sub">${a.sub}</span></button>`).join('');
    }
    if (q) {
      html += '<div class="pal-group">搜索笔记</div><div id="palNotes"><div class="pal-item" style="color:var(--faint)">正在搜索…</div></div>';
    } else {
      html += '<div class="pal-group">最近</div>';
    }
    results.innerHTML = html;
    results.querySelectorAll('[data-act]').forEach((b) => b.onclick = () => {
      const a = actions.find((x) => x.title === b.getAttribute('data-act'));
      P.closePalette(); if (a) a.run();
    });
    if (!q) {
      store().listNotes({ limit: 5, sort: 'updated_desc' }).then((r) => {
        const box = results;
        if (!box) return;
        const items = (r.notes || []).map((n) => paletteNote(n)).join('');
        box.insertAdjacentHTML('beforeend', items || '');
        bindNotes(box);
        const first = box.querySelector('.pal-item');
        if (first) first.classList.add('active');
      }).catch(() => {});
    } else {
      store().listNotes({ q, limit: 8 }).then((r) => {
        const box = $('#palNotes');
        if (!box) return;
        box.innerHTML = (r.notes || []).map((n) => paletteNote(n)).join('') ||
          '<div class="pal-item" style="color:var(--faint)">没有找到匹配笔记</div>';
        bindNotes(box);
        S.paletteIndex = 0;
        const items = $$('#paletteResults .pal-item');
        items.forEach((x, i) => x.classList.toggle('active', i === 0));
      }).catch(() => {});
    }
  }
  function paletteNote(n) {
    return `<button class="pal-item" data-open="${n.uid}"><span class="pi-icon">${icon('file-text', 16)}</span>
      <span class="pi-title">${esc(n.title)}</span><span class="pi-sub">${esc(n.notebookName || '')}</span></button>`;
  }
  function bindNotes(root) {
    root.querySelectorAll('[data-open]').forEach((b) => b.onclick = () => {
      P.closePalette();
      window.openNote(b.getAttribute('data-open'));
    });
  }

  P.openNotebookMenu = function (uid, name) {
    Modal.drawer(`<div class="drawer-head"><h3>${esc(name || '笔记本')}</h3><button class="icon-btn" data-drawer-close>${icon('x', 16)}</button></div>
      <div class="drawer-body">
        <div class="menu-group"><div class="mg-title">整理</div>
          <button class="menu-item" data-nbact="child">${icon('folder', 15)}<span>新建子笔记本</span></button>
          <button class="menu-item" data-nbact="rename">${icon('edit', 15)}<span>重命名</span></button>
          <button class="menu-item" data-nbact="index">${icon('list-tree', 15)}<span>生成目录笔记</span></button>
          <button class="menu-item" data-nbact="export">${icon('download', 15)}<span>导出这个笔记本</span></button>
        </div>
        <div class="menu-sep"></div>
        <div class="menu-group"><div class="mg-title">危险操作</div>
          <button class="menu-item danger" data-nbact="delete">${icon('trash', 15)}<span>删除笔记本（笔记移到上级）</span></button>
        </div>
      </div>`, (root) => {
      root.querySelector('[data-drawer-close]').onclick = Modal.closeDrawer;
      root.querySelectorAll('[data-nbact]').forEach((b) => b.onclick = async () => {
        const act = b.getAttribute('data-nbact');
        try {
          if (act === 'child') {
            const n = await Dialogs.prompt({ title: '新建子笔记本', label: '名称' });
            if (!n) return;
            await store().createNotebook({ name: n, parentUid: uid });
            Modal.closeDrawer(); await window.refreshAll();
          }
          if (act === 'rename') {
            const n = await Dialogs.prompt({ title: '重命名笔记本', label: '名称', value: name });
            if (!n) return;
            await store().updateNotebook(uid, { name: n, parentUid: '' });
            Modal.closeDrawer(); await window.refreshAll();
          }
          if (act === 'index') {
            const note = await store().makeIndexNote(uid);
            Modal.closeDrawer(); await window.refreshAll(); window.openNote(note.uid);
            Utils.toast('目录笔记已生成', 'success');
          }
          if (act === 'export') {
            Modal.closeDrawer();
            const p = Object.assign(currentFilterParams(), { notebook: uid, format: 'md', groupBy: 'notebook', summary: 1, tags: 1, dates: 1, words: 1, numbered: 1, title: name + ' 目录' });
            store().exportTOC(p);
            Utils.toast('正在导出笔记本目录…', 'success');
          }
          if (act === 'delete') {
            Modal.closeDrawer();
            if (await Dialogs.confirm({ title: '删除笔记本', message: '「' + esc(name) + '」会被删除，里面的笔记会移动到上级笔记本，不会丢失。', danger: true, okText: '删除' })) {
              await store().deleteNotebook(uid);
              if (S.notebook === uid) S.notebook = null;
              await window.refreshAll();
              Utils.toast('已删除笔记本', 'success');
            }
          }
        } catch (e) { Utils.toast(e.message, 'error'); }
      });
    });
  };

  P.openTagMenu = function (tag) {
    Modal.drawer(`<div class="drawer-head"><h3>#${esc(tag.name)}</h3><button class="icon-btn" data-drawer-close>${icon('x', 16)}</button></div>
      <div class="drawer-body">
        <div class="menu-group"><div class="mg-title">标签</div>
          <button class="menu-item" data-tagact="filter">${icon('filter', 15)}<span>只看这个标签</span></button>
          <button class="menu-item" data-tagact="rename">${icon('edit', 15)}<span>重命名</span></button>
        </div>
        <div class="menu-sep"></div>
        <div class="menu-group"><div class="mg-title">危险操作</div>
          <button class="menu-item danger" data-tagact="delete">${icon('trash', 15)}<span>删除标签（笔记保留）</span></button>
        </div>
      </div>`, (root) => {
      root.querySelector('[data-drawer-close]').onclick = Modal.closeDrawer;
      root.querySelectorAll('[data-tagact]').forEach((b) => b.onclick = async () => {
        const act = b.getAttribute('data-tagact');
        try {
          if (act === 'filter') { Modal.closeDrawer(); window.selectTag(tag.name); }
          if (act === 'rename') {
            const n = await Dialogs.prompt({ title: '重命名标签', label: '名称', value: tag.name });
            if (!n) return;
            await store().updateTag(tag.uid, { name: n });
            Modal.closeDrawer(); await window.refreshAll();
          }
          if (act === 'delete') {
            Modal.closeDrawer();
            if (await Dialogs.confirm({ title: '删除标签', message: '标签会从所有笔记上移除，笔记本身不受影响。', danger: true, okText: '删除' })) {
              await store().deleteTag(tag.uid);
              if (S.tag === tag.name) S.tag = null;
              await window.refreshAll();
            }
          }
        } catch (e) { Utils.toast(e.message, 'error'); }
      });
    });
  };

  window.Panels = P;
})();
