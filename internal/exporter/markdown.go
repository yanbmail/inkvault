package exporter

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reInlineImage = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	reInlineLink  = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	reBold        = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	reItalic      = regexp.MustCompile(`(^|[\s(])\*([^*\n]+)\*($|[\s).,，。])`)
	reStrike      = regexp.MustCompile(`~~([^~]+)~~`)
	reCodeSpan    = regexp.MustCompile("`([^`]+)`")
	reTask        = regexp.MustCompile(`^\[([ xX])\]\s+`)
)

// MarkdownToHTML is a compact renderer used for standalone note exports.
// The in-app preview uses the richer JavaScript renderer.
func MarkdownToHTML(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// fenced code
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // skip closing fence
			b.WriteString(`<pre><code`)
			if lang != "" {
				b.WriteString(` class="language-` + html.EscapeString(lang) + `"`)
			}
			b.WriteString(`>` + html.EscapeString(strings.Join(code, "\n")) + `</code></pre>`)
			continue
		}
		if trimmed == "" {
			i++
			continue
		}
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			b.WriteString("<hr>")
			i++
			continue
		}
		// heading
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level <= 6 && level < len(trimmed) && trimmed[level] == ' ' {
				text := inlineMD(strings.TrimSpace(trimmed[level:]))
				b.WriteString(fmt.Sprintf(`<h%d id="h%d">%s</h%d>`, level, level, text, level))
				i++
				continue
			}
		}
		// table
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && strings.Contains(lines[i+1], "---") {
			var rows [][]string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				cells := strings.Split(strings.Trim(strings.TrimSpace(lines[i]), "|"), "|")
				clean := make([]string, len(cells))
				for j, c := range cells {
					clean[j] = strings.TrimSpace(c)
				}
				rows = append(rows, clean)
				i++
			}
			if len(rows) >= 2 {
				b.WriteString("<table><thead><tr>")
				for _, c := range rows[0] {
					b.WriteString("<th>" + inlineMD(c) + "</th>")
				}
				b.WriteString("</tr></thead><tbody>")
				for _, r := range rows[2:] {
					b.WriteString("<tr>")
					for _, c := range r {
						b.WriteString("<td>" + inlineMD(c) + "</td>")
					}
					b.WriteString("</tr>")
				}
				b.WriteString("</tbody></table>")
			}
			continue
		}
		// blockquote
		if strings.HasPrefix(trimmed, ">") {
			var quoted []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				quoted = append(quoted, inlineMD(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">"))))
				i++
			}
			b.WriteString("<blockquote>" + strings.Join(quoted, "<br>") + "</blockquote>")
			continue
		}
		// lists
		if isListLine(line) {
			tag := "ul"
			if regexp.MustCompile(`^\s*\d+\.\s`).MatchString(line) {
				tag = "ol"
			}
			b.WriteString("<" + tag + ">")
			for i < len(lines) && isListLine(lines[i]) {
				item := strings.TrimSpace(lines[i])
				item = regexp.MustCompile(`^([-*+]|\d+\.)\s+`).ReplaceAllString(item, "")
				cls := ""
				if m := reTask.FindStringSubmatch(item); m != nil {
					cls = ` class="task"`
					checked := ""
					if strings.EqualFold(m[1], "x") {
						checked = " checked"
					}
					item = reTask.ReplaceAllString(item, "")
					b.WriteString(`<li` + cls + `><input type="checkbox" disabled` + checked + `> ` + inlineMD(item) + `</li>`)
				} else {
					b.WriteString("<li>" + inlineMD(item) + "</li>")
				}
				i++
			}
			b.WriteString("</" + tag + ">")
			continue
		}
		// paragraph
		var para []string
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "```") ||
				strings.HasPrefix(t, ">") || isListLine(lines[i]) || strings.HasPrefix(t, "|") {
				break
			}
			para = append(para, inlineMD(t))
			i++
		}
		if len(para) > 0 {
			b.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
		}
	}
	return b.String()
}

func isListLine(line string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") {
		return true
	}
	return regexp.MustCompile(`^\d+\.\s`).MatchString(t)
}

func inlineMD(s string) string {
	if s == "" {
		return ""
	}
	// protect inline code
	codes := []string{}
	s = reCodeSpan.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, strings.Trim(m, "`"))
		return fmt.Sprintf("\x00%d\x00", len(codes)-1)
	})
	s = html.EscapeString(s)
	// images
	s = reInlineImage.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineImage.FindStringSubmatch(m)
		src := sub[2]
		if strings.HasPrefix(src, "&quot;") {
			return m
		}
		return `<img src="` + src + `" alt="` + sub[1] + `" loading="lazy">`
	})
	s = reInlineLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = reBold.ReplaceAllStringFunc(s, func(m string) string {
		sub := reBold.FindStringSubmatch(m)
		text := sub[1]
		if text == "" {
			text = sub[2]
		}
		return "<strong>" + text + "</strong>"
	})
	s = reStrike.ReplaceAllString(s, "<del>$1</del>")
	s = reItalic.ReplaceAllString(s, "$1<em>$2</em>$3")
	s = strings.ReplaceAll(s, "  \n", "<br>")
	// restore code
	for i, c := range codes {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), "<code>"+html.EscapeString(c)+"</code>")
	}
	return s
}

// RewriteImageLinks points markdown image URLs at new locations.
func RewriteImageLinks(md string, mapFn func(src string) string) string {
	return reInlineImage.ReplaceAllStringFunc(md, func(m string) string {
		sub := reInlineImage.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return "![" + sub[1] + "](" + mapFn(sub[2]) + ")"
	})
}
