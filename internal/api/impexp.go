package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"inkvault/internal/exporter"
	"inkvault/internal/importer"
	"inkvault/internal/store"
)

func contentDisposition(name string) string {
	ascii := urlEncode(name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, ascii)
}

func (s *Server) readAttachment(a *store.Attachment) ([]byte, error) {
	return os.ReadFile(s.DB().AttachmentPath(a))
}

// tocFilterFromQuery builds the note filter shared by preview and export.
func tocFilterFromQuery(r *http.Request) store.NoteFilter {
	q := r.URL.Query()
	f := store.NoteFilter{
		Query:       q.Get("q"),
		NotebookUID: q.Get("notebook"),
		Tag:         q.Get("tag"),
		Starred:     q.Get("starred") == "1",
		Trashed:     q.Get("trashed") == "1",
		Type:        q.Get("type"),
		Sort:        firstNonEmpty(q.Get("sort"), "updated_desc"),
		Limit:       5000,
		IncludeDesc: true,
	}
	return f
}

func tocOptions(r *http.Request, s *Server) exporter.TOCOptions {
	q := r.URL.Query()
	summaryLen := atoiDefault(q.Get("summaryLen"), 110)
	opt := exporter.TOCOptions{
		Title:          q.Get("title"),
		Subtitle:       q.Get("subtitle"),
		VaultName:      s.Config().VaultName,
		GroupBy:        firstNonEmpty(q.Get("groupBy"), "notebook"),
		SummaryLen:     summaryLen,
		IncludeSummary: q.Get("summary") != "0",
		IncludeTags:    q.Get("tags") != "0",
		IncludeDates:   q.Get("dates") != "0",
		IncludeWords:   q.Get("words") != "0",
		Numbered:       q.Get("numbered") != "0",
		AppURL:         s.publicBase(r),
		FilterNote:     q.Get("filterNote"),
	}
	if opt.Title == "" {
		opt.Title = "知识库目录"
	}
	return opt
}

func (s *Server) loadTOCNotes(r *http.Request) ([]*store.Note, int, error) {
	f := tocFilterFromQuery(r)
	list, total, err := s.DB().Notes(f)
	if err != nil {
		return nil, 0, err
	}
	full := make([]*store.Note, 0, len(list))
	for _, n := range list {
		if fn, err := s.DB().GetNote(n.UID); err == nil {
			full = append(full, fn)
		}
	}
	return full, total, nil
}

func (s *Server) handleTOCPreview(w http.ResponseWriter, r *http.Request) {
	notes, total, err := s.loadTOCNotes(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	doc := exporter.BuildTOC(notes, tocOptions(r, s))
	ok(w, map[string]any{"doc": doc, "total": total, "preview": string(doc.Markdown())})
}

func (s *Server) handleExportTOC(w http.ResponseWriter, r *http.Request) {
	notes, _, err := s.loadTOCNotes(r)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	opt := tocOptions(r, s)
	doc := exporter.BuildTOC(notes, opt)
	format := firstNonEmpty(r.URL.Query().Get("format"), "md")
	data, mime, ext, err := exporter.RenderTOC(doc, format)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	name := exporter.Filename(opt.Title, ext)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", contentDisposition(name))
	_, _ = w.Write(data)
	s.DB().Audit("export.toc", format, fmt.Sprintf("%d notes", len(notes)), clientIP(r))
}

func (s *Server) handleExportNotes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Format    string   `json:"format"` // zip | json | enex
		NoteUIDs  []string `json:"noteUids"`
		Notebook  string   `json:"notebook"`
		Tag       string   `json:"tag"`
		Query     string   `json:"q"`
		Starred   bool     `json:"starred"`
		IncludeMD bool     `json:"includeMarkdown"`
		IncludeJSON bool   `json:"includeJson"`
		IncludeENEX bool   `json:"includeEnex"`
		IncludeHTML bool   `json:"includeHtml"`
		Title     string   `json:"title"`
	}
	_ = readJSON(r, &body)
	if body.Format == "" {
		body.Format = "zip"
	}
	var notes []*store.Note
	if len(body.NoteUIDs) > 0 {
		for _, uid := range body.NoteUIDs {
			if n, err := s.DB().GetNote(uid); err == nil {
				notes = append(notes, n)
			}
		}
	} else {
		list, _, err := s.DB().Notes(store.NoteFilter{NotebookUID: body.Notebook, Tag: body.Tag, Query: body.Query,
			Starred: body.Starred, Limit: 5000, IncludeDesc: true, Sort: "updated_desc"})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, n := range list {
			if full, err := s.DB().GetNote(n.UID); err == nil {
				notes = append(notes, full)
			}
		}
	}
	if len(notes) == 0 {
		writeErr(w, 400, "没有可导出的笔记")
		return
	}
	if body.Format == "json" {
		raw, _ := json.MarshalIndent(map[string]any{"app": "InkVault", "exportedAt": store.Now(), "notes": notes}, "", "  ")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(fmt.Sprintf("inkvault-notes-%s.json", store.Now()[:10])))
		_, _ = w.Write(raw)
		return
	}
	if body.Format == "enex" {
		raw, err := exporter.ENEX(notes, s.readAttachment)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(fmt.Sprintf("inkvault-%s.enex", store.Now()[:10])))
		_, _ = w.Write(raw)
		return
	}
	dump, err := s.DB().DumpVault(false, false)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	toc := exporter.BuildTOC(notes, exporter.TOCOptions{
		Title: firstNonEmpty(body.Title, "笔记导出目录"), VaultName: s.Config().VaultName,
		GroupBy: "notebook", SummaryLen: 100, IncludeSummary: true, IncludeTags: true,
		IncludeDates: true, IncludeWords: true, Numbered: true, AppURL: s.publicBase(r),
	})
	opt := exporter.VaultZipOptions{VaultName: s.Config().VaultName, IncludeJSON: body.IncludeJSON, IncludeENEX: body.IncludeENEX, IncludeHTML: body.IncludeHTML}
	raw, err := exporter.VaultZip(dump, notes, toc, opt, s.readAttachment)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(fmt.Sprintf("inkvault-notes-%s.zip", store.Now()[:10])))
	_, _ = w.Write(raw)
	s.DB().Audit("export.notes.zip", "", fmt.Sprintf("%d notes", len(notes)), clientIP(r))
}

