package exporter

import (
	"fmt"
	"html"
	"strings"

	"inkvault/internal/store"
)

// StatsHTML renders a printable, self-contained statistics report.
func StatsHTML(st *store.Stats) []byte {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(st.VaultName) + ` · 数据报告</title>
<style>
:root{--ink:#23211e;--muted:#7c756a;--line:#e4dfd5;--paper:#fbf9f4;--accent:#9c3d2e;--accent2:#2f4a44;--soft:#f2eee5}
*{box-sizing:border-box}body{margin:0;background:var(--paper);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Microsoft YaHei",sans-serif;line-height:1.7}
.page{max-width:1000px;margin:0 auto;padding:64px 32px 96px}
.kicker{font-size:12px;letter-spacing:.22em;color:var(--accent);font-weight:600}
h1{font-family:Georgia,"Songti SC",serif;font-size:40px;margin:12px 0 6px;font-weight:600}
.sub{color:var(--muted);margin:0 0 34px}
.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px}
.kpi{background:#fff;border:1px solid var(--line);border-radius:14px;padding:16px 18px}
.kpi .v{font-family:Georgia,serif;font-size:30px;line-height:1.2}
.kpi .l{font-size:12px;color:var(--muted);margin-top:2px}
section{margin-top:52px}
h2{font-family:Georgia,"Songti SC",serif;font-size:22px;margin:0 0 18px;padding-bottom:10px;border-bottom:1px solid var(--line);font-weight:600}
.bars{display:flex;align-items:flex-end;gap:10px;height:180px;padding-top:10px}
.bar{flex:1;display:flex;flex-direction:column;justify-content:flex-end;align-items:center;gap:6px;min-width:0}
.bar i{display:block;width:100%;max-width:44px;background:linear-gradient(180deg,#b8563f,#8e3528);border-radius:4px 4px 0 0;min-height:2px}
.bar span{font-size:10.5px;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:100%}
.rows{display:grid;gap:10px}
.row{display:grid;grid-template-columns:160px 1fr 70px;gap:12px;align-items:center;font-size:13.5px}
.row .track{background:var(--soft);height:8px;border-radius:99px;overflow:hidden}
.row .fill{height:100%;background:var(--accent2);border-radius:99px}
.row .n{text-align:right;color:var(--muted);font-variant-numeric:tabular-nums}
table{width:100%;border-collapse:collapse;font-size:13.5px}
th,td{text-align:left;padding:9px 10px;border-bottom:1px solid var(--line)}
th{font-size:12px;color:var(--muted);font-weight:500}
td.num{font-variant-numeric:tabular-nums}
.heat{display:grid;grid-auto-flow:column;grid-template-rows:repeat(7,11px);gap:3px;overflow-x:auto;padding:6px 0}
.heat i{width:11px;height:11px;border-radius:2px;display:block}
.legend{font-size:11px;color:var(--muted);display:flex;gap:6px;align-items:center;margin-top:8px}
.legend i{width:10px;height:10px;border-radius:2px;display:inline-block}
.footer{margin-top:56px;padding-top:16px;border-top:1px solid var(--line);color:var(--muted);font-size:12px;display:flex;justify-content:space-between}
@media print{body{background:#fff}.page{padding:0}section{page-break-inside:avoid}}
</style></head><body><div class="page">`)
	b.WriteString(`<div class="kicker">INKVAULT · 数据报告</div>`)
	b.WriteString(`<h1>` + html.EscapeString(st.VaultName) + `</h1>`)
	b.WriteString(`<p class="sub">生成于 ` + html.EscapeString(store.FormatLocal(st.GeneratedAt)) + ` · 数据全部来自本地 SQLite 数据库</p>`)
	b.WriteString(`<div class="kpis">`)
	kpis := [][2]string{
		{formatInt(st.ActiveNotes), "笔记"},
		{formatInt(st.TotalWords), "总字数"},
		{formatInt(st.AvgWords), "篇均字数"},
		{formatInt(st.NotebookCount), "笔记本"},
		{formatInt(st.TagCount), "标签"},
		{formatInt(st.Streak) + " 天", "连续记录"},
		{formatInt(st.AttachmentNum), "附件"},
		{formatInt(st.CreatedMonth), "本月新增"},
	}
	for _, k := range kpis {
		b.WriteString(`<div class="kpi"><div class="v">` + k[0] + `</div><div class="l">` + k[1] + `</div></div>`)
	}
	b.WriteString(`</div>`)

	// monthly trend
	b.WriteString(`<section><h2>每月新增笔记</h2><div class="bars">`)
	maxMonth := 1
	for _, m := range st.ByMonth {
		if m.Notes > maxMonth {
			maxMonth = m.Notes
		}
	}
	for _, m := range st.ByMonth {
		h := 20 + int(float64(m.Notes)/float64(maxMonth)*140)
		label := m.Month
		if len(label) >= 7 {
			label = label[2:7]
		}
		b.WriteString(fmt.Sprintf(`<div class="bar" title="%s：%d 篇 / %s 字"><i style="height:%dpx"></i><span>%s</span></div>`,
			m.Month, m.Notes, formatInt(m.Words), h, label))
	}
	b.WriteString(`</div></section>`)

	// notebook distribution
	b.WriteString(`<section><h2>笔记本分布</h2><div class="rows">`)
	maxNB := 1
	for _, n := range st.ByNotebook {
		if n.Count > maxNB {
			maxNB = n.Count
		}
	}
	for i, n := range st.ByNotebook {
		if i >= 12 {
			break
		}
		b.WriteString(fmt.Sprintf(`<div class="row"><span>%s</span><div class="track"><div class="fill" style="width:%d%%"></div></div><span class="n">%d</span></div>`,
			html.EscapeString(n.Name), n.Count*100/maxNB, n.Count))
	}
	b.WriteString(`</div></section>`)

	// tags
	b.WriteString(`<section><h2>高频标签</h2><div class="rows">`)
	maxTag := 1
	for _, t := range st.ByTag {
		if t.Count > maxTag {
			maxTag = t.Count
		}
	}
	for i, t := range st.ByTag {
		if i >= 15 {
			break
		}
		b.WriteString(fmt.Sprintf(`<div class="row"><span>#%s</span><div class="track"><div class="fill" style="width:%d%%;background:#9c3d2e"></div></div><span class="n">%d</span></div>`,
			html.EscapeString(t.Name), t.Count*100/maxTag, t.Count))
	}
	b.WriteString(`</div></section>`)

	// heatmap
	b.WriteString(`<section><h2>近一年记录热度</h2><div class="heat">`)
	maxDay := 1
	for _, d := range st.Daily {
		if d.Edits > maxDay {
			maxDay = d.Edits
		}
	}
	for _, d := range st.Daily {
		level := 0
		if d.Edits > 0 {
			level = 1 + d.Edits*3/maxDay
			if level > 4 {
				level = 4
			}
		}
		colors := []string{"#efece4", "#d9cfc4", "#bd9f92", "#a1685a", "#7f3a2c"}
		b.WriteString(fmt.Sprintf(`<i style="background:%s" title="%s · %d 次编辑"></i>`, colors[level], d.Date, d.Edits))
	}
	b.WriteString(`</div><div class="legend">少 <i style="background:#efece4"></i><i style="background:#d9cfc4"></i><i style="background:#bd9f92"></i><i style="background:#a1685a"></i><i style="background:#7f3a2c"></i> 多</div></section>`)

	// top notes
	b.WriteString(`<section><h2>最长的笔记</h2><table><thead><tr><th>标题</th><th>笔记本</th><th>字数</th><th>更新时间</th></tr></thead><tbody>`)
	for _, n := range st.TopNotes {
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td class="num">%s</td><td>%s</td></tr>`,
			html.EscapeString(n.Title), html.EscapeString(n.Notebook), formatInt(n.Words), dateOnly(n.UpdatedAt)))
	}
	b.WriteString(`</tbody></table></section>`)

	// monthly table
	b.WriteString(`<section><h2>月度明细</h2><table><thead><tr><th>月份</th><th>新增</th><th>字数</th></tr></thead><tbody>`)
	for i := len(st.ByMonth) - 1; i >= 0; i-- {
		m := st.ByMonth[i]
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td class="num">%d</td><td class="num">%s</td></tr>`, m.Month, m.Notes, formatInt(m.Words)))
	}
	b.WriteString(`</tbody></table></section>`)

	b.WriteString(`<div class="footer"><span>墨库 InkVault · 本地优先的笔记系统</span><span>` + dateOnly(st.GeneratedAt) + `</span></div>`)
	b.WriteString(`</div></body></html>`)
	return []byte(b.String())
}

