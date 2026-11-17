package importer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

type IncomingAttachment struct {
	Filename string
	Mime     string
	Data     []byte
}

type IncomingNote struct {
	Title       string
	Content     string
	Notebook    string
	Tags        []string
	CreatedAt   string
	UpdatedAt   string
	Source      string
	SourceURL   string
	Attachments []IncomingAttachment
}

type Result struct {
	Format   string          `json:"format"`
	Notes    []IncomingNote  `json:"-"`
	Warnings []string        `json:"warnings"`
	Summary  map[string]int  `json:"summary"`
}

const (
	maxArchiveBytes = 512 << 20
	maxFileBytes    = 128 << 20
)

// Import sniffs the payload and dispatches to the right parser.
func Import(filename string, data []byte) (*Result, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".enex":
		return importENEX(filename, data)
	case ".jex", ".tar":
		return importTar(filename, data)
	case ".tgz", ".tar.gz":
		return importTarGz(filename, data)
	case ".zip":
		return importZip(filename, data)
	case ".md", ".markdown", ".mdown", ".txt":
		return importSingleText(filename, data)
	case ".html", ".htm":
		return importSingleHTML(filename, data)
	case ".csv", ".tsv":
		return importTable(filename, data, ext)
	case ".json":
		return importJSON(filename, data)
	}
	// content sniffing
	switch {
	case bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")) && bytes.Contains(data, []byte("<en-export")):
		return importENEX(filename, data)
	case bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) || bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")):
		return importJSON(filename, data)
	case bytes.Contains(data[:min(len(data), 512)], []byte("<html")):
		return importSingleHTML(filename, data)
	}
	return nil, fmt.Errorf("暂不支持这种格式：%s。可导入 ENEX、HTML、Markdown、TXT、CSV、JSON、ZIP、JEX", ext)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// decodeText converts GBK/GB18030 content to UTF-8 and strips BOMs.
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) {
		return string(b)
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(b), "�")
}

type archive struct {
	files map[string][]byte
	names []string
}

func readZip(data []byte, _ string) (*archive, error) {
	if len(data) > maxArchiveBytes {
		return nil, fmt.Errorf("压缩包过大（上限 512 MB）")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("无法读取 ZIP：%w", err)
	}
	a := &archive{files: map[string][]byte{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > maxFileBytes {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxFileBytes))
		rc.Close()
		if err != nil {
			continue
		}
		name := cleanArchivePath(f.Name)
		if name == "" || strings.Contains(name, "__MACOSX/") {
			continue
		}
		a.files[name] = content
		a.names = append(a.names, name)
	}
	sort.Strings(a.names)
	return a, nil
}

func cleanArchivePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
		return ""
	}
	return p
}

func readTarBytes(r io.Reader) (*archive, error) {
	tr := tar.NewReader(r)
	a := &archive{files: map[string][]byte{}}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || h.Size > maxFileBytes {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(tr, maxFileBytes))
		if err != nil {
			continue
		}
		name := cleanArchivePath(h.Name)
		if name == "" {
			continue
		}
		a.files[name] = content
		a.names = append(a.names, name)
	}
	sort.Strings(a.names)
	return a, nil
}

func importZip(filename string, data []byte) (*Result, error) {
	a, err := readZip(data, filename)
	if err != nil {
		return nil, err
	}
	return importArchive(filename, a)
}

func importTar(filename string, data []byte) (*Result, error) {
	a, err := readTarBytes(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return importArchive(filename, a)
}

func importTarGz(filename string, data []byte) (*Result, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	a, err := readTarBytes(gz)
	if err != nil {
		return nil, err
	}
	return importArchive(filename, a)
}

func importArchive(filename string, a *archive) (*Result, error) {
	// 1) Evernote exports first: they carry the richest metadata.
	var enexFiles []string
	for _, n := range a.names {
		if strings.HasSuffix(strings.ToLower(n), ".enex") {
			enexFiles = append(enexFiles, n)
		}
	}
	if len(enexFiles) > 0 {
		res := &Result{Format: "enex"}
		for _, f := range enexFiles {
			r, err := importENEX(f, a.files[f])
			if err != nil {
				res.Warnings = append(res.Warnings, path.Base(f)+": "+err.Error())
				continue
			}
			res.Notes = append(res.Notes, r.Notes...)
			res.Warnings = append(res.Warnings, r.Warnings...)
		}
		return finalize(res), nil
	}

	// 2) Markdown-based vaults (Obsidian / Logseq / Joplin / Notion / Smartisan)
	var mdFiles []string
	for _, n := range a.names {
		lower := strings.ToLower(n)
		if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
			mdFiles = append(mdFiles, n)
		}
	}
	if len(mdFiles) > 0 {
		return importArchiveMarkdown(filename, a, mdFiles)
	}

	// 3) HTML exports (Apple Notes, Evernote HTML, Wiz, Yinxiang, Youdao)
	var htmlFiles []string
	for _, n := range a.names {
		lower := strings.ToLower(n)
		if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
			base := strings.ToLower(path.Base(n))
			if strings.HasPrefix(base, "index") || base == "untitled.html" && len(a.names) > 3 {
				continue
			}
			htmlFiles = append(htmlFiles, n)
		}
	}
	if len(htmlFiles) > 0 {
		return importArchiveHTML(filename, a, htmlFiles)
	}

	// 4) CSV / JSON tables
	for _, n := range a.names {
		lower := strings.ToLower(n)
		if strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".json") {
			return importTableFile(n, a.files[n], strings.ToLower(path.Ext(n)))
		}
	}
	return nil, fmt.Errorf("压缩包里没有找到可导入的笔记文件（支持 .enex / .md / .html / .csv / .json）")
}

