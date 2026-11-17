package exporter

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"inkvault/internal/store"
)

type Entry struct {
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	UID          string   `json:"uid"`
	Notebook     string   `json:"notebook"`
	NotebookPath string   `json:"notebookPath"`
	Tags         []string `json:"tags"`
	Summary      string   `json:"summary"`
	Words        int      `json:"words"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated"`
	Starred      bool     `json:"starred"`
	Pinned       bool     `json:"pinned"`
	Type         string   `json:"type"`
}

type Group struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
	Words   int     `json:"words"`
}

type TOCDoc struct {
	Title        string  `json:"title"`
	Subtitle     string  `json:"subtitle"`
	VaultName    string  `json:"vaultName"`
	GeneratedAt  string  `json:"generatedAt"`
	GroupBy      string  `json:"groupBy"`
	AppURL       string  `json:"appUrl"`
	TotalNotes   int     `json:"totalNotes"`
	TotalWords   int     `json:"totalWords"`
	Groups       []Group `json:"groups"`
	IncludeTags  bool    `json:"includeTags"`
	IncludeDates bool    `json:"includeDates"`
	IncludeWords bool    `json:"includeWords"`
	IncludeSum   bool    `json:"includeSummary"`
	Numbered     bool    `json:"numbered"`
	FilterNote   string  `json:"filterNote"`
}

type TOCOptions struct {
	Title          string
	Subtitle       string
	VaultName      string
	GroupBy        string // notebook | tag | month | none
	SummaryLen     int
	IncludeSummary bool
	IncludeTags    bool
	IncludeDates   bool
	IncludeWords   bool
	Numbered       bool
	AppURL         string
	FilterNote     string
}

// BuildTOC turns notes into a structured table of contents.
func BuildTOC(notes []*store.Note, opt TOCOptions) *TOCDoc {
	doc := &TOCDoc{
		Title: opt.Title, Subtitle: opt.Subtitle, VaultName: opt.VaultName,
		GeneratedAt: store.Now(), GroupBy: opt.GroupBy, AppURL: strings.TrimRight(opt.AppURL, "/"),
		IncludeTags: opt.IncludeTags, IncludeDates: opt.IncludeDates, IncludeWords: opt.IncludeWords,
		IncludeSum: opt.IncludeSummary, Numbered: opt.Numbered, FilterNote: opt.FilterNote,
	}
	if doc.Title == "" {
		doc.Title = "知识库目录"
	}
	groupMap := map[string]*Group{}
	order := []string{}
	number := 0
	for _, n := range notes {
		number++
		entry := Entry{
			Number: number, Title: n.Title, UID: n.UID, Notebook: n.NotebookName, NotebookPath: n.NotebookName,
			Tags: n.Tags, Words: n.WordCount, Created: n.CreatedAt, Updated: n.UpdatedAt,
			Starred: n.Starred, Pinned: n.Pinned, Type: n.Type,
		}
		if opt.IncludeSummary {
			limit := opt.SummaryLen
			if limit <= 0 {
				limit = 100
			}
			entry.Summary = truncateRunes(n.Excerpt, limit)
		}
		var key string
		switch opt.GroupBy {
		case "notebook":
			key = n.NotebookName
			if key == "" {
				key = "未分类"
			}
		case "tag":
			if len(n.Tags) > 0 {
				key = n.Tags[0]
			} else {
				key = "无标签"
			}
		case "month":
			key = n.CreatedAt
			if len(key) >= 7 {
				key = key[:7]
			}
		case "type":
			key = noteTypeLabel(n.Type)
		default:
			key = ""
		}
		g, ok := groupMap[key]
		if !ok {
			g = &Group{Name: key}
			groupMap[key] = g
			order = append(order, key)
		}
		g.Entries = append(g.Entries, entry)
		g.Words += n.WordCount
		doc.TotalWords += n.WordCount
	}
	doc.TotalNotes = len(notes)
	if opt.GroupBy == "month" {
		sort.Sort(sort.Reverse(sort.StringSlice(order)))
	}
	if opt.GroupBy == "none" {
		doc.Groups = []Group{{Name: "", Entries: groupMap[""].Entries, Words: groupMap[""].Words}}
	} else {
		for _, key := range order {
			doc.Groups = append(doc.Groups, *groupMap[key])
		}
	}
	if doc.Groups == nil {
		doc.Groups = []Group{}
	}
	return doc
}

