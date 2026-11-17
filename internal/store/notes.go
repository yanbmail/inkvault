package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Notebook struct {
	UID         string      `json:"uid"`
	Name        string      `json:"name"`
	ParentUID   string      `json:"parentUid"`
	Icon        string      `json:"icon"`
	Color       string      `json:"color"`
	Description string      `json:"description"`
	Sort        int         `json:"sort"`
	CreatedAt   string      `json:"createdAt"`
	UpdatedAt   string      `json:"updatedAt"`
	Count       int         `json:"count"`
	Total       int         `json:"total"`
	Children    []*Notebook `json:"children,omitempty"`
}

type Tag struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Count     int    `json:"count"`
	CreatedAt string `json:"createdAt"`
}

type Attachment struct {
	UID        string `json:"uid"`
	NoteUID    string `json:"noteUid"`
	Filename   string `json:"filename"`
	StoredName string `json:"-"`
	Mime       string `json:"mime"`
	Size       int64  `json:"size"`
	Kind       string `json:"kind"`
	CreatedAt  string `json:"createdAt"`
	URL        string `json:"url"`
}

type Note struct {
	UID          string            `json:"uid"`
	Title        string            `json:"title"`
	Content      string            `json:"content,omitempty"`
	Excerpt      string            `json:"excerpt"`
	NotebookUID  string            `json:"notebookUid"`
	NotebookName string            `json:"notebookName,omitempty"`
	Type         string            `json:"type"`
	Status       string            `json:"status"`
	Pinned       bool              `json:"pinned"`
	Starred      bool              `json:"starred"`
	WordCount    int               `json:"wordCount"`
	Source       string            `json:"source"`
	SourceURL    string            `json:"sourceUrl"`
	Author       string            `json:"author"`
	Color        string            `json:"color"`
	Extra        map[string]string `json:"extra,omitempty"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
	DeletedAt    string            `json:"deletedAt,omitempty"`
	Tags         []string          `json:"tags"`
	Attachments  []Attachment      `json:"attachments,omitempty"`
	Fields       map[string]string `json:"fields,omitempty"`
	VersionCount int               `json:"versionCount,omitempty"`
}

type NoteFilter struct {
	Query       string
	NotebookUID string
	Tag         string
	Type        string
	Status      string
	Starred     bool
	Pinned      bool
	Trashed     bool
	Archived    bool
	From        string
	To          string
	Sort        string
	Limit       int
	Offset      int
	IncludeDesc bool
}

type Field struct {
	UID      string   `json:"uid"`
	Name     string   `json:"name"`
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Scope    string   `json:"scope"`
	Required bool     `json:"required"`
	Sort     int      `json:"sort"`
}

type Template struct {
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	NotebookUID string `json:"notebookUid"`
	Icon        string `json:"icon"`
	Sort        int    `json:"sort"`
}

type SavedSearch struct {
	UID   string `json:"uid"`
	Name  string `json:"name"`
	Query string `json:"query"`
	Icon  string `json:"icon"`
	Sort  int    `json:"sort"`
}

/* ------------------------------- notebooks ------------------------------- */

func (db *DB) Notebooks() ([]*Notebook, error) {
	rows, err := db.SQL.Query(`SELECT uid,name,parent_uid,icon,color,description,sort,created_at,updated_at
		FROM notebooks WHERE deleted_at IS NULL ORDER BY sort, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []*Notebook{}
	byUID := map[string]*Notebook{}
	for rows.Next() {
		n := &Notebook{}
		if err := rows.Scan(&n.UID, &n.Name, &n.ParentUID, &n.Icon, &n.Color, &n.Description, &n.Sort, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
		byUID[n.UID] = n
	}
	// direct counts
	rows2, err := db.SQL.Query(`SELECT notebook_uid, COUNT(*) FROM notes WHERE deleted_at IS NULL GROUP BY notebook_uid`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var uid string
			var c int
			if rows2.Scan(&uid, &c) == nil {
				if nb := byUID[uid]; nb != nil {
					nb.Count = c
				}
			}
		}
	}
	roots := []*Notebook{}
	for _, n := range list {
		if n.ParentUID != "" {
			if p := byUID[n.ParentUID]; p != nil {
				p.Children = append(p.Children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	var walk func(n *Notebook) int
	walk = func(n *Notebook) int {
		total := n.Count
		for _, c := range n.Children {
			total += walk(c)
		}
		n.Total = total
		return total
	}
	for _, r := range roots {
		walk(r)
	}
	return roots, nil
}

func (db *DB) AllNotebooks() ([]*Notebook, error) {
	rows, err := db.SQL.Query(`SELECT uid,name,parent_uid FROM notebooks WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Notebook{}
	for rows.Next() {
		n := &Notebook{}
		if err := rows.Scan(&n.UID, &n.Name, &n.ParentUID); err == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

func (db *DB) NotebookPath(uid string) string {
	all, err := db.AllNotebooks()
	if err != nil {
		return ""
	}
	byUID := map[string]*Notebook{}
	for _, n := range all {
		byUID[n.UID] = n
	}
	parts := []string{}
	seen := map[string]bool{}
	for cur := byUID[uid]; cur != nil && !seen[cur.UID]; cur = byUID[cur.ParentUID] {
		seen[cur.UID] = true
		parts = append([]string{cur.Name}, parts...)
	}
	return strings.Join(parts, " / ")
}

func (db *DB) CreateNotebook(n *Notebook) (*Notebook, error) {
	if n.UID == "" {
		n.UID = NewUID("nb-")
	}
	if strings.TrimSpace(n.Name) == "" {
		n.Name = "未命名笔记本"
	}
	now := Now()
	n.CreatedAt, n.UpdatedAt = now, now
	_, err := db.SQL.Exec(`INSERT INTO notebooks(uid,name,parent_uid,icon,color,description,sort,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, n.UID, n.Name, n.ParentUID, n.Icon, n.Color, n.Description, n.Sort, n.CreatedAt, n.UpdatedAt)
	return n, err
}

func (db *DB) UpdateNotebook(n *Notebook) error {
	n.UpdatedAt = Now()
	_, err := db.SQL.Exec(`UPDATE notebooks SET name=?,parent_uid=?,icon=?,color=?,description=?,sort=?,updated_at=? WHERE uid=?`,
		n.Name, n.ParentUID, n.Icon, n.Color, n.Description, n.Sort, n.UpdatedAt, n.UID)
	return err
}

// DeleteNotebook removes a notebook but never the notes inside it: notes and
// child notebooks move up to the parent (or the inbox).
func (db *DB) DeleteNotebook(uid string) error {
	var parent string
	_ = db.SQL.QueryRow(`SELECT parent_uid FROM notebooks WHERE uid=?`, uid).Scan(&parent)
	if parent == "" {
		parent = db.DefaultNotebook()
	}
	now := Now()
	if _, err := db.SQL.Exec(`UPDATE notes SET notebook_uid=?, updated_at=? WHERE notebook_uid=?`, parent, now, uid); err != nil {
		return err
	}
	if _, err := db.SQL.Exec(`UPDATE notebooks SET parent_uid=? WHERE parent_uid=?`, parent, uid); err != nil {
		return err
	}
	_, err := db.SQL.Exec(`UPDATE notebooks SET deleted_at=?, updated_at=? WHERE uid=?`, now, now, uid)
	return err
}

func (db *DB) DefaultNotebook() string {
	var uid string
	err := db.SQL.QueryRow(`SELECT uid FROM notebooks WHERE deleted_at IS NULL ORDER BY sort, created_at LIMIT 1`).Scan(&uid)
	if err != nil || uid == "" {
		return "nb-inbox"
	}
	return uid
}

/* ---------------------------------- tags --------------------------------- */

func (db *DB) Tags() ([]Tag, error) {
	rows, err := db.SQL.Query(`SELECT t.uid,t.name,t.color,t.created_at,
		(SELECT COUNT(*) FROM note_tags nt JOIN notes n ON n.uid=nt.note_uid WHERE nt.tag_uid=t.uid AND n.deleted_at IS NULL)
		FROM tags t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.UID, &t.Name, &t.Color, &t.CreatedAt, &t.Count); err == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (db *DB) EnsureTag(name string) (string, error) {
	name = NormalizeTag(name)
	if name == "" {
		return "", fmt.Errorf("标签名为空")
	}
	var uid string
	err := db.SQL.QueryRow(`SELECT uid FROM tags WHERE name=?`, name).Scan(&uid)
	if err == nil {
		return uid, nil
	}
	uid = NewUID("tg-")
	_, err = db.SQL.Exec(`INSERT INTO tags(uid,name,color,created_at) VALUES(?,?,?,?)`, uid, name, "", Now())
	return uid, err
}

func (db *DB) SetNoteTags(noteUID string, names []string) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM note_tags WHERE note_uid=?`, noteUID); err != nil {
		return err
	}
	for _, name := range names {
		name = NormalizeTag(name)
		if name == "" {
			continue
		}
		var uid string
		if err := tx.QueryRow(`SELECT uid FROM tags WHERE name=?`, name).Scan(&uid); err != nil {
			uid = NewUID("tg-")
			if _, err := tx.Exec(`INSERT INTO tags(uid,name,color,created_at) VALUES(?,?,?,?)`, uid, name, "", Now()); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO note_tags(note_uid,tag_uid) VALUES(?,?)`, noteUID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) RenameTag(uid, name string) error {
	_, err := db.SQL.Exec(`UPDATE tags SET name=? WHERE uid=?`, NormalizeTag(name), uid)
	return err
}

func (db *DB) DeleteTag(uid string) error {
	if _, err := db.SQL.Exec(`DELETE FROM note_tags WHERE tag_uid=?`, uid); err != nil {
		return err
	}
	_, err := db.SQL.Exec(`DELETE FROM tags WHERE uid=?`, uid)
	return err
}

func (db *DB) SetTagColor(uid, color string) error {
	_, err := db.SQL.Exec(`UPDATE tags SET color=? WHERE uid=?`, color, uid)
	return err
}

/* ---------------------------------- notes -------------------------------- */

func (db *DB) noteTags(uid string) []string {
	out := []string{}
	rows, err := db.SQL.Query(`SELECT t.name FROM tags t JOIN note_tags nt ON nt.tag_uid=t.uid WHERE nt.note_uid=? ORDER BY t.name`, uid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

func (db *DB) notebookNames() map[string]string {
	m := map[string]string{}
	rows, err := db.SQL.Query(`SELECT uid,name FROM notebooks`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			m[k] = v
		}
	}
	return m
}

func (db *DB) notesTagsBulk(uids []string) map[string][]string {
	out := map[string][]string{}
	if len(uids) == 0 {
		return out
	}
	ph := strings.Repeat("?,", len(uids))
	ph = ph[:len(ph)-1]
	args := make([]any, len(uids))
	for i, u := range uids {
		args[i] = u
	}
	rows, err := db.SQL.Query(`SELECT nt.note_uid, t.name FROM note_tags nt JOIN tags t ON t.uid=nt.tag_uid
		WHERE nt.note_uid IN (`+ph+`) ORDER BY t.name`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var uid, name string
		if rows.Scan(&uid, &name) == nil {
			out[uid] = append(out[uid], name)
		}
	}
	return out
}

// parseQuery extracts tag:/notebook:/is: operators from the free-text query.
func parseQuery(q string) (text string, tags []string, notebook string, starred, pinned, trashed bool, from, to string) {
	fields := strings.Fields(q)
	var rest []string
	for _, f := range fields {
		lower := strings.ToLower(f)
		switch {
		case strings.HasPrefix(lower, "tag:"):
			tags = append(tags, NormalizeTag(f[4:]))
		case strings.HasPrefix(lower, "#") && len(f) > 1:
			tags = append(tags, NormalizeTag(f[1:]))
		case strings.HasPrefix(lower, "notebook:"):
			notebook = f[9:]
		case lower == "is:starred" || lower == "starred:true":
			starred = true
		case lower == "is:pinned" || lower == "pinned:true":
			pinned = true
		case lower == "is:trashed" || lower == "in:trash":
			trashed = true
		case strings.HasPrefix(lower, "before:"):
			to = f[7:]
		case strings.HasPrefix(lower, "after:"):
			from = f[6:]
		default:
			rest = append(rest, f)
		}
	}
	return strings.Join(rest, " "), tags, notebook, starred, pinned, trashed, from, to
}

func (db *DB) descendantNotebooks(uid string) []string {
	all, err := db.AllNotebooks()
	if err != nil {
		return []string{uid}
	}
	children := map[string][]string{}
	for _, n := range all {
		children[n.ParentUID] = append(children[n.ParentUID], n.UID)
	}
	out := []string{}
	stack := []string{uid}
	seen := map[string]bool{}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		out = append(out, cur)
		stack = append(stack, children[cur]...)
	}
	return out
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// Notes returns a filtered, sorted page of notes plus the total match count.
func (db *DB) Notes(f NoteFilter) ([]*Note, int, error) {
	text, qt, nbName, qStar, qPin, qTrash, from, to := parseQuery(f.Query)
	where := []string{}
	args := []any{}

	if f.Trashed || qTrash {
		where = append(where, "n.deleted_at IS NOT NULL")
	} else {
		where = append(where, "n.deleted_at IS NULL")
	}
	if f.Archived {
		where = append(where, "n.status='archived'")
	} else if !f.Trashed && !qTrash {
		where = append(where, "n.status<>'archived'")
	}
	if f.Status != "" {
		where = append(where, "n.status=?")
		args = append(args, f.Status)
	}
	if f.Type != "" {
		where = append(where, "n.type=?")
		args = append(args, f.Type)
	}
	if f.Starred || qStar {
		where = append(where, "n.starred=1")
	}
	if f.Pinned || qPin {
		where = append(where, "n.pinned=1")
	}
	if uid := firstNonEmpty(f.NotebookUID, qNotebookUID(db, nbName)); uid != "" {
		if f.IncludeDesc || f.NotebookUID == "" {
			ids := db.descendantNotebooks(uid)
			ph := strings.Repeat("?,", len(ids))
			ph = ph[:len(ph)-1]
			where = append(where, "n.notebook_uid IN ("+ph+")")
			for _, id := range ids {
				args = append(args, id)
			}
		} else {
			where = append(where, "n.notebook_uid=?")
			args = append(args, uid)
		}
	}
	tagFilters := qt
	if f.Tag != "" {
		tagFilters = append(tagFilters, f.Tag)
	}
	for _, tag := range tagFilters {
		where = append(where, `n.uid IN (SELECT nt.note_uid FROM note_tags nt JOIN tags t ON t.uid=nt.tag_uid WHERE t.name=?)`)
		args = append(args, tag)
	}
	startDate := firstNonEmpty(f.From, from)
	endDate := firstNonEmpty(f.To, to)
	if startDate != "" {
		where = append(where, "n.updated_at>=?")
		args = append(args, NormalizeTime(startDate))
	}
	if endDate != "" {
		where = append(where, "n.updated_at<=?")
		args = append(args, NormalizeTime(endDate)+"~")
	}
	if text != "" {
		terms := strings.Fields(text)
		allLong := db.FTS && len(terms) > 0
		for _, t := range terms {
			if utf8.RuneCountInString(t) < 3 {
				allLong = false
				break
			}
		}
		if allLong {
			where = append(where, `n.uid IN (SELECT uid FROM notes_fts WHERE notes_fts MATCH ?)`)
			args = append(args, ftsQuery(text))
		} else {
			for _, t := range terms {
				like := "%" + escapeLike(t) + "%"
				where = append(where, `(n.title LIKE ? ESCAPE '\' OR n.content LIKE ? ESCAPE '\')`)
				args = append(args, like, like)
			}
		}
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM notes n `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "n.pinned DESC, n.updated_at DESC"
	scoreArgs := []any{}
	if text != "" && (f.Sort == "" || f.Sort == "relevance") {
		terms := strings.Fields(text)
		parts := make([]string, 0, len(terms))
		for _, t := range terms {
			parts = append(parts, "((LENGTH(n.title)-LENGTH(REPLACE(n.title, ?, '')))*4+(LENGTH(n.content)-LENGTH(REPLACE(n.content, ?, ''))))")
			scoreArgs = append(scoreArgs, t, t)
		}
		if len(parts) > 0 {
			order = "n.pinned DESC, (" + strings.Join(parts, " + ") + ") DESC, n.updated_at DESC"
		}
	}
	switch f.Sort {
	case "updated_asc":
		order = "n.updated_at ASC"
	case "created_desc":
		order = "n.created_at DESC"
	case "created_asc":
		order = "n.created_at ASC"
	case "title":
		order = "n.title COLLATE NOCASE ASC"
	case "words":
		order = "n.word_count DESC"
	case "oldest":
		order = "n.updated_at ASC"
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 60
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q := `SELECT n.uid,n.title,n.excerpt,n.notebook_uid,n.type,n.status,n.pinned,n.starred,n.word_count,
		n.source,n.source_url,n.author,n.color,n.created_at,n.updated_at,COALESCE(n.deleted_at,'')
		FROM notes n ` + whereSQL + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, scoreArgs...)
	args = append(args, limit, f.Offset)
	rows, err := db.SQL.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []*Note{}
	uids := []string{}
	for rows.Next() {
		n := &Note{}
		if err := rows.Scan(&n.UID, &n.Title, &n.Excerpt, &n.NotebookUID, &n.Type, &n.Status, &n.Pinned, &n.Starred,
			&n.WordCount, &n.Source, &n.SourceURL, &n.Author, &n.Color, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, n)
		uids = append(uids, n.UID)
	}
	tagMap := db.notesTagsBulk(uids)
	names := db.notebookNames()
	for _, n := range list {
		n.Tags = tagMap[n.UID]
		if n.Tags == nil {
			n.Tags = []string{}
		}
		n.NotebookName = names[n.NotebookUID]
	}
	return list, total, nil
}

func qNotebookUID(db *DB, name string) string {
	if name == "" {
		return ""
	}
	var uid string
	if db.SQL.QueryRow(`SELECT uid FROM notebooks WHERE name=? AND deleted_at IS NULL LIMIT 1`, name).Scan(&uid) == nil {
		return uid
	}
	return ""
}

func ftsQuery(q string) string {
	parts := strings.Fields(q)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ReplaceAll(p, `"`, `""`)
		out = append(out, `"`+p+`"`)
	}
	return strings.Join(out, " AND ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// GetNote loads a full note including content, tags, attachments and fields.
func (db *DB) GetNote(uid string) (*Note, error) {
	n := &Note{}
	var extra string
	var deleted sql.NullString
	err := db.SQL.QueryRow(`SELECT uid,title,content,excerpt,notebook_uid,type,status,pinned,starred,word_count,
		source,source_url,author,color,extra,created_at,updated_at,deleted_at FROM notes WHERE uid=?`, uid).
		Scan(&n.UID, &n.Title, &n.Content, &n.Excerpt, &n.NotebookUID, &n.Type, &n.Status, &n.Pinned, &n.Starred,
			&n.WordCount, &n.Source, &n.SourceURL, &n.Author, &n.Color, &extra, &n.CreatedAt, &n.UpdatedAt, &deleted)
	if err != nil {
		return nil, err
	}
	if deleted.Valid {
		n.DeletedAt = deleted.String
	}
	n.Extra = map[string]string{}
	_ = json.Unmarshal([]byte(extra), &n.Extra)
	n.Tags = db.noteTags(uid)
	n.NotebookName = db.NotebookPath(n.NotebookUID)
	n.Attachments = db.NoteAttachments(uid)
	n.Fields = db.NoteFields(uid)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM note_versions WHERE note_uid=?`, uid).Scan(&n.VersionCount)
	return n, nil
}

func (db *DB) CreateNote(n *Note) (*Note, error) {
	if n.UID == "" {
		n.UID = NewUID("nt-")
	}
	if strings.TrimSpace(n.Title) == "" {
		n.Title = TitleFromContent(n.Content)
	}
	if n.NotebookUID == "" {
		n.NotebookUID = db.DefaultNotebook()
	}
	if n.Type == "" {
		n.Type = "note"
	}
	if n.Status == "" {
		n.Status = "active"
	}
	now := Now()
	if n.CreatedAt == "" {
		n.CreatedAt = now
	} else {
		n.CreatedAt = NormalizeTime(n.CreatedAt)
	}
	n.UpdatedAt = now
	n.WordCount = CountWords(n.Content)
	n.Excerpt = Excerpt(n.Content, 180)
	if n.Extra == nil {
		n.Extra = map[string]string{}
	}
	extra, _ := json.Marshal(n.Extra)
	_, err := db.SQL.Exec(`INSERT INTO notes(uid,title,content,excerpt,notebook_uid,type,status,pinned,starred,word_count,
		source,source_url,author,color,extra,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.UID, n.Title, n.Content, n.Excerpt, n.NotebookUID, n.Type, n.Status, n.Pinned, n.Starred, n.WordCount,
		n.Source, n.SourceURL, n.Author, n.Color, string(extra), n.CreatedAt, n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(n.Tags) > 0 {
		_ = db.SetNoteTags(n.UID, n.Tags)
	}
	db.ftsUpsert(n.UID, n.Title, n.Content)
	return n, nil
}

func (db *DB) UpdateNote(n *Note) (*Note, error) {
	old, err := db.GetNote(n.UID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(n.Title) == "" {
		n.Title = TitleFromContent(n.Content)
	}
	if old.Content != n.Content {
		db.snapshot(old, "edit")
	}
	n.WordCount = CountWords(n.Content)
	n.Excerpt = Excerpt(n.Content, 180)
	n.UpdatedAt = Now()
	if n.CreatedAt == "" {
		n.CreatedAt = old.CreatedAt
	}
	if n.Type == "" {
		n.Type = old.Type
	}
	if n.Status == "" {
		n.Status = old.Status
	}
	if n.NotebookUID == "" {
		n.NotebookUID = old.NotebookUID
	}
	if n.Extra == nil {
		n.Extra = old.Extra
	}
	extra, _ := json.Marshal(n.Extra)
	_, err = db.SQL.Exec(`UPDATE notes SET title=?,content=?,excerpt=?,notebook_uid=?,type=?,status=?,pinned=?,starred=?,
		word_count=?,source=?,source_url=?,author=?,color=?,extra=?,updated_at=? WHERE uid=?`,
		n.Title, n.Content, n.Excerpt, n.NotebookUID, n.Type, n.Status, n.Pinned, n.Starred,
		n.WordCount, n.Source, n.SourceURL, n.Author, n.Color, string(extra), n.UpdatedAt, n.UID)
	if err != nil {
		return nil, err
	}
	if err := db.SetNoteTags(n.UID, n.Tags); err != nil {
		return nil, err
	}
	db.ftsUpsert(n.UID, n.Title, n.Content)
	return n, nil
}

// PatchNote updates selected fields of a note. Lightweight fields
// (pin, star, notebook, status) take a fast path; content/title/tags go
// through UpdateNote so word count, excerpt, FTS and version snapshots
// stay consistent.
func (db *DB) PatchNote(uid string, fields map[string]any) error {
	full := map[string]any{}
	light := map[string]any{}
	for k, v := range fields {
		switch k {
		case "content", "title", "tags":
			full[k] = v
		default:
			light[k] = v
		}
	}
	if len(full) > 0 {
		old, err := db.GetNote(uid)
		if err != nil {
			return err
		}
		updated := *old
		updated.DeletedAt = ""
		if v, ok := full["content"].(string); ok {
			updated.Content = v
		}
		if v, ok := full["title"].(string); ok {
			updated.Title = v
		}
		if v, ok := full["tags"].([]any); ok {
			tags := make([]string, 0, len(v))
			for _, t := range v {
				if s, ok := t.(string); ok {
					tags = append(tags, s)
				}
			}
			updated.Tags = tags
		}
		if _, err := db.UpdateNote(&updated); err != nil {
			return err
		}
	}
	sets := []string{}
	args := []any{}
	for k, v := range light {
		switch k {
		case "pinned", "starred":
			b := 0
			if vb, ok := v.(bool); ok && vb {
				b = 1
			}
			sets = append(sets, k+"=?")
			args = append(args, b)
		case "notebookUid", "status", "type":
			col := map[string]string{"notebookUid": "notebook_uid"}[k]
			if col == "" {
				col = k
			}
			sets = append(sets, col+"=?")
			args = append(args, fmt.Sprint(v))
		}
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at=?")
	args = append(args, Now(), uid)
	_, err := db.SQL.Exec(`UPDATE notes SET `+strings.Join(sets, ",")+` WHERE uid=?`, args...)
	return err
}

func (db *DB) DeleteNote(uid string) error {
	now := Now()
	_, err := db.SQL.Exec(`UPDATE notes SET deleted_at=?,updated_at=? WHERE uid=?`, now, now, uid)
	return err
}

func (db *DB) RestoreNote(uid string) error {
	_, err := db.SQL.Exec(`UPDATE notes SET deleted_at=NULL,updated_at=? WHERE uid=?`, Now(), uid)
	return err
}

func (db *DB) PurgeNote(uid string) error {
	for _, a := range db.NoteAttachments(uid) {
		_ = os.Remove(filepath.Join(db.FilesDir, a.StoredName))
	}
	stmts := []string{
		`DELETE FROM note_tags WHERE note_uid=?`,
		`DELETE FROM field_values WHERE note_uid=?`,
		`DELETE FROM attachments WHERE note_uid=?`,
		`DELETE FROM note_versions WHERE note_uid=?`,
		`DELETE FROM notes WHERE uid=?`,
	}
	for _, s := range stmts {
		if _, err := db.SQL.Exec(s, uid); err != nil {
			return err
		}
	}
	db.ftsDelete(uid)
	return nil
}

func (db *DB) EmptyTrash() (int, error) {
	rows, err := db.SQL.Query(`SELECT uid FROM notes WHERE deleted_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	uids := []string{}
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			uids = append(uids, u)
		}
	}
	rows.Close()
	for _, u := range uids {
		_ = db.PurgeNote(u)
	}
	return len(uids), nil
}

/* -------------------------------- versions ------------------------------- */

func (db *DB) snapshot(n *Note, reason string) {
	_, _ = db.SQL.Exec(`INSERT INTO note_versions(note_uid,title,content,word_count,reason,created_at) VALUES(?,?,?,?,?,?)`,
		n.UID, n.Title, n.Content, n.WordCount, reason, Now())
	// keep the newest 40 versions per note
	_, _ = db.SQL.Exec(`DELETE FROM note_versions WHERE note_uid=? AND id NOT IN
		(SELECT id FROM note_versions WHERE note_uid=? ORDER BY id DESC LIMIT 40)`, n.UID, n.UID)
}

type Version struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content,omitempty"`
	WordCount int    `json:"wordCount"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

func (db *DB) Versions(noteUID string, withContent bool) ([]Version, error) {
	cols := "id,title,word_count,reason,created_at"
	if withContent {
		cols = "id,title,content,word_count,reason,created_at"
	}
	rows, err := db.SQL.Query(`SELECT `+cols+` FROM note_versions WHERE note_uid=? ORDER BY id DESC LIMIT 60`, noteUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		v := Version{}
		if withContent {
			if rows.Scan(&v.ID, &v.Title, &v.Content, &v.WordCount, &v.Reason, &v.CreatedAt) == nil {
				out = append(out, v)
			}
		} else if rows.Scan(&v.ID, &v.Title, &v.WordCount, &v.Reason, &v.CreatedAt) == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

func (db *DB) RestoreVersion(noteUID string, id int) error {
	var title, content string
	if err := db.SQL.QueryRow(`SELECT title,content FROM note_versions WHERE id=? AND note_uid=?`, id, noteUID).Scan(&title, &content); err != nil {
		return err
	}
	current, err := db.GetNote(noteUID)
	if err != nil {
		return err
	}
	db.snapshot(current, "before-restore")
	_, err = db.SQL.Exec(`UPDATE notes SET title=?,content=?,word_count=?,excerpt=?,updated_at=? WHERE uid=?`,
		title, content, CountWords(content), Excerpt(content, 180), Now(), noteUID)
	if err == nil {
		db.ftsUpsert(noteUID, title, content)
	}
	return err
}

/* ------------------------------- attachments ----------------------------- */

func (db *DB) AddAttachment(noteUID, filename, mime string, data []byte) (*Attachment, error) {
	if filename == "" {
		filename = "附件"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = guessExt(mime)
	}
	uid := NewUID("at-")
	rel := filepath.Join(Now()[:7], uid+ext)
	full := filepath.Join(db.FilesDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return nil, err
	}
	if mime == "" {
		mime = guessMime(filename)
	}
	a := &Attachment{UID: uid, NoteUID: noteUID, Filename: filename, StoredName: filepath.ToSlash(rel),
		Mime: mime, Size: int64(len(data)), Kind: "file", CreatedAt: Now(), URL: "/api/files/" + uid}
	_, err := db.SQL.Exec(`INSERT INTO attachments(uid,note_uid,filename,stored_name,mime,size,kind,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, a.UID, a.NoteUID, a.Filename, a.StoredName, a.Mime, a.Size, a.Kind, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if noteUID != "" {
		_, _ = db.SQL.Exec(`UPDATE notes SET updated_at=? WHERE uid=?`, Now(), noteUID)
	}
	return a, nil
}

func (db *DB) NoteAttachments(noteUID string) []Attachment {
	rows, err := db.SQL.Query(`SELECT uid,note_uid,filename,stored_name,mime,size,kind,created_at FROM attachments
		WHERE note_uid=? ORDER BY created_at`, noteUID)
	if err != nil {
		return []Attachment{}
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if rows.Scan(&a.UID, &a.NoteUID, &a.Filename, &a.StoredName, &a.Mime, &a.Size, &a.Kind, &a.CreatedAt) == nil {
			a.URL = "/api/files/" + a.UID
			out = append(out, a)
		}
	}
	return out
}

func (db *DB) Attachment(uid string) (*Attachment, error) {
	a := &Attachment{}
	err := db.SQL.QueryRow(`SELECT uid,note_uid,filename,stored_name,mime,size,kind,created_at FROM attachments WHERE uid=?`, uid).
		Scan(&a.UID, &a.NoteUID, &a.Filename, &a.StoredName, &a.Mime, &a.Size, &a.Kind, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	a.URL = "/api/files/" + a.UID
	return a, nil
}

func (db *DB) AttachmentPath(a *Attachment) string {
	return filepath.Join(db.FilesDir, filepath.FromSlash(a.StoredName))
}

func (db *DB) DeleteAttachment(uid string) error {
	a, err := db.Attachment(uid)
	if err != nil {
		return err
	}
	_ = os.Remove(db.AttachmentPath(a))
	_, err = db.SQL.Exec(`DELETE FROM attachments WHERE uid=?`, uid)
	return err
}

/* --------------------------------- fields -------------------------------- */

func (db *DB) Fields() []Field {
	rows, err := db.SQL.Query(`SELECT uid,name,key,type,options,scope,required,sort FROM fields ORDER BY sort,name`)
	if err != nil {
		return []Field{}
	}
	defer rows.Close()
	out := []Field{}
	for rows.Next() {
		f := Field{}
		var opts string
		if rows.Scan(&f.UID, &f.Name, &f.Key, &f.Type, &opts, &f.Scope, &f.Required, &f.Sort) == nil {
			_ = json.Unmarshal([]byte(opts), &f.Options)
			if f.Options == nil {
				f.Options = []string{}
			}
			out = append(out, f)
		}
	}
	return out
}

func (db *DB) SaveField(f *Field) (*Field, error) {
	if f.UID == "" {
		f.UID = NewUID("fd-")
	}
	if f.Key == "" {
		f.Key = f.UID
	}
	if f.Type == "" {
		f.Type = "text"
	}
	opts, _ := json.Marshal(f.Options)
	_, err := db.SQL.Exec(`INSERT INTO fields(uid,name,key,type,options,scope,required,sort,created_at) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(uid) DO UPDATE SET name=excluded.name,key=excluded.key,type=excluded.type,options=excluded.options,
		scope=excluded.scope,required=excluded.required,sort=excluded.sort`,
		f.UID, f.Name, f.Key, f.Type, string(opts), f.Scope, f.Required, f.Sort, Now())
	return f, err
}

func (db *DB) DeleteField(uid string) error {
	_, _ = db.SQL.Exec(`DELETE FROM field_values WHERE field_uid=?`, uid)
	_, err := db.SQL.Exec(`DELETE FROM fields WHERE uid=?`, uid)
	return err
}

func (db *DB) NoteFields(noteUID string) map[string]string {
	out := map[string]string{}
	rows, err := db.SQL.Query(`SELECT field_uid,value FROM field_values WHERE note_uid=?`, noteUID)
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

func (db *DB) SetNoteFields(noteUID string, values map[string]string) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM field_values WHERE note_uid=?`, noteUID); err != nil {
		return err
	}
	for k, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO field_values(note_uid,field_uid,value) VALUES(?,?,?)`, noteUID, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

/* ------------------------------- templates ------------------------------- */

func (db *DB) Templates() []Template {
	rows, err := db.SQL.Query(`SELECT uid,name,content,notebook_uid,icon,sort FROM templates ORDER BY sort,name`)
	if err != nil {
		return []Template{}
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		if rows.Scan(&t.UID, &t.Name, &t.Content, &t.NotebookUID, &t.Icon, &t.Sort) == nil {
			out = append(out, t)
		}
	}
	return out
}

func (db *DB) SaveTemplate(t *Template) (*Template, error) {
	if t.UID == "" {
		t.UID = NewUID("tp-")
	}
	_, err := db.SQL.Exec(`INSERT INTO templates(uid,name,content,notebook_uid,icon,sort,created_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(uid) DO UPDATE SET name=excluded.name,content=excluded.content,notebook_uid=excluded.notebook_uid,
		icon=excluded.icon,sort=excluded.sort`, t.UID, t.Name, t.Content, t.NotebookUID, t.Icon, t.Sort, Now())
	return t, err
}

func (db *DB) DeleteTemplate(uid string) error {
	_, err := db.SQL.Exec(`DELETE FROM templates WHERE uid=?`, uid)
	return err
}

/* ----------------------------- saved searches ---------------------------- */

func (db *DB) SavedSearches() []SavedSearch {
	rows, err := db.SQL.Query(`SELECT uid,name,query,icon,sort FROM saved_searches ORDER BY sort,name`)
	if err != nil {
		return []SavedSearch{}
	}
	defer rows.Close()
	out := []SavedSearch{}
	for rows.Next() {
		var s SavedSearch
		if rows.Scan(&s.UID, &s.Name, &s.Query, &s.Icon, &s.Sort) == nil {
			out = append(out, s)
		}
	}
	return out
}

func (db *DB) SaveSearch(s *SavedSearch) (*SavedSearch, error) {
	if s.UID == "" {
		s.UID = NewUID("sv-")
	}
	_, err := db.SQL.Exec(`INSERT INTO saved_searches(uid,name,query,icon,sort) VALUES(?,?,?,?,?)
		ON CONFLICT(uid) DO UPDATE SET name=excluded.name,query=excluded.query,icon=excluded.icon,sort=excluded.sort`,
		s.UID, s.Name, s.Query, s.Icon, s.Sort)
	return s, err
}

func (db *DB) DeleteSearch(uid string) error {
	_, err := db.SQL.Exec(`DELETE FROM saved_searches WHERE uid=?`, uid)
	return err
}

/* ---------------------------------- fts ---------------------------------- */

func (db *DB) ftsUpsert(uid, title, content string) {
	if !db.FTS {
		return
	}
	_, _ = db.SQL.Exec(`DELETE FROM notes_fts WHERE uid=?`, uid)
	_, _ = db.SQL.Exec(`INSERT INTO notes_fts(uid,title,content) VALUES(?,?,?)`, uid, title, content)
}

func (db *DB) ftsDelete(uid string) {
	if !db.FTS {
		return
	}
	_, _ = db.SQL.Exec(`DELETE FROM notes_fts WHERE uid=?`, uid)
}

func (db *DB) RebuildFTS() error {
	if !db.FTS {
		return nil
	}
	if _, err := db.SQL.Exec(`DELETE FROM notes_fts`); err != nil {
		return err
	}
	rows, err := db.SQL.Query(`SELECT uid,title,content FROM notes`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u, t, c string
		if rows.Scan(&u, &t, &c) == nil {
			db.ftsUpsert(u, t, c)
		}
	}
	return nil
}

/* --------------------------------- helpers ------------------------------- */

func guessMime(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".zip":
		return "application/zip"
	}
	return "application/octet-stream"
}

func guessExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	}
	return ".bin"
}

// SortNotebooks flattens a notebook tree with depth for selects.
type NotebookOption struct {
	UID   string `json:"uid"`
	Name  string `json:"name"`
	Depth int    `json:"depth"`
	Path  string `json:"path"`
}

func (db *DB) NotebookOptions() []NotebookOption {
	tree, _ := db.Notebooks()
	out := []NotebookOption{}
	var walk func(list []*Notebook, depth int, prefix string)
	walk = func(list []*Notebook, depth int, prefix string) {
		for _, n := range list {
			path := n.Name
			if prefix != "" {
				path = prefix + " / " + n.Name
			}
			out = append(out, NotebookOption{UID: n.UID, Name: n.Name, Depth: depth, Path: path})
			walk(n.Children, depth+1, path)
		}
	}
	walk(tree, 0, "")
	return out
}

// NoteCounts returns the badge numbers used by the sidebar.
func (db *DB) NoteCounts() map[string]int {
	out := map[string]int{"all": 0, "starred": 0, "trash": 0, "archived": 0, "pinned": 0}
	rows, err := db.SQL.Query(`SELECT
		SUM(CASE WHEN deleted_at IS NULL AND status<>'archived' THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND status<>'archived' AND starred=1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND status='archived' THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND status<>'archived' AND pinned=1 THEN 1 ELSE 0 END)
		FROM notes`)
	if err != nil {
		return out
	}
	defer rows.Close()
	if rows.Next() {
		var a, b, c, d, e sql.NullInt64
		if rows.Scan(&a, &b, &c, &d, &e) == nil {
			out["all"], out["starred"], out["trash"], out["archived"], out["pinned"] = int(a.Int64), int(b.Int64), int(c.Int64), int(d.Int64), int(e.Int64)
		}
	}
	return out
}

// SortTagsByCount is used by statistics exports.
func SortTagsByCount(tags []Tag) {
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].Count == tags[j].Count {
			return tags[i].Name < tags[j].Name
		}
		return tags[i].Count > tags[j].Count
	})
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
