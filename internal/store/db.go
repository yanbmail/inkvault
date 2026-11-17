package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database and the on-disk data directory.
// Everything in the vault lives under DataDir: the database, attachments,
// version snapshots metadata (inside the DB) and backups.
type DB struct {
	SQL      *sql.DB
	DataDir  string
	DBPath   string
	FilesDir string
	FTS      bool
}

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

CREATE TABLE IF NOT EXISTS settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS notebooks (
  uid         TEXT PRIMARY KEY,
  name        TEXT NOT NULL DEFAULT '',
  parent_uid  TEXT NOT NULL DEFAULT '',
  icon        TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  sort        INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  deleted_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_notebooks_parent ON notebooks(parent_uid);

CREATE TABLE IF NOT EXISTS notes (
  uid          TEXT PRIMARY KEY,
  title        TEXT NOT NULL DEFAULT '',
  content      TEXT NOT NULL DEFAULT '',
  excerpt      TEXT NOT NULL DEFAULT '',
  notebook_uid TEXT NOT NULL DEFAULT '',
  type         TEXT NOT NULL DEFAULT 'note',
  status       TEXT NOT NULL DEFAULT 'active',
  pinned       INTEGER NOT NULL DEFAULT 0,
  starred      INTEGER NOT NULL DEFAULT 0,
  word_count   INTEGER NOT NULL DEFAULT 0,
  source       TEXT NOT NULL DEFAULT '',
  source_url   TEXT NOT NULL DEFAULT '',
  author       TEXT NOT NULL DEFAULT '',
  color        TEXT NOT NULL DEFAULT '',
  extra        TEXT NOT NULL DEFAULT '{}',
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_notes_notebook ON notes(notebook_uid);
CREATE INDEX IF NOT EXISTS idx_notes_updated ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_notes_deleted ON notes(deleted_at);
CREATE INDEX IF NOT EXISTS idx_notes_status ON notes(status);

CREATE TABLE IF NOT EXISTS tags (
  uid        TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  color      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS note_tags (
  note_uid TEXT NOT NULL,
  tag_uid  TEXT NOT NULL,
  PRIMARY KEY (note_uid, tag_uid)
);
CREATE INDEX IF NOT EXISTS idx_note_tags_tag ON note_tags(tag_uid);

CREATE TABLE IF NOT EXISTS attachments (
  uid         TEXT PRIMARY KEY,
  note_uid    TEXT NOT NULL DEFAULT '',
  filename    TEXT NOT NULL DEFAULT '',
  stored_name TEXT NOT NULL DEFAULT '',
  mime        TEXT NOT NULL DEFAULT '',
  size        INTEGER NOT NULL DEFAULT 0,
  kind        TEXT NOT NULL DEFAULT 'file',
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attachments_note ON attachments(note_uid);

CREATE TABLE IF NOT EXISTS note_versions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  note_uid   TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  content    TEXT NOT NULL DEFAULT '',
  word_count INTEGER NOT NULL DEFAULT 0,
  reason     TEXT NOT NULL DEFAULT 'edit',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_versions_note ON note_versions(note_uid, id DESC);

CREATE TABLE IF NOT EXISTS fields (
  uid        TEXT PRIMARY KEY,
  name       TEXT NOT NULL DEFAULT '',
  key        TEXT NOT NULL DEFAULT '',
  type       TEXT NOT NULL DEFAULT 'text',
  options    TEXT NOT NULL DEFAULT '[]',
  scope      TEXT NOT NULL DEFAULT '',
  required   INTEGER NOT NULL DEFAULT 0,
  sort       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS field_values (
  note_uid  TEXT NOT NULL,
  field_uid TEXT NOT NULL,
  value     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (note_uid, field_uid)
);

CREATE TABLE IF NOT EXISTS templates (
  uid          TEXT PRIMARY KEY,
  name         TEXT NOT NULL DEFAULT '',
  content      TEXT NOT NULL DEFAULT '',
  notebook_uid TEXT NOT NULL DEFAULT '',
  icon         TEXT NOT NULL DEFAULT 'file-text',
  sort         INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS saved_searches (
  uid   TEXT PRIMARY KEY,
  name  TEXT NOT NULL DEFAULT '',
  query TEXT NOT NULL DEFAULT '',
  icon  TEXT NOT NULL DEFAULT 'search',
  sort  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS import_batches (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  filename   TEXT NOT NULL DEFAULT '',
  format     TEXT NOT NULL DEFAULT '',
  total      INTEGER NOT NULL DEFAULT 0,
  created    INTEGER NOT NULL DEFAULT 0,
  updated    INTEGER NOT NULL DEFAULT 0,
  skipped    INTEGER NOT NULL DEFAULT 0,
  failed     INTEGER NOT NULL DEFAULT 0,
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  action     TEXT NOT NULL DEFAULT '',
  target     TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);
`

const ftsSchema = `CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(uid UNINDEXED, title, content, tokenize='trigram');`

// Open prepares the data directory and opens the vault database.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	files := filepath.Join(dir, "files")
	if err := os.MkdirAll(files, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件目录失败: %w", err)
	}
	path := filepath.Join(dir, "vault.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if _, err := sqlDB.Exec(schema); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}
	db := &DB{SQL: sqlDB, DataDir: dir, DBPath: path, FilesDir: files, FTS: true}
	if _, err := sqlDB.Exec(ftsSchema); err != nil {
		// Older SQLite builds may lack FTS5/trigram; search falls back to LIKE.
		db.FTS = false
	}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.seed(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	db.refreshDerived()
	return db, nil
}

func (db *DB) Close() error { return db.SQL.Close() }

// refreshDerived recomputes word counts and summaries. It keeps older vaults
// consistent after the excerpt algorithm improves.
func (db *DB) refreshDerived() {
	rows, err := db.SQL.Query(`SELECT uid,content,excerpt,word_count FROM notes`)
	if err != nil {
		return
	}
	type row struct {
		uid, content, excerpt string
		words                 int
	}
	list := []row{}
	for rows.Next() {
		var r row
		if rows.Scan(&r.uid, &r.content, &r.excerpt, &r.words) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	for _, r := range list {
		w := CountWords(r.content)
		e := Excerpt(r.content, 180)
		if w != r.words || e != r.excerpt {
			_, _ = db.SQL.Exec(`UPDATE notes SET word_count=?, excerpt=? WHERE uid=?`, w, e, r.uid)
		}
	}
}

// migrate applies additive changes for vaults created by older builds.
func (db *DB) migrate() error {
	cols := map[string]string{
		"notes":     "ALTER TABLE notes ADD COLUMN extra TEXT NOT NULL DEFAULT '{}'",
		"notebooks": "ALTER TABLE notebooks ADD COLUMN description TEXT NOT NULL DEFAULT ''",
	}
	for table, stmt := range cols {
		if !db.hasColumn(table, lastFieldOf(stmt)) {
			if _, err := db.SQL.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	_, _ = db.SQL.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(uid UNINDEXED, title, content, tokenize='trigram');`)
	return nil
}

func lastFieldOf(stmt string) string {
	// "ALTER TABLE x ADD COLUMN <name> ..." -> <name>
	parts := strings.Fields(stmt)
	for i, p := range parts {
		if p == "COLUMN" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (db *DB) hasColumn(table, name string) bool {
	rows, err := db.SQL.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var col, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notnull, &dflt, &pk); err == nil && col == name {
			return true
		}
	}
	return false
}

// seed creates the default notebook, starter tags and a welcome note on a
// brand-new vault. Existing vaults are never touched.
func (db *DB) seed() error {
	var n int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM notebooks`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	now := Now()
	if _, err := db.SQL.Exec(`INSERT INTO notebooks(uid,name,parent_uid,icon,color,description,sort,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, "nb-inbox", "收集箱", "", "inbox", "#b4462f", "来不及整理的灵感先放这里", 0, now, now); err != nil {
		return err
	}
	// A brand-new vault gets three short guides instead of an empty screen.
	guides := []struct {
		title   string
		content string
		tags    []string
	}{
		{"欢迎使用墨库", `# 欢迎使用墨库

这是你的本地知识库。所有内容都保存在这台电脑上，**没有云端账号，也没有订阅**。

## 三个入口

- 左侧「全部笔记」：所有内容
- 左下「备份与恢复」：随时把整个库打包带走
- 顶部搜索（⌘K）：找人、找笔记、执行命令

## 常用快捷键

| 快捷键 | 动作 |
| --- | --- |
| ⌘K | 搜索 / 命令面板 |
| ⌘N | 新建笔记 |
| ⌘S | 立即保存 |
| ⌘E | 导出目录 |

## 数据在哪

macOS 在 ~/InkVault，Windows 在程序旁边的 data 文件夹。
数据库是标准 SQLite 文件，附件是普通文件，随时可以复制走。

> 先写下第一条笔记吧。写什么都可以，重要的是开始。
`, []string{"使用指南"}},
		{"把旧笔记搬进来", `# 把旧笔记搬进来

点左侧的导入图标，或者按 ⌘K 输入「导入」。

## 支持哪些来源

- **印象笔记 / Evernote**：导出 .enex，标签、附件、时间都会保留
- **锤子便签**：网页版导出 Markdown 压缩包
- **Apple 备忘录**：导出 HTML 文件夹
- **Obsidian / Logseq**：整个 vault 压缩成 zip
- **Notion**：导出 Markdown & CSV 压缩包
- **Joplin**：导出 .jex
- **通用**：CSV / JSON / TXT / Markdown

## 导入规则

只增不改。导入永远创建新笔记，不会覆盖你现有的内容。

文件夹层级会变成笔记本，图片会进入附件，标签会保留。
`, []string{"使用指南", "迁移"}},
		{"目录、统计与备份", `# 目录、统计与备份

墨库最值得先试的三件事。

## 1. 导出目录

按 ⌘E，把当前范围（全部 / 某个笔记本 / 搜索结果）导出成：

- Markdown：放进 Obsidian 或博客
- 网页：带封面、分组、可点击的阅读版
- Excel：可筛选、可透视
- Word：正式文档
- CSV / JSON：数据交换

可以选分组方式、简介长度、是否显示标签与字数。

## 2. 看统计

左侧图表按钮：笔记数、总字数、连续记录、月度趋势、记录热度。
右上角可以「生成统计笔记」，把数据写进知识库；也可以导出 HTML / Excel 报告。

## 3. 备份

左下「备份与恢复」→ 立即备份。
备份包含数据库、全部附件和版本历史，恢复前会自动再备份一次当前数据。

- [ ] 创建第一次备份
- [ ] 导入一份旧笔记
- [ ] 导出一份目录

`, []string{"使用指南", "备份"}},
	}
	for _, g := range guides {
		if _, err := db.CreateNote(&Note{Title: g.title, Content: g.content, Tags: g.tags, NotebookUID: "nb-inbox", Type: "note"}); err != nil {
			return err
		}
	}
	return nil
}

/* ---------- settings ---------- */

func (db *DB) GetSetting(key, def string) string {
	var v string
	err := db.SQL.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

func (db *DB) SetSetting(key, value string) error {
	_, err := db.SQL.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (db *DB) AllSettings() map[string]string {
	out := map[string]string{}
	rows, err := db.SQL.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func (db *DB) Audit(action, target, detail, ip string) {
	_, _ = db.SQL.Exec(`INSERT INTO audit_log(action,target,detail,ip,created_at) VALUES(?,?,?,?,?)`,
		action, target, detail, ip, Now())
}

/* ---------- helpers ---------- */

const timeLayout = "2006-01-02T15:04:05.000Z"

// Now returns a lexicographically sortable UTC timestamp with milliseconds.
func Now() string { return time.Now().UTC().Format(timeLayout) }

// NormalizeTime converts common timestamp shapes into the canonical layout.
func NormalizeTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return Now()
	}
	layouts := []string{
		time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05",
		"2006-01-02T15:04", "2006-01-02", "20060102T150405Z", "20060102T150405",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format(timeLayout)
		}
	}
	if n, err := parseInt64(s); err == nil && n > 0 {
		// accept seconds or milliseconds since epoch
		if n > 1e11 {
			return time.UnixMilli(n).UTC().Format(timeLayout)
		}
		return time.Unix(n, 0).UTC().Format(timeLayout)
	}
	return Now()
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}

// FormatLocal renders a canonical timestamp for humans (local timezone).
func FormatLocal(s string) string {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2.Local().Format("2006-01-02 15:04")
		}
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}

// DayKey returns the local calendar date for a canonical timestamp.
func DayKey(s string) string {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return ""
	}
	return t.Local().Format("2006-01-02")
}