func noteTypeLabel(t string) string {
	switch t {
	case "journal":
		return "日记"
	case "webclip":
		return "剪藏"
	case "todo":
		return "待办"
	case "index":
		return "目录"
	case "template":
		return "模板"
	}
	return "笔记"
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

func formatInt(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

/* ------------------------------- markdown -------------------------------- */

func (d *TOCDoc) Markdown() []byte {
	var b strings.Builder
	b.WriteString("# " + d.Title + "\n\n")
	if d.Subtitle != "" {
		b.WriteString(d.Subtitle + "\n\n")
	}
	meta := fmt.Sprintf("> 生成于 %s · 共 %s 篇 · %s 字", store.FormatLocal(d.GeneratedAt), formatInt(d.TotalNotes), formatInt(d.TotalWords))
	if d.VaultName != "" {
		meta += " · " + d.VaultName
	}
	b.WriteString(meta + "\n")
	if d.FilterNote != "" {
		b.WriteString("> 范围：" + d.FilterNote + "\n")
	}
	b.WriteString("\n")

	if len(d.Groups) > 1 {
		b.WriteString("## 分组概览\n\n")
		for _, g := range d.Groups {
			b.WriteString(fmt.Sprintf("- **%s** — %d 篇 · %s 字\n", g.Name, len(g.Entries), formatInt(g.Words)))
		}
		b.WriteString("\n")
	}

	for _, g := range d.Groups {
		if g.Name != "" {
			b.WriteString("## " + g.Name + "\n\n")
		}
		for _, e := range g.Entries {
			prefix := "- "
			if d.Numbered {
				prefix = fmt.Sprintf("%d. ", e.Number)
			}
			line := prefix + "**" + e.Title + "**"
			if d.AppURL != "" {
				line = prefix + "[" + e.Title + "](" + d.AppURL + "/#/note/" + e.UID + ")"
			}
			b.WriteString(line + "\n")
			if d.IncludeSum && e.Summary != "" {
				b.WriteString("  " + strings.ReplaceAll(e.Summary, "\n", " ") + "\n")
			}
			var bits []string
			if d.IncludeTags && len(e.Tags) > 0 {
				bits = append(bits, "#"+strings.Join(e.Tags, " #"))
			}
			if d.IncludeWords && e.Words > 0 {
				bits = append(bits, formatInt(e.Words)+" 字")
			}
			if d.IncludeDates {
				bits = append(bits, "更新于 "+dateOnly(e.Updated))
			}
			if d.GroupBy != "notebook" && e.Notebook != "" {
				bits = append(bits, e.Notebook)
			}
			if len(bits) > 0 {
				b.WriteString("  *" + strings.Join(bits, " · ") + "*\n")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n---\n\n*由墨库 InkVault 自动生成 · " + dateOnly(d.GeneratedAt) + "*\n")
	return []byte(b.String())
}

/* ---------------------------------- html --------------------------------- */

func (d *TOCDoc) HTML() []byte {
	var b strings.Builder
	title := html.EscapeString(d.Title)
	b.WriteString(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + title + `</title>
<style>
:root{--ink:#23211e;--muted:#7d766a;--line:#e3ded4;--paper:#fbf9f4;--accent:#9c3d2e;--accent-soft:#f4e9e4}
*{box-sizing:border-box}
body{margin:0;background:var(--paper);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Microsoft YaHei",sans-serif;line-height:1.75}
.page{max-width:860px;margin:0 auto;padding:72px 32px 96px}
.kicker{font-size:12px;letter-spacing:.22em;text-transform:uppercase;color:var(--accent);font-weight:600}
h1{font-family:Georgia,"Songti SC","Noto Serif SC",serif;font-size:42px;line-height:1.25;margin:14px 0 10px;font-weight:600}
.sub{color:var(--muted);font-size:16px;margin:0 0 22px}
.meta{display:flex;flex-wrap:wrap;gap:10px;margin:26px 0 8px}
.chip{background:#fff;border:1px solid var(--line);border-radius:999px;padding:6px 14px;font-size:13px;color:#4e4a43}
.chip b{color:var(--ink);font-weight:600}
.rule{height:1px;background:linear-gradient(90deg,var(--accent),var(--line) 45%,transparent);margin:30px 0 40px}
.overview{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:10px;margin:0 0 48px;padding:0;list-style:none}
.overview a{display:flex;justify-content:space-between;gap:12px;text-decoration:none;color:var(--ink);background:#fff;border:1px solid var(--line);border-radius:12px;padding:12px 14px;font-size:14px}
.overview a:hover{border-color:var(--accent);color:var(--accent)}
.overview span{color:var(--muted);font-size:12px;white-space:nowrap}
h2{font-family:Georgia,"Songti SC",serif;font-size:24px;margin:52px 0 6px;padding-bottom:10px;border-bottom:1px solid var(--line);font-weight:600}
h2 .count{font-family:-apple-system,sans-serif;font-size:12px;color:var(--muted);font-weight:400;margin-left:10px}
.entries{list-style:none;padding:0;margin:0}
.entry{padding:18px 0 16px;border-bottom:1px dashed var(--line);page-break-inside:avoid}
.entry:last-child{border-bottom:0}
.entry-head{display:flex;gap:14px;align-items:baseline}
.num{font-family:Georgia,serif;color:var(--accent);font-size:13px;min-width:26px;font-variant-numeric:tabular-nums}
.entry-title{font-size:17px;font-weight:600;color:var(--ink);text-decoration:none}
.entry-title:hover{color:var(--accent)}
.summary{color:#5a554d;font-size:14px;margin:8px 0 0 40px}
.entry-meta{display:flex;flex-wrap:wrap;gap:12px;margin:8px 0 0 40px;font-size:12px;color:var(--muted)}
.tag{color:var(--accent);background:var(--accent-soft);border-radius:4px;padding:2px 7px}
.footer{margin-top:64px;padding-top:18px;border-top:1px solid var(--line);font-size:12px;color:var(--muted);display:flex;justify-content:space-between;gap:16px}
@media print{body{background:#fff}.page{padding:0;max-width:none}.overview{display:none}h2{page-break-after:avoid}}
@media(max-width:640px){.page{padding:40px 20px}h1{font-size:30px}}
</style></head><body><div class="page">`)
	b.WriteString(`<div class="kicker">InkVault · 目录</div>`)
	b.WriteString(`<h1>` + title + `</h1>`)
	if d.Subtitle != "" {
		b.WriteString(`<p class="sub">` + html.EscapeString(d.Subtitle) + `</p>`)
	}
	b.WriteString(`<div class="meta">`)
	b.WriteString(`<span class="chip"><b>` + formatInt(d.TotalNotes) + `</b> 篇笔记</span>`)
	b.WriteString(`<span class="chip"><b>` + formatInt(d.TotalWords) + `</b> 字</span>`)
	b.WriteString(`<span class="chip">` + html.EscapeString(store.FormatLocal(d.GeneratedAt)) + `</span>`)
	if d.VaultName != "" {
		b.WriteString(`<span class="chip">` + html.EscapeString(d.VaultName) + `</span>`)
	}
	if d.FilterNote != "" {
		b.WriteString(`<span class="chip">范围：` + html.EscapeString(d.FilterNote) + `</span>`)
	}
	b.WriteString(`</div><div class="rule"></div>`)

	if len(d.Groups) > 1 {
		b.WriteString(`<ul class="overview">`)
		for i, g := range d.Groups {
			b.WriteString(fmt.Sprintf(`<li><a href="#g%d"><span>%s</span><span>%d 篇 · %s 字</span></a></li>`, i, html.EscapeString(g.Name), len(g.Entries), formatInt(g.Words)))
		}
		b.WriteString(`</ul>`)
	}
	for gi, g := range d.Groups {
		if g.Name != "" {
			b.WriteString(fmt.Sprintf(`<h2 id="g%d">%s<span class="count">%d 篇 · %s 字</span></h2>`, gi, html.EscapeString(g.Name), len(g.Entries), formatInt(g.Words)))
		}
		b.WriteString(`<ol class="entries" style="list-style:none">`)
		for _, e := range g.Entries {
			titleText := html.EscapeString(e.Title)
			link := ""
			if d.AppURL != "" {
				link = d.AppURL + "/#/note/" + e.UID
			}
			b.WriteString(`<li class="entry"><div class="entry-head">`)
			if d.Numbered {
				b.WriteString(fmt.Sprintf(`<span class="num">%02d</span>`, e.Number))
			}
			if link != "" {
				b.WriteString(`<a class="entry-title" href="` + html.EscapeString(link) + `">` + titleText + `</a>`)
			} else {
				b.WriteString(`<span class="entry-title">` + titleText + `</span>`)
			}
			b.WriteString(`</div>`)
			if d.IncludeSum && e.Summary != "" {
				b.WriteString(`<p class="summary">` + html.EscapeString(e.Summary) + `</p>`)
			}
			var bits []string
			if d.IncludeTags {
				for _, t := range e.Tags {
					bits = append(bits, `<span class="tag">#`+html.EscapeString(t)+`</span>`)
				}
			}
			if d.IncludeWords && e.Words > 0 {
				bits = append(bits, formatInt(e.Words)+" 字")
			}
			if d.IncludeDates {
				bits = append(bits, "更新于 "+dateOnly(e.Updated))
			}
			if d.GroupBy != "notebook" && e.Notebook != "" {
				bits = append(bits, html.EscapeString(e.Notebook))
			}
			if e.Pinned {
				bits = append(bits, "置顶")
			}
			if len(bits) > 0 {
				b.WriteString(`<div class="entry-meta">` + strings.Join(bits, " · ") + `</div>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ol>`)
	}
	b.WriteString(`<div class="footer"><span>由墨库 InkVault 自动生成</span><span>` + dateOnly(d.GeneratedAt) + `</span></div>`)
	b.WriteString(`</div></body></html>`)
	return []byte(b.String())
}

/* ----------------------------------- csv --------------------------------- */

func (d *TOCDoc) CSV() []byte {
	var b strings.Builder
	b.WriteString("\ufeff")
	header := []string{"序号", "标题", "笔记本", "标签", "摘要", "字数", "创建时间", "更新时间", "星标", "置顶", "类型", "UID"}
	b.WriteString(strings.Join(header, ",") + "\r\n")
	esc := func(s string) string {
		if strings.ContainsAny(s, ",\"\r\n") {
			return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		return s
	}
	for _, g := range d.Groups {
		for _, e := range g.Entries {
			row := []string{
				fmt.Sprint(e.Number), esc(e.Title), esc(e.Notebook), esc(strings.Join(e.Tags, " ")),
				esc(e.Summary), fmt.Sprint(e.Words), dateOnly(e.Created), dateOnly(e.Updated),
				boolMark(e.Starred), boolMark(e.Pinned), noteTypeLabel(e.Type), e.UID,
			}
			b.WriteString(strings.Join(row, ",") + "\r\n")
		}
	}
	return []byte(b.String())
}

func boolMark(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

/* ---------------------------------- xlsx --------------------------------- */

func (d *TOCDoc) XLSX() ([]byte, error) {
	rows := [][]string{}
	for _, g := range d.Groups {
		for _, e := range g.Entries {
			rows = append(rows, []string{
				fmt.Sprint(e.Number), e.Title, e.Notebook, strings.Join(e.Tags, " "), e.Summary,
				fmt.Sprint(e.Words), dateOnly(e.Created), dateOnly(e.Updated),
				boolMark(e.Starred), boolMark(e.Pinned), noteTypeLabel(e.Type),
			})
		}
	}
	sheets := []Sheet{{
		Name: "笔记目录",
		TitleRow: d.Title,
		Header: []string{"序号", "标题", "笔记本", "标签", "摘要", "字数", "创建", "更新", "星标", "置顶", "类型"},
		Widths: []float64{6, 34, 16, 18, 52, 8, 12, 12, 6, 6, 8},
		Rows: rows, Freeze: true, AutoFilter: true,
	}}
	overview := [][]string{}
	for _, g := range d.Groups {
		if g.Name == "" {
			continue
		}
		overview = append(overview, []string{g.Name, fmt.Sprint(len(g.Entries)), fmt.Sprint(g.Words)})
	}
	if len(overview) > 0 {
		sheets = append(sheets, Sheet{
			Name: "分组概览", Header: []string{"分组", "笔记数", "字数"},
			Widths: []float64{30, 12, 12}, Rows: overview, Freeze: true,
		})
	}
	return BuildXLSX(sheets, map[string]string{"title": d.Title})
}

/* ---------------------------------- docx --------------------------------- */

func (d *TOCDoc) DOCX() ([]byte, error) {
	doc := &Doc{Title: d.Title}
	doc.P(fmt.Sprintf("共 %s 篇笔记 · %s 字 · 生成于 %s", formatInt(d.TotalNotes), formatInt(d.TotalWords), store.FormatLocal(d.GeneratedAt)))
	if d.Subtitle != "" {
		doc.P(d.Subtitle)
	}
	if d.FilterNote != "" {
		doc.Meta("范围：" + d.FilterNote)
	}
	doc.Space()
	if len(d.Groups) > 1 {
		doc.H2("分组概览")
		rows := [][]string{{"分组", "笔记数", "字数"}}
		for _, g := range d.Groups {
			rows = append(rows, []string{g.Name, fmt.Sprint(len(g.Entries)), formatInt(g.Words)})
		}
		doc.Table(rows, []int{4000, 1500, 1500})
	}
	for _, g := range d.Groups {
		if g.Name != "" {
			doc.H2(g.Name)
		}
		for _, e := range g.Entries {
			label := e.Title
			if d.Numbered {
				label = fmt.Sprintf("%02d  %s", e.Number, e.Title)
			}
			doc.H3(label)
			if d.IncludeSum && e.Summary != "" {
				doc.P(e.Summary)
			}
			var bits []string
			if d.IncludeTags && len(e.Tags) > 0 {
				bits = append(bits, "#"+strings.Join(e.Tags, " #"))
			}
			if d.IncludeWords && e.Words > 0 {
				bits = append(bits, formatInt(e.Words)+" 字")
			}
			if d.IncludeDates {
				bits = append(bits, "更新于 "+dateOnly(e.Updated))
			}
			if len(bits) > 0 {
				doc.Meta(strings.Join(bits, " · "))
			}
		}
	}
	return BuildDOCX(doc)
}

func (d *TOCDoc) JSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// RenderTOC dispatches to the requested format.
func RenderTOC(doc *TOCDoc, format string) ([]byte, string, string, error) {
	switch strings.ToLower(format) {
	case "md", "markdown":
		return doc.Markdown(), "text/markdown; charset=utf-8", "md", nil
	case "html":
		return doc.HTML(), "text/html; charset=utf-8", "html", nil
	case "csv":
		return doc.CSV(), "text/csv; charset=utf-8", "csv", nil
	case "xlsx":
		b, err := doc.XLSX()
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", err
	case "docx":
		b, err := doc.DOCX()
		return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	case "json":
		b, err := doc.JSON()
		return b, "application/json; charset=utf-8", "json", err
	}
	return doc.Markdown(), "text/markdown; charset=utf-8", "md", nil
}

// Filename builds a friendly download name.
func Filename(base, ext string) string {
	if base == "" {
		base = "inkvault"
	}
	return fmt.Sprintf("%s-%s.%s", store.SafeFileName(base), time.Now().Format("20060102-1504"), ext)
}