func StatsMarkdown(st *store.Stats) []byte {
	var b strings.Builder
	b.WriteString("# " + st.VaultName + " · 数据报告\n\n")
	b.WriteString("> 生成于 " + store.FormatLocal(st.GeneratedAt) + "\n\n")
	b.WriteString("## 总览\n\n| 指标 | 数值 |\n| --- | --- |\n")
	rows := [][2]string{
		{"笔记总数", formatInt(st.ActiveNotes)}, {"总字数", formatInt(st.TotalWords)},
		{"篇均字数", formatInt(st.AvgWords)}, {"笔记本", formatInt(st.NotebookCount)},
		{"标签", formatInt(st.TagCount)}, {"附件", formatInt(st.AttachmentNum)},
		{"星标", formatInt(st.StarredNotes)}, {"回收站", formatInt(st.TrashedNotes)},
		{"今日新增", formatInt(st.CreatedToday)}, {"本周新增", formatInt(st.CreatedWeek)},
		{"本月新增", formatInt(st.CreatedMonth)}, {"连续记录", formatInt(st.Streak) + " 天"},
		{"最活跃", st.BusiestWeekday}, {"最长笔记", st.LongestNote},
	}
	for _, r := range rows {
		b.WriteString("| " + r[0] + " | " + r[1] + " |\n")
	}
	b.WriteString("\n## 笔记本分布\n\n| 笔记本 | 笔记数 | 字数 |\n| --- | --- | --- |\n")
	for _, n := range st.ByNotebook {
		b.WriteString(fmt.Sprintf("| %s | %d | %s |\n", n.Name, n.Count, formatInt(n.Words)))
	}
	b.WriteString("\n## 高频标签\n\n| 标签 | 笔记数 |\n| --- | --- |\n")
	for i, t := range st.ByTag {
		if i >= 20 {
			break
		}
		b.WriteString(fmt.Sprintf("| #%s | %d |\n", t.Name, t.Count))
	}
	b.WriteString("\n## 月度趋势\n\n| 月份 | 新增 | 字数 |\n| --- | --- | --- |\n")
	for i := len(st.ByMonth) - 1; i >= 0; i-- {
		m := st.ByMonth[i]
		b.WriteString(fmt.Sprintf("| %s | %d | %s |\n", m.Month, m.Notes, formatInt(m.Words)))
	}
	b.WriteString("\n## 最长的笔记\n\n")
	for i, n := range st.TopNotes {
		b.WriteString(fmt.Sprintf("%d. **%s** — %s 字 · %s\n", i+1, n.Title, formatInt(n.Words), n.Notebook))
	}
	b.WriteString("\n---\n\n*由墨库 InkVault 自动生成*\n")
	return []byte(b.String())
}