func (s *Server) handleExportNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.DB().GetNote(r.PathValue("uid"))
	if err != nil {
		writeErr(w, 404, "笔记不存在")
		return
	}
	format := firstNonEmpty(r.URL.Query().Get("format"), "md")
	inline := r.URL.Query().Get("inline") != "0"
	var data []byte
	var mime, ext string
	switch format {
	case "md", "markdown":
		data, mime, ext = exporter.NoteMarkdown(n, true), "text/markdown; charset=utf-8", "md"
	case "txt":
		data, mime, ext = exporter.NoteTXT(n), "text/plain; charset=utf-8", "txt"
	case "html":
		data, mime, ext = exporter.NoteHTML(n, inline, s.readAttachment), "text/html; charset=utf-8", "html"
	case "docx":
		data, err = exporter.NoteDOCX(n)
		mime, ext = "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"
	case "json":
		data, err = exporter.NoteJSON(n)
		mime, ext = "application/json; charset=utf-8", "json"
	case "enex":
		data, err = exporter.ENEX([]*store.Note{n}, s.readAttachment)
		mime, ext = "application/xml; charset=utf-8", "enex"
	default:
		writeErr(w, 400, "不支持的格式")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", contentDisposition(store.SafeFileName(n.Title) + "." + ext))
	_, _ = w.Write(data)
}

func (s *Server) handleExportStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.DB().Stats()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	format := firstNonEmpty(r.URL.Query().Get("format"), "html")
	data, mime, ext, err := exporter.RenderStats(st, format)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", contentDisposition(exporter.Filename("数据报告", ext)))
	_, _ = w.Write(data)
}

func (s *Server) handleExportVault(w http.ResponseWriter, r *http.Request) {
	format := firstNonEmpty(r.URL.Query().Get("format"), "zip")
	dump, err := s.DB().DumpVault(true, true)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if format == "json" {
		raw, _ := json.MarshalIndent(dump, "", "  ")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(exporter.Filename("inkvault-vault", "json")))
		_, _ = w.Write(raw)
		return
	}
	notes := []*store.Note{}
	for _, n := range dump.Notes {
		if n.DeletedAt == "" {
			notes = append(notes, n)
		}
	}
	toc := exporter.BuildTOC(notes, exporter.TOCOptions{
		Title: "知识库总目录", VaultName: s.Config().VaultName, GroupBy: "notebook",
		SummaryLen: 110, IncludeSummary: true, IncludeTags: true, IncludeDates: true,
		IncludeWords: true, Numbered: true, AppURL: s.publicBase(r),
	})
	opt := exporter.VaultZipOptions{VaultName: s.Config().VaultName, IncludeJSON: r.URL.Query().Get("json") != "0",
		IncludeENEX: r.URL.Query().Get("enex") == "1", IncludeHTML: true}
	raw, err := exporter.VaultZip(dump, notes, toc, opt, s.readAttachment)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(exporter.Filename("inkvault-vault", "zip")))
	_, _ = w.Write(raw)
	s.DB().Audit("export.vault", "", fmt.Sprintf("%d notes", len(notes)), clientIP(r))
}

