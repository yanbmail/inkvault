package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"inkvault/internal/store"
)

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

/* --------------------------------- notes --------------------------------- */

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.NoteFilter{
		Query:       q.Get("q"),
		NotebookUID: q.Get("notebook"),
		Tag:         q.Get("tag"),
		Type:        q.Get("type"),
		Status:      q.Get("status"),
		Starred:     q.Get("starred") == "1",
		Pinned:      q.Get("pinned") == "1",
		Trashed:     q.Get("trashed") == "1",
		Archived:    q.Get("archived") == "1",
		From:        q.Get("from"),
		To:          q.Get("to"),
		Sort:        q.Get("sort"),
		IncludeDesc: q.Get("desc") != "0",
	}
	f.Limit = atoiDefault(q.Get("limit"), 60)
	f.Offset = atoiDefault(q.Get("offset"), 0)
	list, total, err := s.DB().Notes(f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"notes": list, "total": total, "limit": f.Limit, "offset": f.Offset})
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.DB().GetNote(r.PathValue("uid"))
	if err != nil {
		writeErr(w, 404, "笔记不存在")
		return
	}
	ok(w, n)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string            `json:"title"`
		Content     string            `json:"content"`
		NotebookUID string            `json:"notebookUid"`
		Type        string            `json:"type"`
		Tags        []string          `json:"tags"`
		Starred     bool              `json:"starred"`
		Pinned      bool              `json:"pinned"`
		Source      string            `json:"source"`
		SourceURL   string            `json:"sourceUrl"`
		Fields      map[string]string `json:"fields"`
		Extra       map[string]string `json:"extra"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if body.NotebookUID == "" {
		body.NotebookUID = s.DB().GetSetting("default_notebook", "")
	}
	n, err := s.DB().CreateNote(&store.Note{
		Title: body.Title, Content: body.Content, NotebookUID: body.NotebookUID, Type: body.Type,
		Tags: body.Tags, Starred: body.Starred, Pinned: body.Pinned, Source: body.Source,
		SourceURL: body.SourceURL, Extra: body.Extra,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(body.Fields) > 0 {
		_ = s.DB().SetNoteFields(n.UID, body.Fields)
	}
	full, _ := s.DB().GetNote(n.UID)
	writeJSON(w, 201, full)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	var body store.Note
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	body.UID = uid
	n, err := s.DB().UpdateNote(&body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if body.Fields != nil {
		_ = s.DB().SetNoteFields(uid, body.Fields)
	}
	full, _ := s.DB().GetNote(n.UID)
	ok(w, full)
}

func (s *Server) handlePatchNote(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.DB().PatchNote(r.PathValue("uid"), body); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	full, err := s.DB().GetNote(r.PathValue("uid"))
	if err != nil {
		writeErr(w, 404, "笔记不存在")
		return
	}
	ok(w, full)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteNote(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handleRestoreNote(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().RestoreNote(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handlePurgeNote(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().PurgeNote(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handleDuplicateNote(w http.ResponseWriter, r *http.Request) {
	src, err := s.DB().GetNote(r.PathValue("uid"))
	if err != nil {
		writeErr(w, 404, "笔记不存在")
		return
	}
	copyNote := &store.Note{
		Title: src.Title + " 副本", Content: src.Content, NotebookUID: src.NotebookUID,
		Type: src.Type, Tags: src.Tags, Starred: src.Starred, Source: src.Source, SourceURL: src.SourceURL,
		Author: src.Author, Extra: src.Extra,
	}
	n, err := s.DB().CreateNote(copyNote)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// copy attachments
	for _, a := range src.Attachments {
		data, err := os.ReadFile(s.DB().AttachmentPath(&a))
		if err != nil {
			continue
		}
		na, err := s.DB().AddAttachment(n.UID, a.Filename, a.Mime, data)
		if err != nil {
			continue
		}
		oldURL := "/api/files/" + a.UID
		newURL := "/api/files/" + na.UID
		n.Content = strings.ReplaceAll(n.Content, oldURL, newURL)
	}
	if len(src.Attachments) > 0 {
		updated, _ := s.DB().UpdateNote(n)
		if updated != nil {
			n = updated
		}
	}
	_ = s.DB().SetNoteFields(n.UID, src.Fields)
	full, _ := s.DB().GetNote(n.UID)
	writeJSON(w, 201, full)
}

func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request) {
	withContent := r.URL.Query().Get("content") == "1"
	list, err := s.DB().Versions(r.PathValue("uid"), withContent)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

func (s *Server) handleRestoreVersion(w http.ResponseWriter, r *http.Request) {
	id := atoiDefault(r.PathValue("id"), 0)
	if err := s.DB().RestoreVersion(r.PathValue("uid"), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	full, _ := s.DB().GetNote(r.PathValue("uid"))
	ok(w, full)
}

func (s *Server) handleSetFields(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.DB().SetNoteFields(r.PathValue("uid"), body); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	cfg := s.Config()
	maxBytes := int64(cfg.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, 400, "上传失败或文件超过限制")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		writeErr(w, 400, "文件过大")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	att, err := s.DB().AddAttachment(r.PathValue("uid"), header.Filename, mimeType, data)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, att)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	att, err := s.DB().Attachment(r.PathValue("uid"))
	if err != nil {
		writeErr(w, 404, "文件不存在")
		return
	}
	path := s.DB().AttachmentPath(att)
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 404, "文件已丢失")
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	w.Header().Set("Content-Type", att.Mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	disposition := "inline"
	if !strings.HasPrefix(att.Mime, "image/") && !strings.HasPrefix(att.Mime, "audio/") &&
		!strings.HasPrefix(att.Mime, "video/") && att.Mime != "application/pdf" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, urlEncode(att.Filename)))
	http.ServeContent(w, r, att.Filename, fi.ModTime(), f)
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, r := range []byte(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' || r == '~' {
			b.WriteByte(r)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteAttachment(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* ------------------------------- notebooks ------------------------------- */

func (s *Server) handleListNotebooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB().Notebooks()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"notebooks": list, "options": s.DB().NotebookOptions()})
}

func (s *Server) handleCreateNotebook(w http.ResponseWriter, r *http.Request) {
	var body store.Notebook
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	nb, err := s.DB().CreateNotebook(&body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, nb)
}

func (s *Server) handleUpdateNotebook(w http.ResponseWriter, r *http.Request) {
	var body store.Notebook
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	body.UID = r.PathValue("uid")
	if body.ParentUID == body.UID {
		body.ParentUID = ""
	}
	if err := s.DB().UpdateNotebook(&body); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, body)
}

func (s *Server) handleDeleteNotebook(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteNotebook(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* --------------------------------- tags ---------------------------------- */

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	ok(w, s.DB().TagCloud())
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, 400, "标签名为空")
		return
	}
	uid, err := s.DB().EnsureTag(body.Name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if body.Color != "" {
		_ = s.DB().SetTagColor(uid, body.Color)
	}
	ok(w, map[string]any{"uid": uid, "name": store.NormalizeTag(body.Name)})
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if body.Name != "" {
		if err := s.DB().RenameTag(r.PathValue("uid"), body.Name); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.Color != "" {
		_ = s.DB().SetTagColor(r.PathValue("uid"), body.Color)
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteTag(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* -------------------------------- fields --------------------------------- */

func (s *Server) handleListFields(w http.ResponseWriter, r *http.Request) {
	ok(w, s.DB().Fields())
}

func (s *Server) handleSaveField(w http.ResponseWriter, r *http.Request) {
	var body store.Field
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	f, err := s.DB().SaveField(&body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, f)
}

func (s *Server) handleDeleteField(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteField(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* ------------------------------- templates ------------------------------- */

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	ok(w, s.DB().Templates())
}

func (s *Server) handleSaveTemplate(w http.ResponseWriter, r *http.Request) {
	var body store.Template
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	t, err := s.DB().SaveTemplate(&body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, t)
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteTemplate(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* ----------------------------- saved searches ---------------------------- */

func (s *Server) handleListSearches(w http.ResponseWriter, r *http.Request) {
	ok(w, s.DB().SavedSearches())
}

func (s *Server) handleSaveSearch(w http.ResponseWriter, r *http.Request) {
	var body store.SavedSearch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Query) == "" {
		writeErr(w, 400, "名称和查询不能为空")
		return
	}
	sv, err := s.DB().SaveSearch(&body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, sv)
}

func (s *Server) handleDeleteSearch(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteSearch(r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* --------------------------------- trash --------------------------------- */

func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	n, err := s.DB().EmptyTrash()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true, "deleted": n})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.DB().Stats()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, st)
}

/* ----------------------------- index note -------------------------------- */

// handleMakeIndexNote creates (or refreshes) a note that acts as a living
// table of contents for a notebook.
func (s *Server) handleMakeIndexNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NotebookUID string `json:"notebookUid"`
		Title       string `json:"title"`
		NoteUID     string `json:"noteUid"`
		Include     string `json:"include"` // summary|title|tags
	}
	_ = readJSON(r, &body)
	if body.NotebookUID == "" {
		body.NotebookUID = r.PathValue("uid")
	}
	if body.NotebookUID == "" {
		writeErr(w, 400, "请指定笔记本")
		return
	}
	notes, total, err := s.DB().Notes(store.NoteFilter{NotebookUID: body.NotebookUID, IncludeDesc: true, Limit: 1000, Sort: "title"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	path := s.DB().NotebookPath(body.NotebookUID)
	title := body.Title
	if title == "" {
		title = "目录 · " + path
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	b.WriteString(fmt.Sprintf("> 自动汇总 · 共 %d 篇笔记 · 更新于 %s\n\n", total, store.FormatLocal(store.Now())))
	groups := map[string][]*store.Note{}
	order := []string{}
	for _, n := range notes {
		key := n.NotebookName
		if key == "" {
			key = path
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], n)
	}
	for _, key := range order {
		if len(groups) > 1 {
			b.WriteString("## " + key + "\n\n")
		}
		for _, n := range groups[key] {
			line := fmt.Sprintf("- [%s](#/note/%s)", n.Title, n.UID)
			if n.WordCount > 0 {
				line += fmt.Sprintf(" · %d 字", n.WordCount)
			}
			if len(n.Tags) > 0 {
				line += " · #" + strings.Join(n.Tags, " #")
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	content := b.String()

	var note *store.Note
	if body.NoteUID != "" {
		existing, err := s.DB().GetNote(body.NoteUID)
		if err == nil {
			existing.Content = content
			existing.Title = title
			note, err = s.DB().UpdateNote(existing)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	if note == nil {
		// reuse an existing auto index note in this notebook when possible
		existing, _, _ := s.DB().Notes(store.NoteFilter{NotebookUID: body.NotebookUID, Query: "目录", Limit: 50})
		for _, e := range existing {
			if strings.HasPrefix(e.Title, "目录 · ") {
				full, err := s.DB().GetNote(e.UID)
				if err == nil {
					full.Content = content
					full.Title = title
					note, _ = s.DB().UpdateNote(full)
					break
				}
			}
		}
	}
	if note == nil {
		note, err = s.DB().CreateNote(&store.Note{Title: title, Content: content, NotebookUID: body.NotebookUID, Type: "index", Tags: []string{"目录"}})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	full, _ := s.DB().GetNote(note.UID)
	ok(w, full)
}

// ensure sort is used in this file (for future handlers)
var _ = sort.Strings