func StatsCSV(st *store.Stats) []byte {
	var b strings.Builder
	b.WriteString("\ufeff指标,数值\n")
	rows := [][2]string{
		{"笔记总数", fmt.Sprint(st.ActiveNotes)}, {"总字数", fmt.Sprint(st.TotalWords)},
		{"篇均字数", fmt.Sprint(st.AvgWords)}, {"笔记本数", fmt.Sprint(st.NotebookCount)},
		{"标签数", fmt.Sprint(st.TagCount)}, {"附件数", fmt.Sprint(st.AttachmentNum)},
		{"附件体积(字节)", fmt.Sprint(st.AttachmentSize)}, {"星标", fmt.Sprint(st.StarredNotes)},
		{"回收站", fmt.Sprint(st.TrashedNotes)}, {"今日新增", fmt.Sprint(st.CreatedToday)},
		{"本周新增", fmt.Sprint(st.CreatedWeek)}, {"本月新增", fmt.Sprint(st.CreatedMonth)},
		{"连续记录天数", fmt.Sprint(st.Streak)}, {"最活跃星期", st.BusiestWeekday},
		{"最长笔记", st.LongestNote}, {"生成时间", st.GeneratedAt},
	}
	for _, r := range rows {
		b.WriteString(csvCell(r[0]) + "," + csvCell(r[1]) + "\r\n")
	}
	b.WriteString("\r\n笔记本,笔记数,字数\r\n")
	for _, n := range st.ByNotebook {
		b.WriteString(csvCell(n.Name) + fmt.Sprintf(",%d,%d\r\n", n.Count, n.Words))
	}
	b.WriteString("\r\n标签,笔记数\r\n")
	for _, t := range st.ByTag {
		b.WriteString(csvCell(t.Name) + fmt.Sprintf(",%d\r\n", t.Count))
	}
	b.WriteString("\r\n月份,新增,字数\r\n")
	for _, m := range st.ByMonth {
		b.WriteString(fmt.Sprintf("%s,%d,%d\r\n", m.Month, m.Notes, m.Words))
	}
	return []byte(b.String())
}