func finalize(res *Result) *Result {
	if res.Summary == nil {
		res.Summary = map[string]int{}
	}
	res.Summary["notes"] = len(res.Notes)
	att := 0
	for _, n := range res.Notes {
		att += len(n.Attachments)
	}
	res.Summary["attachments"] = att
	if res.Notes == nil {
		res.Notes = []IncomingNote{}
	}
	return res
}

func importSingleText(filename string, data []byte) (*Result, error) {
	text := decodeText(data)
	content, meta := splitFrontmatter(text)
	title := meta["title"]
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	tags := parseTagList(meta["tags"])
	if tags == nil {
		tags = parseTagList(meta["tag"])
	}
	note := IncomingNote{
		Title: title, Content: strings.TrimSpace(content), Notebook: meta["notebook"],
		Tags: tags, CreatedAt: meta["created"], UpdatedAt: meta["updated"],
		Source: "file", SourceURL: filename,
	}
	return finalize(&Result{Format: "markdown", Notes: []IncomingNote{note}}), nil
}

func importSingleHTML(filename string, data []byte) (*Result, error) {
	raw := decodeText(data)
	title, body := htmlTitleAndBody(raw)
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	md := HTMLToMarkdown(body)
	note := IncomingNote{Title: title, Content: md, Source: "html", SourceURL: filename}
	return finalize(&Result{Format: "html", Notes: []IncomingNote{note}}), nil
}

func importTable(filename string, data []byte, ext string) (*Result, error) {
	return importTableFile(filename, data, ext)
}

func importTableFile(filename string, data []byte, ext string) (*Result, error) {
	if ext == ".json" {
		return importJSON(filename, data)
	}
	text := decodeText(data)
	var delimiter rune = ','
	if ext == ".tsv" {
		delimiter = '\t'
	} else {
		firstLine := text
		if i := strings.IndexAny(text, "\r\n"); i >= 0 {
			firstLine = text[:i]
		}
		if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
			delimiter = '\t'
		} else if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
			delimiter = ';'
		}
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delimiter
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil && len(records) == 0 {
		return nil, fmt.Errorf("CSV 解析失败：%w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("文件为空")
	}
	header := records[0]
	idx := mapColumns(header)
	start := 1
	if idx["title"] < 0 && idx["content"] < 0 {
		// no header: assume title, content, tags, notebook
		idx = map[string]int{"title": 0, "content": 1, "tags": 2, "notebook": 3, "created": 4, "updated": 5}
		start = 0
	}
	res := &Result{Format: "csv"}
	for _, rec := range records[start:] {
		get := func(key string) string {
			i, ok := idx[key]
			if !ok || i < 0 || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		title := get("title")
		content := get("content")
		if title == "" && content == "" {
			continue
		}
		if content == "" {
			content = get("excerpt")
		}
		res.Notes = append(res.Notes, IncomingNote{
			Title: title, Content: content, Notebook: get("notebook"), Tags: parseTagList(get("tags")),
			CreatedAt: get("created"), UpdatedAt: get("updated"), Source: "csv", SourceURL: filename,
		})
	}
	if len(res.Notes) == 0 {
		return nil, fmt.Errorf("CSV 里没有可导入的行")
	}
	return finalize(res), nil
}

func mapColumns(header []string) map[string]int {
	idx := map[string]int{}
	for _, k := range []string{"title", "content", "tags", "notebook", "created", "updated", "excerpt"} {
		idx[k] = -1
	}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		h = strings.ReplaceAll(h, " ", "")
		switch h {
		case "title", "name", "标题", "题目", "主题", "note", "notetitle":
			idx["title"] = i
		case "content", "body", "text", "正文", "内容", "笔记内容", "notecontent", "description":
			idx["content"] = i
		case "tags", "tag", "标签", "分类", "category":
			if idx["tags"] < 0 {
				idx["tags"] = i
			}
		case "notebook", "folder", "笔记本", "文件夹", "所属笔记本", "组":
			idx["notebook"] = i
		case "created", "createdat", "创建时间", "创建日期", "date":
			idx["created"] = i
		case "updated", "updatedat", "修改时间", "更新时间", "modified":
			idx["updated"] = i
		case "excerpt", "summary", "摘要", "简介":
			idx["excerpt"] = i
		}
	}
	return idx
}

