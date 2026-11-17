package exporter

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"inkvault/internal/store"
)

// NoteMarkdown renders a note with portable YAML frontmatter.
func NoteMarkdown(n *store.Note, withFrontmatter bool) []byte {
	var b strings.Builder
	if withFrontmatter {
		b.WriteString("---\n")
		b.WriteString("title: " + yamlString(n.Title) + "\n")
		if len(n.Tags) > 0 {
			b.WriteString("tags: [" + strings.Join(n.Tags, ", ") + "]\n")
		}
		if n.NotebookName != "" {
			b.WriteString("notebook: " + yamlString(n.NotebookName) + "\n")
		}
		b.WriteString("created: " + n.CreatedAt + "\n")
		b.WriteString("updated: " + n.UpdatedAt + "\n")
		b.WriteString("uid: " + n.UID + "\n")
		if n.SourceURL != "" {
			b.WriteString("source: " + yamlString(n.SourceURL) + "\n")
		}
		b.WriteString("---\n\n")
	}
	b.WriteString(n.Content)
	if !strings.HasSuffix(n.Content, "\n") {
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func yamlString(s string) string {
	if strings.ContainsAny(s, ":#\"'\n[]{}") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func NoteTXT(n *store.Note) []byte {
	var b strings.Builder
	b.WriteString(n.Title + "\n")
	b.WriteString(strings.Repeat("=", minInt(40, len([]rune(n.Title)))) + "\n\n")
	b.WriteString(store.PlainText(n.Content))
	b.WriteString("\n\n---\n")
	b.WriteString("笔记本：" + n.NotebookName + "\n")
	if len(n.Tags) > 0 {
		b.WriteString("标签：" + strings.Join(n.Tags, " / ") + "\n")
	}
	b.WriteString("字数：" + fmt.Sprint(n.WordCount) + "\n")
	b.WriteString("创建：" + store.FormatLocal(n.CreatedAt) + "\n")
	b.WriteString("更新：" + store.FormatLocal(n.UpdatedAt) + "\n")
	return []byte(b.String())
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var reAPIFile = regexp.MustCompile(`!\[([^\]]*)\]\((/api/files/([a-zA-Z0-9\-]+))\)`)

// NoteHTML produces a self-contained reading page. When inlineFiles is true
// images are embedded as data URIs.
func NoteHTML(n *store.Note, inlineFiles bool, readFile func(*store.Attachment) ([]byte, error)) []byte {
	md := n.Content
	if inlineFiles && readFile != nil {
		byUID := map[string]*store.Attachment{}
		for i := range n.Attachments {
			byUID[n.Attachments[i].UID] = &n.Attachments[i]
		}
		md = reAPIFile.ReplaceAllStringFunc(md, func(m string) string {
			sub := reAPIFile.FindStringSubmatch(m)
			if len(sub) < 4 {
				return m
			}
			att := byUID[sub[3]]
			if att == nil {
				return m
			}
			data, err := readFile(att)
			if err != nil {
				return m
			}
			return fmt.Sprintf("![%s](data:%s;base64,%s)", sub[1], att.Mime, base64.StdEncoding.EncodeToString(data))
		})
	}
	body := MarkdownToHTML(md)
	var tags strings.Builder
	for _, t := range n.Tags {
		tags.WriteString(`<span class="tag">#` + html.EscapeString(t) + `</span>`)
	}
	page := `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(n.Title) + `</title>
<style>
:root{--ink:#23211e;--muted:#7d766a;--line:#e5e0d6;--paper:#fbf9f4;--accent:#9c3d2e}
*{box-sizing:border-box}body{margin:0;background:var(--paper);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Microsoft YaHei",sans-serif;line-height:1.85}
.page{max-width:760px;margin:0 auto;padding:72px 28px 96px}
h1{font-family:Georgia,"Songti SC",serif;font-size:38px;line-height:1.3;margin:0 0 12px;font-weight:600}
.meta{color:var(--muted);font-size:13px;display:flex;flex-wrap:wrap;gap:12px;margin-bottom:26px}
.tag{color:var(--accent);background:#f4e9e4;border-radius:4px;padding:2px 8px;font-size:12px}
hr{border:0;border-top:1px solid var(--line);margin:32px 0}
h2,h3,h4{font-family:Georgia,"Songti SC",serif;margin:36px 0 12px;line-height:1.4}
h2{font-size:26px}h3{font-size:21px}h4{font-size:18px}
p{margin:0 0 16px}img{max-width:100%;border-radius:10px;display:block;margin:20px 0}
blockquote{margin:20px 0;padding:6px 18px;border-left:3px solid var(--accent);color:#5d574e;background:#f6f2ea}
code{background:#f1ece1;padding:2px 6px;border-radius:4px;font-size:.9em}
pre{background:#f1ece1;padding:16px;border-radius:10px;overflow:auto}pre code{background:none;padding:0}
table{border-collapse:collapse;width:100%;margin:20px 0;font-size:14px}th,td{border:1px solid var(--line);padding:8px 10px;text-align:left}
th{background:#f6f2ea}
a{color:var(--accent)}
.footer{margin-top:48px;padding-top:16px;border-top:1px solid var(--line);font-size:12px;color:var(--muted)}
li.task{list-style:none;margin-left:-20px}
</style></head><body><div class="page">`
	page += `<h1>` + html.EscapeString(n.Title) + `</h1><div class="meta"><span>` + html.EscapeString(n.NotebookName) + `</span><span>` + store.FormatLocal(n.UpdatedAt) + `</span><span>` + fmt.Sprint(n.WordCount) + ` 字</span>` + tags.String() + `</div><hr>`
	page += body
	page += `<div class="footer">由墨库 InkVault 导出 · ` + store.FormatLocal(store.Now()) + `</div></div></body></html>`
	return []byte(page)
}

func NoteDOCX(n *store.Note) ([]byte, error) {
	doc := &Doc{Title: n.Title}
	meta := n.NotebookName
	if meta != "" {
		meta += " · "
	}
	meta += store.FormatLocal(n.UpdatedAt) + fmt.Sprintf(" · %d 字", n.WordCount)
	doc.Meta(meta)
	if len(n.Tags) > 0 {
		doc.Meta("#" + strings.Join(n.Tags, " #"))
	}
	doc.Space()
	appendMarkdownBlocks(doc, n.Content)
	return BuildDOCX(doc)
}

// appendMarkdownBlocks maps markdown structure onto the DOCX block model.
func appendMarkdownBlocks(doc *Doc, md string) {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			i++
		case strings.HasPrefix(line, "```"):
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++
			doc.P(strings.Join(code, "\n"))
		case strings.HasPrefix(line, "### "):
			doc.H3(strings.TrimSpace(line[4:]))
			i++
		case strings.HasPrefix(line, "## "):
			doc.H3(strings.TrimSpace(line[3:]))
			i++
		case strings.HasPrefix(line, "# "):
			doc.H2(strings.TrimSpace(line[2:]))
			i++
		case strings.HasPrefix(line, "> "):
			doc.Quote(strings.TrimSpace(strings.TrimPrefix(line, ">")))
			i++
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			doc.Bullet(strings.TrimSpace(line[2:]))
			i++
		case strings.HasPrefix(line, "|") && i+1 < len(lines) && strings.Contains(lines[i+1], "---"):
			var rows [][]string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				cells := strings.Split(strings.Trim(strings.TrimSpace(lines[i]), "|"), "|")
				for j := range cells {
					cells[j] = strings.TrimSpace(cells[j])
				}
				rows = append(rows, cells)
				i++
			}
			if len(rows) >= 2 {
				clean := [][]string{rows[0]}
				clean = append(clean, rows[2:]...)
				doc.Table(clean, nil)
			}
		default:
			doc.P(plainInline(line))
			i++
		}
	}
}

func plainInline(s string) string {
	s = reInlineImage.ReplaceAllString(s, "")
	s = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile("`([^`]+)`").ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

func NoteJSON(n *store.Note) ([]byte, error) {
	return json.MarshalIndent(n, "", "  ")
}

/* ---------------------------------- ENEX --------------------------------- */

func ENEX(notes []*store.Note, readFile func(*store.Attachment) ([]byte, error)) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE en-export SYSTEM "http://xml.evernote.com/pub/evernote-export4.dtd">` + "\n")
	b.WriteString(fmt.Sprintf(`<en-export export-date="%s" application="InkVault" version="%s">`+"\n",
		time.Now().UTC().Format("20060102T150405Z"), "1.0.0"))
	for _, n := range notes {
		body := MarkdownToHTML(n.Content)
		attByUID := map[string]store.Attachment{}
		for _, a := range n.Attachments {
			attByUID[a.UID] = a
		}
		var resources strings.Builder
		body = reAPIFile.ReplaceAllStringFunc(body, func(m string) string {
			sub := reAPIFile.FindStringSubmatch(m)
			if len(sub) < 4 || readFile == nil {
				return m
			}
			att, ok := attByUID[sub[3]]
			if !ok {
				return m
			}
			data, err := readFile(&att)
			if err != nil {
				return m
			}
			sum := md5.Sum(data)
			resources.WriteString("<resource><data encoding=\"base64\">" + base64.StdEncoding.EncodeToString(data) + "</data>")
			resources.WriteString("<mime>" + att.Mime + "</mime>")
			resources.WriteString("<resource-attributes><file-name>" + html.EscapeString(att.Filename) + "</file-name></resource-attributes></resource>\n")
			return fmt.Sprintf(`<en-media hash="%s" type="%s"/>`, hex.EncodeToString(sum[:]), att.Mime)
		})
		b.WriteString("<note>")
		b.WriteString("<title>" + html.EscapeString(n.Title) + "</title>")
		b.WriteString("<content><![CDATA[<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE en-note SYSTEM \"http://xml.evernote.com/pub/enml2.dtd\"><en-note>" + body + "</en-note>]]></content>")
		b.WriteString("<created>" + enexTime(n.CreatedAt) + "</created>")
		b.WriteString("<updated>" + enexTime(n.UpdatedAt) + "</updated>")
		for _, t := range n.Tags {
			b.WriteString("<tag>" + html.EscapeString(t) + "</tag>")
		}
		if n.SourceURL != "" {
			b.WriteString("<note-attributes><source-url>" + html.EscapeString(n.SourceURL) + "</source-url></note-attributes>")
		}
		b.WriteString(resources.String())
		b.WriteString("</note>\n")
	}
	b.WriteString("</en-export>")
	return []byte(b.String()), nil
}

func enexTime(s string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", s)
	if err != nil {
		return time.Now().UTC().Format("20060102T150405Z")
	}
	return t.UTC().Format("20060102T150405Z")
}

/* -------------------------------- vault zip ------------------------------ */

type VaultZipOptions struct {
	VaultName   string
	IncludeJSON bool
	IncludeENEX bool
	IncludeHTML bool
}

// VaultZip exports the whole vault as a portable folder tree:
// markdown notes grouped by notebook, attachments, TOC and a manifest.
func VaultZip(dump *store.VaultDump, notes []*store.Note, toc *TOCDoc, opt VaultZipOptions, readFile func(*store.Attachment) ([]byte, error)) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	add := func(name string, data []byte) error {
		if name == "" {
			return nil
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	root := store.SafeFileName(opt.VaultName)
	if root == "" {
		root = "InkVault"
	}
	// notebook path map
	byUID := map[string]*store.Notebook{}
	for _, nb := range dump.Notebooks {
		byUID[nb.UID] = nb
	}
	pathOf := func(uid string) string {
		parts := []string{}
		seen := map[string]bool{}
		for cur := byUID[uid]; cur != nil && !seen[cur.UID]; cur = byUID[cur.ParentUID] {
			seen[cur.UID] = true
			parts = append([]string{store.SafeFileName(cur.Name)}, parts...)
		}
		return strings.Join(parts, "/")
	}

	used := map[string]int{}
	attNames := map[string]string{}
	noteDir := map[string]string{}
	for _, n := range notes {
		dir := pathOf(n.NotebookUID)
		noteDir[n.UID] = dir
	}
	// write attachments once, flat, with uid names
	for _, n := range notes {
		for i := range n.Attachments {
			a := n.Attachments[i]
			if _, ok := attNames[a.UID]; ok {
				continue
			}
			data, err := readFile(&a)
			if err != nil {
				continue
			}
			name := a.UID + extOf(a.Filename)
			attNames[a.UID] = name
			if err := add(root+"/_附件/"+name, data); err != nil {
				return nil, err
			}
		}
	}
	count := 0
	for _, n := range notes {
		dir := noteDir[n.UID]
		content := string(NoteMarkdown(n, true))
		// point image links at the attachment folder, relative to this note
		depth := 0
		if dir != "" {
			depth = len(strings.Split(dir, "/"))
		}
		content = reAPIFile.ReplaceAllStringFunc(content, func(m string) string {
			sub := reAPIFile.FindStringSubmatch(m)
			if len(sub) < 4 {
				return m
			}
			name, ok := attNames[sub[3]]
			if !ok {
				return m
			}
			rel := strings.Repeat("../", depth) + "_附件/" + name
			return "![" + sub[1] + "](" + rel + ")"
		})
		base := store.SafeFileName(n.Title)
		if base == "未命名" || base == "" {
			base = "笔记"
		}
		fname := base + ".md"
		full := root
		if dir != "" {
			full += "/" + dir
		}
		full += "/" + fname
		if c, ok := used[full]; ok {
			used[full] = c + 1
			full = strings.TrimSuffix(full, ".md") + fmt.Sprintf(" (%d).md", c+1)
		} else {
			used[full] = 1
		}
		if err := add(full, []byte(content)); err != nil {
			return nil, err
		}
		count++
	}
	if toc != nil {
		if err := add(root+"/目录.md", toc.Markdown()); err != nil {
			return nil, err
		}
		if opt.IncludeHTML {
			if err := add(root+"/目录.html", toc.HTML()); err != nil {
				return nil, err
			}
		}
	}
	if opt.IncludeJSON && dump != nil {
		raw, err := json.MarshalIndent(dump, "", "  ")
		if err == nil {
			if err := add(root+"/数据.json", raw); err != nil {
				return nil, err
			}
		}
	}
	if opt.IncludeENEX {
		raw, err := ENEX(notes, readFile)
		if err == nil {
			if err := add(root+"/全部笔记.enex", raw); err != nil {
				return nil, err
			}
		}
	}
	man := map[string]any{
		"app": "InkVault", "version": "1.0.0", "exportedAt": store.Now(),
		"vault": opt.VaultName, "notes": count, "attachments": len(attNames),
		"notebooks": len(dump.Notebooks), "tags": len(dump.Tags),
	}
	raw, _ := json.MarshalIndent(man, "", "  ")
	if err := add(root+"/manifest.json", raw); err != nil {
		return nil, err
	}
	readme := "墨库 InkVault 导出包\n\n" +
		fmt.Sprintf("笔记数量：%d\n附件数量：%d\n导出时间：%s\n\n", count, len(attNames), store.FormatLocal(store.Now())) +
		"目录结构：\n" +
		"  目录.md / 目录.html   —— 全部笔记的目录（标题、简介、标签、字数）\n" +
		"  <笔记本>/<标题>.md    —— 每篇笔记的 Markdown 原文（含 YAML 头信息）\n" +
		"  _附件/                —— 图片与附件原文件\n" +
		"  数据.json             —— 完整结构化数据（可再次导入墨库）\n\n" +
		"这些 Markdown 文件可以直接放进 Obsidian、Logseq、Typora、VS Code 使用。\n"
	if err := add(root+"/说明.txt", []byte(readme)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 || i < len(name)-8 {
		if i < 0 {
			return ""
		}
	}
	return name[i:]
}
