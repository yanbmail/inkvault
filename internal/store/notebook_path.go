package store

import "strings"

// EnsureNotebookPath finds or creates a notebook from a "A / B / C" path and
// returns the leaf uid. Used by every importer.
func (db *DB) EnsureNotebookPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.Trim(path, "/")
	if path == "" {
		return db.DefaultNotebook()
	}
	parts := strings.Split(path, "/")
	parent := ""
	cur := ""
	for _, raw := range parts {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		cur = name
		var uid string
		err := db.SQL.QueryRow(`SELECT uid FROM notebooks WHERE deleted_at IS NULL AND name=? AND parent_uid=?`, name, parent).Scan(&uid)
		if err != nil {
			nb, cerr := db.CreateNotebook(&Notebook{Name: name, ParentUID: parent})
			if cerr != nil {
				return db.DefaultNotebook()
			}
			uid = nb.UID
		}
		parent = uid
	}
	if parent == "" {
		return db.DefaultNotebook()
	}
	_ = cur
	return parent
}

// NotebookExists reports whether a notebook uid is valid.
func (db *DB) NotebookExists(uid string) bool {
	var n int
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notebooks WHERE uid=? AND deleted_at IS NULL`, uid).Scan(&n)
	return n > 0
}