/* --------------------------------- import -------------------------------- */

func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	res, filename, err := s.parseImport(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	titles := []string{}
	for i, n := range res.Notes {
		if i >= 12 {
			break
		}
		titles = append(titles, n.Title)
	}
	notebooks := map[string]int{}
	for _, n := range res.Notes {
		notebooks[n.Notebook]++
	}
	nbList := []map[string]any{}
	for k, v := range notebooks {
		nbList = append(nbList, map[string]any{"name": k, "count": v})
	}
	sort.Slice(nbList, func(i, j int) bool { return nbList[i]["count"].(int) > nbList[j]["count"].(int) })
	ok(w, map[string]any{
		"format": res.Format, "filename": filename, "notes": len(res.Notes),
		"attachments": res.Summary["attachments"], "warnings": res.Warnings,
		"titles": titles, "notebooks": nbList,
	})
}

func (s *Server) parseImport(r *http.Request) (*importer.Result, string, error) {
	cfg := s.Config()
	r.Body = http.MaxBytesReader(wrapResponseWriter{}, r.Body, int64(cfg.MaxUploadMB)<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return nil, "", fmt.Errorf("文件过大或上传失败")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("缺少文件")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (int64(cfg.MaxUploadMB)<<20)+1))
	if err != nil {
		return nil, "", fmt.Errorf("读取文件失败")
	}
	res, err := importer.Import(header.Filename, data)
	if err != nil {
		return nil, header.Filename, err
	}
	return res, header.Filename, nil
}

// wrapResponseWriter is only used to satisfy MaxBytesReader.
type wrapResponseWriter struct{}