func importJSON(filename string, data []byte) (*Result, error) {
	text := decodeText(data)
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("JSON 解析失败：%w", err)
	}
	res := &Result{Format: "json"}
	push := func(m map[string]any) {
		get := func(keys ...string) string {
			for _, k := range keys {
				if v, ok := m[k]; ok {
					switch t := v.(type) {
					case string:
						if strings.TrimSpace(t) != "" {
							return t
						}
					case float64:
						return fmt.Sprintf("%.0f", t)
					case bool:
						if t {
							return "true"
						}
					}
				}
			}
			return ""
		}
		title := get("title", "name", "标题")
		content := get("content", "body", "text", "markdown", "正文", "内容")
		tagsRaw, _ := m["tags"].([]any)
		tags := []string{}
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		if len(tags) == 0 {
			tags = parseTagList(get("tags", "标签"))
		}
		if title == "" && content == "" {
			return
		}
		res.Notes = append(res.Notes, IncomingNote{
			Title: title, Content: content, Notebook: get("notebook", "notebookName", "folder", "笔记本", "分类"),
			Tags: tags, CreatedAt: get("createdAt", "created", "创建时间"), UpdatedAt: get("updatedAt", "updated", "修改时间"),
			Source: "json", SourceURL: get("sourceUrl", "url"),
		})
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				push(m)
			}
		}
	case map[string]any:
		// InkVault dump
		nbNames := map[string]string{}
		if nbs, ok := v["notebooks"].([]any); ok {
			for _, nb := range nbs {
				if m, ok := nb.(map[string]any); ok {
					uid, _ := m["uid"].(string)
					name, _ := m["name"].(string)
					nbNames[uid] = name
				}
			}
		}
		if notes, ok := v["notes"].([]any); ok {
			for _, item := range notes {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if nb, _ := m["notebookUid"].(string); nb != "" && m["notebook"] == nil {
					m["notebook"] = nbNames[nb]
				}
				if del, _ := m["deletedAt"].(string); del != "" {
					continue
				}
				push(m)
			}
		} else {
			push(v)
		}
	}
	if len(res.Notes) == 0 {
		return nil, fmt.Errorf("JSON 里没有识别到笔记数组")
	}
	return finalize(res), nil
}

func parseTagList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '、' || r == '#' || r == ' '
	})
	out := []string{}
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitFrontmatter returns (body, metadata) for YAML-ish frontmatter.
func splitFrontmatter(text string) (string, map[string]string) {
	meta := map[string]string{}
	text = strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(text, "---") {
		return text, meta
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return text, meta
	}
	block := rest[:end]
	body := rest[end+4:]
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(line[:idx]))
		v := strings.TrimSpace(line[idx+1:])
		v = strings.Trim(v, `"'`)
		meta[k] = v
	}
	return body, meta
}

func htmlTitleAndBody(raw string) (string, string) {
	title := ""
	lower := strings.ToLower(raw)
	if i := strings.Index(lower, "<title"); i >= 0 {
		if j := strings.Index(lower[i:], ">"); j >= 0 {
			start := i + j + 1
			if k := strings.Index(lower[start:], "</title>"); k >= 0 {
				title = strings.TrimSpace(raw[start : start+k])
			}
		}
	}
	body := raw
	if i := strings.Index(lower, "<body"); i >= 0 {
		if j := strings.Index(lower[i:], ">"); j >= 0 {
			body = raw[i+j+1:]
			if k := strings.LastIndex(strings.ToLower(body), "</body>"); k >= 0 {
				body = body[:k]
			}
		}
	}
	return title, body
}

func guessMimeByName(name string) string {
	if m := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); m != "" {
		return strings.Split(m, ";")[0]
	}
	return "application/octet-stream"
}