func csvCell(s string) string {
	if strings.ContainsAny(s, ",\"\r\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func StatsXLSX(st *store.Stats) ([]byte, error) {
	overview := [][]string{
		{"笔记总数", formatInt(st.ActiveNotes)}, {"总字数", formatInt(st.TotalWords)},
		{"篇均字数", formatInt(st.AvgWords)}, {"笔记本数", formatInt(st.NotebookCount)},
		{"标签数", formatInt(st.TagCount)}, {"附件数", formatInt(st.AttachmentNum)},
		{"星标", formatInt(st.StarredNotes)}, {"回收站", formatInt(st.TrashedNotes)},
		{"连续记录天数", formatInt(st.Streak)}, {"最活跃星期", st.BusiestWeekday},
		{"最长笔记", st.LongestNote}, {"生成时间", store.FormatLocal(st.GeneratedAt)},
	}
	nbs := [][]string{}
	for _, n := range st.ByNotebook {
		nbs = append(nbs, []string{n.Name, fmt.Sprint(n.Count), fmt.Sprint(n.Words)})
	}
	tags := [][]string{}
	for _, t := range st.ByTag {
		tags = append(tags, []string{t.Name, fmt.Sprint(t.Count)})
	}
	months := [][]string{}
	for _, m := range st.ByMonth {
		months = append(months, []string{m.Month, fmt.Sprint(m.Notes), fmt.Sprint(m.Words)})
	}
	daily := [][]string{}
	for i := len(st.Daily) - 1; i >= 0; i-- {
		d := st.Daily[i]
		daily = append(daily, []string{d.Date, fmt.Sprint(d.Notes), fmt.Sprint(d.Edits)})
	}
	top := [][]string{}
	for i, n := range st.TopNotes {
		top = append(top, []string{fmt.Sprint(i + 1), n.Title, n.Notebook, fmt.Sprint(n.Words), dateOnly(n.UpdatedAt)})
	}
	sheets := []Sheet{
		{Name: "总览", TitleRow: st.VaultName + " 数据报告", Header: []string{"指标", "数值"}, Widths: []float64{26, 40}, Rows: overview},
		{Name: "笔记本分布", Header: []string{"笔记本", "笔记数", "字数"}, Widths: []float64{28, 12, 14}, Rows: nbs, Freeze: true, AutoFilter: true},
		{Name: "标签分布", Header: []string{"标签", "笔记数"}, Widths: []float64{26, 12}, Rows: tags, Freeze: true, AutoFilter: true},
		{Name: "月度趋势", Header: []string{"月份", "新增", "字数"}, Widths: []float64{12, 12, 14}, Rows: months, Freeze: true},
		{Name: "每日活跃", Header: []string{"日期", "新增", "编辑"}, Widths: []float64{14, 10, 10}, Rows: daily},
		{Name: "最长笔记", Header: []string{"序号", "标题", "笔记本", "字数", "更新"}, Widths: []float64{6, 40, 18, 10, 14}, Rows: top, Freeze: true},
	}
	return BuildXLSX(sheets, map[string]string{"title": st.VaultName + " 数据报告"})
}

func StatsDOCX(st *store.Stats) ([]byte, error) {
	doc := &Doc{Title: st.VaultName + " · 数据报告"}
	doc.Meta("生成于 " + store.FormatLocal(st.GeneratedAt))
	doc.Space()
	doc.H2("总览")
	doc.Table([][]string{
		{"指标", "数值"},
		{"笔记", formatInt(st.ActiveNotes)}, {"总字数", formatInt(st.TotalWords)},
		{"篇均字数", formatInt(st.AvgWords)}, {"笔记本", formatInt(st.NotebookCount)},
		{"标签", formatInt(st.TagCount)}, {"附件", formatInt(st.AttachmentNum)},
		{"连续记录", formatInt(st.Streak) + " 天"}, {"最活跃", st.BusiestWeekday},
	}, []int{2600, 4200})
	doc.H2("笔记本分布")
	rows := [][]string{{"笔记本", "笔记数", "字数"}}
	for _, n := range st.ByNotebook {
		rows = append(rows, []string{n.Name, fmt.Sprint(n.Count), formatInt(n.Words)})
	}
	doc.Table(rows, []int{3600, 1400, 1800})
	doc.H2("高频标签")
	rows = [][]string{{"标签", "笔记数"}}
	for i, t := range st.ByTag {
		if i >= 20 {
			break
		}
		rows = append(rows, []string{"#" + t.Name, fmt.Sprint(t.Count)})
	}
	doc.Table(rows, []int{4000, 2000})
	doc.H2("最长的笔记")
	for i, n := range st.TopNotes {
		doc.Bullet(fmt.Sprintf("%s — %s 字 · %s", n.Title, formatInt(n.Words), n.Notebook))
		if i >= 9 {
			break
		}
	}
	return BuildDOCX(doc)
}

// RenderStats dispatches statistics report formats.
func RenderStats(st *store.Stats, format string) ([]byte, string, string, error) {
	switch strings.ToLower(format) {
	case "html":
		return StatsHTML(st), "text/html; charset=utf-8", "html", nil
	case "md", "markdown":
		return StatsMarkdown(st), "text/markdown; charset=utf-8", "md", nil
	case "csv":
		return StatsCSV(st), "text/csv; charset=utf-8", "csv", nil
	case "xlsx":
		b, err := StatsXLSX(st)
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", err
	case "docx":
		b, err := StatsDOCX(st)
		return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	}
	return StatsHTML(st), "text/html; charset=utf-8", "html", nil
}
