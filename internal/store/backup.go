package store

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
	Reason    string `json:"reason"`
	Notes     int    `json:"notes"`
}

type backupManifest struct {
	App       string `json:"app"`
	Version   string `json:"version"`
	CreatedAt string `json:"createdAt"`
	Host      string `json:"host"`
	Notes     int    `json:"notes"`
	Words     int    `json:"words"`
	Notebooks int    `json:"notebooks"`
	Tags      int    `json:"tags"`
	Files     int    `json:"files"`
	Reason    string `json:"reason"`
}

func (db *DB) BackupsDir() string { return filepath.Join(db.DataDir, "backups") }

// CreateBackup writes a consistent snapshot (db + attachments + manifest) as a
// zip archive. VACUUM INTO gives us a coherent copy even while the app runs.
func (db *DB) CreateBackup(reason string) (*BackupInfo, error) {
	dir := db.BackupsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stamp := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("inkvault-%s-%s.zip", stamp, sanitizeReason(reason))
	dest := filepath.Join(dir, name)
	tmpDB := filepath.Join(dir, ".snapshot-"+stamp+".db")
	_ = os.Remove(tmpDB)
	if _, err := db.SQL.Exec(`VACUUM INTO ?`, tmpDB); err != nil {
		return nil, fmt.Errorf("数据库快照失败: %w", err)
	}
	defer os.Remove(tmpDB)

	f, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)

	add := func(name string, r io.Reader, size int64) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, r)
		return err
	}

	// database
	dbf, err := os.Open(tmpDB)
	if err != nil {
		f.Close()
		return nil, err
	}
	info, _ := dbf.Stat()
	if err := add("vault.db", dbf, info.Size()); err != nil {
		dbf.Close()
		f.Close()
		return nil, err
	}
	dbf.Close()

	// attachments
	files := 0
	_ = filepath.Walk(db.FilesDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(db.FilesDir, path)
		if err != nil {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer src.Close()
		files++
		return add("files/"+filepath.ToSlash(rel), src, fi.Size())
	})

	st := &Stats{}
	_ = db.SQL.QueryRow(`SELECT COUNT(*), COALESCE(SUM(word_count),0) FROM notes WHERE deleted_at IS NULL`).Scan(&st.TotalNotes, &st.TotalWords)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notebooks WHERE deleted_at IS NULL`).Scan(&st.NotebookCount)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&st.TagCount)
	host, _ := os.Hostname()
	man := backupManifest{
		App: "InkVault", Version: "1.0.0", CreatedAt: Now(), Host: host,
		Notes: st.TotalNotes, Words: st.TotalWords, Notebooks: st.NotebookCount, Tags: st.TagCount,
		Files: files, Reason: reason,
	}
	raw, _ := json.MarshalIndent(man, "", "  ")
	if _, err := zw.Create("manifest.json"); err == nil {
		w, _ := zw.Create("manifest.json")
		_, _ = w.Write(raw)
	}
	if w, err := zw.Create("说明.txt"); err == nil {
		_, _ = w.Write([]byte("墨库 InkVault 备份\n\n" +
			"vault.db       —— SQLite 数据库（笔记、笔记本、标签、版本历史）\n" +
			"files/         —— 附件与图片原文件\n" +
			"manifest.json  —— 备份元信息\n\n" +
			"恢复方式：墨库「设置 → 备份与恢复 → 从备份恢复」，选择本 zip 文件即可。\n" +
			"数据完全属于你，可长期离线保存。\n"))
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	fi, _ := os.Stat(dest)
	return &BackupInfo{Name: name, Size: fi.Size(), CreatedAt: Now(), Reason: reason, Notes: st.TotalNotes}, nil
}

func sanitizeReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "manual"
	}
	s = reIllegalWin.ReplaceAllString(s, "-")
	if len([]rune(s)) > 20 {
		s = string([]rune(s)[:20])
	}
	return s
}

func (db *DB) ListBackups() []BackupInfo {
	dir := db.BackupsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []BackupInfo{}
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		reason := "manual"
		parts := strings.Split(strings.TrimSuffix(e.Name(), ".zip"), "-")
		if len(parts) >= 4 {
			reason = parts[len(parts)-1]
		}
		out = append(out, BackupInfo{Name: e.Name(), Size: fi.Size(), CreatedAt: fi.ModTime().UTC().Format(timeLayout), Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (db *DB) DeleteBackup(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return fmt.Errorf("非法文件名")
	}
	return os.Remove(filepath.Join(db.BackupsDir(), name))
}

// StageBackup extracts a backup archive into a staging directory and validates
// the database inside. The caller swaps it in after creating a safety backup.
func StageBackup(zipPath, dataDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("无法读取备份文件: %w", err)
	}
	defer zr.Close()
	stage, err := os.MkdirTemp(dataDir, ".restore-")
	if err != nil {
		return "", err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	hasDB := false
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			cleanup()
			return "", fmt.Errorf("备份文件包含非法路径")
		}
		if name == "vault.db" || name == "vault.db-wal" || name == "vault.db-shm" {
			hasDB = true
		}
		dest := filepath.Join(stage, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dest, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			cleanup()
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			cleanup()
			return "", err
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			cleanup()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			cleanup()
			return "", err
		}
		out.Close()
		rc.Close()
	}
	if !hasDB {
		cleanup()
		return "", fmt.Errorf("备份文件里没有 vault.db")
	}
	// validate database
	testDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(stage, "vault.db"))+"?mode=ro")
	if err != nil {
		cleanup()
		return "", fmt.Errorf("备份数据库无法打开: %w", err)
	}
	var cnt int
	if err := testDB.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&cnt); err != nil {
		testDB.Close()
		cleanup()
		return "", fmt.Errorf("备份数据库校验失败: %w", err)
	}
	testDB.Close()
	if _, err := os.Stat(filepath.Join(stage, "files")); err != nil {
		_ = os.MkdirAll(filepath.Join(stage, "files"), 0o755)
	}
	return stage, nil
}

// SwapVault replaces the live database and attachments with a staged restore,
// keeping the previous data as .pre-restore backups.
func SwapVault(stage, dataDir string) error {
	stamp := time.Now().Format("20060102-150405")
	// move old aside
	for _, name := range []string{"vault.db", "vault.db-wal", "vault.db-shm"} {
		src := filepath.Join(dataDir, name)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, filepath.Join(dataDir, name+".pre-restore-"+stamp))
		}
	}
	oldFiles := filepath.Join(dataDir, "files")
	if _, err := os.Stat(oldFiles); err == nil {
		_ = os.Rename(oldFiles, filepath.Join(dataDir, "files.pre-restore-"+stamp))
	}
	if err := os.Rename(filepath.Join(stage, "vault.db"), filepath.Join(dataDir, "vault.db")); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(stage, "files"), filepath.Join(dataDir, "files")); err != nil {
		_ = os.MkdirAll(filepath.Join(dataDir, "files"), 0o755)
	}
	_ = os.RemoveAll(stage)
	return nil
}

// LatestBackupAge reports how old the newest backup is (for the auto-backup loop).
func (db *DB) LatestBackupAge() time.Duration {
	list := db.ListBackups()
	if len(list) == 0 {
		return time.Duration(1<<62)
	}
	t, err := time.Parse(timeLayout, list[0].CreatedAt)
	if err != nil {
		return 0
	}
	return time.Since(t)
}

func (db *DB) PruneBackups(keep int) {
	if keep <= 0 {
		keep = 14
	}
	list := db.ListBackups()
	for i, b := range list {
		if i >= keep {
			_ = db.DeleteBackup(b.Name)
		}
	}
}

/* ------------------------------ vault dump ------------------------------- */

// VaultDump is the JSON representation of the whole vault, used for full
// exports and for merging a mobile device back into the desktop vault.
type VaultDump struct {
	App           string         `json:"app"`
	Version       string         `json:"version"`
	ExportedAt    string         `json:"exportedAt"`
	VaultName     string         `json:"vaultName"`
	Notebooks     []*Notebook    `json:"notebooks"`
	Tags          []Tag          `json:"tags"`
	Notes         []*Note        `json:"notes"`
	Fields        []Field        `json:"fields"`
	Templates     []Template     `json:"templates"`
	SavedSearches []SavedSearch  `json:"savedSearches"`
}

// DumpVault exports everything (including trash) for backup or sync.
func (db *DB) DumpVault(includeTrash bool, withContent bool) (*VaultDump, error) {
	d := &VaultDump{App: "InkVault", Version: "1.0.0", ExportedAt: Now(), VaultName: db.GetSetting("vault_name", "我的知识库")}

	nbs, _ := db.AllNotebooks()
	d.Notebooks = nbs
	d.Tags = db.TagCloud()
	if d.Tags == nil {
		d.Tags = []Tag{}
	}
	d.Fields = db.Fields()
	d.Templates = db.Templates()
	if d.Templates == nil {
		d.Templates = []Template{}
	}
	d.SavedSearches = db.SavedSearches()

	cols := "uid,title,content,excerpt,notebook_uid,type,status,pinned,starred,word_count,source,source_url,author,color,extra,created_at,updated_at,deleted_at"
	if !withContent {
		cols = "uid,title,'',excerpt,notebook_uid,type,status,pinned,starred,word_count,source,source_url,author,color,extra,created_at,updated_at,deleted_at"
	}
	q := `SELECT ` + cols + ` FROM notes`
	if !includeTrash {
		q += ` WHERE deleted_at IS NULL`
	}
	q += ` ORDER BY created_at`
	rows, err := db.SQL.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := db.notebookNames()
	for rows.Next() {
		n := &Note{}
		var extra string
		var deleted sql.NullString
		if err := rows.Scan(&n.UID, &n.Title, &n.Content, &n.Excerpt, &n.NotebookUID, &n.Type, &n.Status, &n.Pinned, &n.Starred,
			&n.WordCount, &n.Source, &n.SourceURL, &n.Author, &n.Color, &extra, &n.CreatedAt, &n.UpdatedAt, &deleted); err != nil {
			return nil, err
		}
		if deleted.Valid {
			n.DeletedAt = deleted.String
		}
		n.Extra = map[string]string{}
		_ = json.Unmarshal([]byte(extra), &n.Extra)
		n.NotebookName = names[n.NotebookUID]
		n.Tags = db.noteTags(n.UID)
		if n.Tags == nil {
			n.Tags = []string{}
		}
		d.Notes = append(d.Notes, n)
	}
	return d, nil
}

// ApplyDump merges notes from a device. Newer updated_at wins; deletions are
// propagated via tombstones. Returns (created, updated, skipped).
func (db *DB) ApplyDump(d *VaultDump) (int, int, int, error) {
	created, updated, skipped := 0, 0, 0
	// notebooks first
	for _, nb := range d.Notebooks {
		if nb.UID == "" {
			continue
		}
		var exists int
		_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notebooks WHERE uid=?`, nb.UID).Scan(&exists)
		if exists == 0 {
			_, err := db.SQL.Exec(`INSERT INTO notebooks(uid,name,parent_uid,icon,color,description,sort,created_at,updated_at,deleted_at)
				VALUES(?,?,?,?,?,?,?,?,?,NULL)`, nb.UID, nb.Name, nb.ParentUID, nb.Icon, nb.Color, nb.Description, nb.Sort, NormalizeTime(nb.CreatedAt), NormalizeTime(nb.UpdatedAt))
			if err != nil {
				return created, updated, skipped, err
			}
		}
	}
	// tags
	for _, t := range d.Tags {
		if t.Name == "" {
			continue
		}
		var uid string
		if err := db.SQL.QueryRow(`SELECT uid FROM tags WHERE name=?`, t.Name).Scan(&uid); err != nil {
			_, _ = db.SQL.Exec(`INSERT INTO tags(uid,name,color,created_at) VALUES(?,?,?,?)`, firstNonEmpty(t.UID, NewUID("tg-")), t.Name, t.Color, Now())
		}
	}
	for _, n := range d.Notes {
		if n.UID == "" {
			n.UID = NewUID("nt-")
		}
		var updatedAt string
		err := db.SQL.QueryRow(`SELECT updated_at FROM notes WHERE uid=?`, n.UID).Scan(&updatedAt)
		if err == sql.ErrNoRows {
			nbUID := n.NotebookUID
			if nbUID == "" {
				nbUID = db.DefaultNotebook()
			}
			extra, _ := json.Marshal(n.Extra)
			_, err = db.SQL.Exec(`INSERT INTO notes(uid,title,content,excerpt,notebook_uid,type,status,pinned,starred,word_count,
				source,source_url,author,color,extra,created_at,updated_at,deleted_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				n.UID, n.Title, n.Content, firstNonEmpty(n.Excerpt, Excerpt(n.Content, 180)), nbUID, firstNonEmpty(n.Type, "note"),
				firstNonEmpty(n.Status, "active"), n.Pinned, n.Starred, CountWords(n.Content), n.Source, n.SourceURL, n.Author, n.Color,
				string(extra), NormalizeTime(n.CreatedAt), NormalizeTime(n.UpdatedAt), nullIfEmpty(n.DeletedAt))
			if err != nil {
				skipped++
				continue
			}
			if len(n.Tags) > 0 {
				_ = db.SetNoteTags(n.UID, n.Tags)
			}
			db.ftsUpsert(n.UID, n.Title, n.Content)
			created++
			continue
		}
		if err != nil {
			skipped++
			continue
		}
		if NormalizeTime(n.UpdatedAt) <= updatedAt {
			skipped++
			continue
		}
		extra, _ := json.Marshal(n.Extra)
		_, err = db.SQL.Exec(`UPDATE notes SET title=?,content=?,excerpt=?,notebook_uid=?,type=?,status=?,pinned=?,starred=?,
			word_count=?,source=?,source_url=?,author=?,color=?,extra=?,updated_at=?,deleted_at=? WHERE uid=?`,
			n.Title, n.Content, firstNonEmpty(n.Excerpt, Excerpt(n.Content, 180)), n.NotebookUID, firstNonEmpty(n.Type, "note"),
			firstNonEmpty(n.Status, "active"), n.Pinned, n.Starred, CountWords(n.Content), n.Source, n.SourceURL, n.Author, n.Color,
			string(extra), NormalizeTime(n.UpdatedAt), nullIfEmpty(n.DeletedAt), n.UID)
		if err != nil {
			skipped++
			continue
		}
		if n.Tags != nil {
			_ = db.SetNoteTags(n.UID, n.Tags)
		}
		db.ftsUpsert(n.UID, n.Title, n.Content)
		updated++
	}
	return created, updated, skipped, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return NormalizeTime(s)
}