func (wrapResponseWriter) Header() http.Header         { return http.Header{} }
func (wrapResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (wrapResponseWriter) WriteHeader(int)             {}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	res, filename, err := s.parseImport(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	targetNotebook := r.FormValue("notebook")
	tagPrefix := strings.TrimSpace(r.FormValue("tagPrefix"))
	created, failed := 0, 0
	skipped := 0
	for _, in := range res.Notes {
		notebook := in.Notebook
		if targetNotebook != "" {
			notebook = targetNotebook
		}
		nbUID := s.DB().DefaultNotebook()
		if notebook != "" {
			nbUID = s.DB().EnsureNotebookPath(notebook)
		}
		tags := in.Tags
		if tagPrefix != "" {
			prefixed := make([]string, 0, len(tags)+1)
			prefixed = append(prefixed, tagPrefix)
			prefixed = append(prefixed, tags...)
			tags = prefixed
		}
		n, err := s.DB().CreateNote(&store.Note{
			Title: in.Title, Content: in.Content, NotebookUID: nbUID, Tags: tags,
			CreatedAt: in.CreatedAt, Source: firstNonEmpty(in.Source, "import"),
			SourceURL: in.SourceURL, Type: "note",
		})
		if err != nil {
			failed++
			continue
		}
		// store attachments and resolve placeholders
		content := n.Content
		for i, att := range in.Attachments {
			saved, err := s.DB().AddAttachment(n.UID, att.Filename, att.Mime, att.Data)
			if err != nil {
				continue
			}
			content = strings.ReplaceAll(content, fmt.Sprintf("{{INKATT:%d}}", i), "/api/files/"+saved.UID)
		}
		if len(in.Attachments) > 0 {
			content = strings.ReplaceAll(content, "{{INKATT:", "")
		}
		if content != n.Content {
			n.Content = content
			_, _ = s.DB().UpdateNote(n)
		}
		if in.UpdatedAt != "" {
			_, _ = s.DB().SQL.Exec(`UPDATE notes SET created_at=?,updated_at=? WHERE uid=?`,
				store.NormalizeTime(in.CreatedAt), store.NormalizeTime(in.UpdatedAt), n.UID)
		}
		created++
	}
	// record job
	_, _ = s.DB().SQL.Exec(`INSERT INTO import_batches(filename,format,total,created,updated,skipped,failed,note,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, filename, res.Format, len(res.Notes), created, 0, skipped, failed, strings.Join(res.Warnings, "; "), store.Now())
	s.DB().Audit("import", filename, fmt.Sprintf("%d notes", created), clientIP(r))
	ok(w, map[string]any{"ok": true, "created": created, "failed": failed, "skipped": skipped,
		"format": res.Format, "warnings": res.Warnings, "total": len(res.Notes)})
}

/* ---------------------------------- sync --------------------------------- */

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	dump, err := s.DB().DumpVault(true, true)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"vault": dump, "serverTime": store.Now(), "vaultName": s.Config().VaultName})
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	var dump store.VaultDump
	if err := readJSON(r, &dump); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	created, updated, skipped, err := s.DB().ApplyDump(&dump)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.DB().Audit("sync.push", "", fmt.Sprintf("+%d ~%d =%d", created, updated, skipped), clientIP(r))
	ok(w, map[string]any{"created": created, "updated": updated, "skipped": skipped, "serverTime": store.Now()})
}

/* ------------------------------ stats note ------------------------------- */

// handleStatsNote writes a snapshot of the dashboard into a note, so the
// statistics themselves live inside the knowledge base.
func (s *Server) handleStatsNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NotebookUID string `json:"notebookUid"`
		NoteUID     string `json:"noteUid"`
		Title       string `json:"title"`
	}
	_ = readJSON(r, &body)
	st, err := s.DB().Stats()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	title := body.Title
	if title == "" {
		title = "数据概览 · " + store.FormatLocal(store.Now())[:7]
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	b.WriteString("> 自动生成的统计快照 · " + store.FormatLocal(st.GeneratedAt) + "\n\n")
	b.WriteString("## 总览\n\n")
	b.WriteString("| 指标 | 数值 |\n| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| 笔记 | %d |\n| 总字数 | %d |\n| 篇均字数 | %d |\n| 笔记本 | %d |\n| 标签 | %d |\n", st.ActiveNotes, st.TotalWords, st.AvgWords, st.NotebookCount, st.TagCount))
	b.WriteString(fmt.Sprintf("| 星标 | %d |\n| 回收站 | %d |\n| 连续记录 | %d 天 |\n| 最活跃 | %s |\n", st.StarredNotes, st.TrashedNotes, st.Streak, st.BusiestWeekday))
	b.WriteString("\n## 笔记本分布\n\n| 笔记本 | 笔记 | 字数 |\n| --- | --- | --- |\n")
	for _, n := range st.ByNotebook {
		b.WriteString(fmt.Sprintf("| %s | %d | %d |\n", n.Name, n.Count, n.Words))
	}
	b.WriteString("\n## 高频标签\n\n")
	for i, t := range st.ByTag {
		if i >= 15 {
			break
		}
		b.WriteString(fmt.Sprintf("- #%s · %d 篇\n", t.Name, t.Count))
	}
	b.WriteString("\n## 月度趋势\n\n| 月份 | 新增 | 字数 |\n| --- | --- | --- |\n")
	for i := len(st.ByMonth) - 1; i >= 0; i-- {
		b.WriteString(fmt.Sprintf("| %s | %d | %d |\n", st.ByMonth[i].Month, st.ByMonth[i].Notes, st.ByMonth[i].Words))
	}
	b.WriteString("\n## 最长的笔记\n\n")
	for i, n := range st.TopNotes {
		b.WriteString(fmt.Sprintf("%d. [%s](#/note/%s) · %d 字\n", i+1, n.Title, n.UID, n.Words))
	}
	content := b.String()

	var note *store.Note
	if body.NoteUID != "" {
		if existing, err := s.DB().GetNote(body.NoteUID); err == nil {
			existing.Content = content
			existing.Title = title
			note, _ = s.DB().UpdateNote(existing)
		}
	}
	if note == nil {
		// reuse last auto snapshot
		list, _, _ := s.DB().Notes(store.NoteFilter{Query: "数据概览", Limit: 20})
		for _, e := range list {
			if strings.HasPrefix(e.Title, "数据概览 · ") {
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
		nb := body.NotebookUID
		if nb == "" {
			nb = s.DB().DefaultNotebook()
		}
		note, err = s.DB().CreateNote(&store.Note{Title: title, Content: content, NotebookUID: nb, Type: "report", Tags: []string{"统计"}})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	full, _ := s.DB().GetNote(note.UID)
	ok(w, full)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

var _ = strconv.Itoa
