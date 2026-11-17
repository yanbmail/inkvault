package importer

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTMLToMarkdown converts note HTML (Evernote, Apple Notes, Yinxiang, Youdao,
// Wiz, Smartisan web export, …) into clean Markdown. It is deliberately
// forgiving: unknown tags are unwrapped instead of dropped.
func HTMLToMarkdown(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	// strip XML declaration and doctype left over from ENEX content
	for strings.HasPrefix(src, "<?") || strings.HasPrefix(src, "<!DOCTYPE") {
		if i := strings.Index(src, ">"); i >= 0 {
			src = strings.TrimSpace(src[i+1:])
		} else {
			break
		}
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return stripTags(src)
	}
	c := &mdConv{}
	c.walk(doc, 0)
	return tidyMarkdown(c.b.String())
}

type mdConv struct {
	b     strings.Builder
	lists []listCtx
}

type listCtx struct {
	ordered bool
	index   int
}

func (c *mdConv) write(s string) { c.b.WriteString(s) }

func (c *mdConv) nl(n int) {
	s := c.b.String()
	// avoid leading blank lines and trim trailing spaces
	existing := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\n'; i-- {
		existing++
	}
	if len(strings.TrimSpace(s)) == 0 {
		return
	}
	for i := existing; i < n; i++ {
		c.b.WriteString("\n")
	}
}

func (c *mdConv) ensurePrefix(prefix string) {
	s := c.b.String()
	if len(s) == 0 || strings.HasSuffix(s, "\n") {
		c.write(prefix)
	}
}

func isBlock(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Header, atom.Footer, atom.Main,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Blockquote,
		atom.Table, atom.Pre, atom.Hr, atom.Tr, atom.Li, atom.Figure, atom.Figcaption, atom.Aside:
		return true
	}
	return false
}

func (c *mdConv) walk(n *html.Node, depth int) {
	switch n.Type {
	case html.TextNode:
		text := n.Data
		text = strings.ReplaceAll(text, "\u00a0", " ")
		text = strings.ReplaceAll(text, "\r", "")
		if strings.TrimSpace(text) == "" {
			if c.b.Len() > 0 && !strings.HasSuffix(c.b.String(), " ") && !strings.HasSuffix(c.b.String(), "\n") {
				c.write(" ")
			}
			return
		}
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if i > 0 {
				if c.inPre() {
					c.write("\n")
				} else {
					c.write(" ")
				}
			}
			c.write(strings.TrimLeft(line, " \t"))
		}
		return
	case html.ElementNode:
	default:
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			c.walk(ch, depth)
		}
		return
	}

	name := n.DataAtom
	getAttr := func(k string) string {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, k) {
				return a.Val
			}
		}
		return ""
	}

	switch name {
	case atom.Script, atom.Style, atom.Noscript, atom.Iframe, atom.Svg:
		return
	case atom.Br:
		c.write("  \n")
		return
	case atom.Hr:
		c.nl(2)
		c.write("---")
		c.nl(2)
		return
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		c.nl(2)
		level := int(n.Data[1] - '0')
		c.write(strings.Repeat("#", level) + " ")
		c.walkChildren(n, depth)
		c.nl(2)
		return
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Header, atom.Footer, atom.Main, atom.Figure, atom.Figcaption, atom.Aside:
		c.nl(2)
		if name == atom.Figcaption {
			c.write("*")
		}
		c.walkChildren(n, depth)
		if name == atom.Figcaption {
			c.write("*")
		}
		c.nl(2)
		return
	case atom.Strong, atom.B:
		c.write("**")
		c.walkChildren(n, depth)
		c.write("**")
		return
	case atom.Em, atom.I, atom.Cite:
		c.write("*")
		c.walkChildren(n, depth)
		c.write("*")
		return
	case atom.Del, atom.S, atom.Strike:
		c.write("~~")
		c.walkChildren(n, depth)
		c.write("~~")
		return
	case atom.Code:
		if c.inPre() {
			c.walkChildren(n, depth)
			return
		}
		c.write("`")
		c.walkChildren(n, depth)
		c.write("`")
		return
	case atom.Pre:
		c.nl(2)
		lang := ""
		if cls := getAttr("class"); strings.Contains(cls, "language-") {
			lang = cls[strings.Index(cls, "language-")+len("language-"):]
			if i := strings.IndexAny(lang, " \t"); i >= 0 {
				lang = lang[:i]
			}
		}
		c.write("```" + lang + "\n")
		c.write(strings.TrimRight(nodeText(n), "\n"))
		c.write("\n```")
		c.nl(2)
		return
	case atom.Blockquote:
		c.nl(2)
		inner := &mdConv{}
		inner.walkChildren(n, depth)
		for _, line := range strings.Split(tidyMarkdown(inner.b.String()), "\n") {
			c.write("> " + line + "\n")
		}
		c.nl(2)
		return
	case atom.Ul, atom.Ol:
		c.lists = append(c.lists, listCtx{ordered: name == atom.Ol})
		c.nl(1)
		c.walkChildren(n, depth)
		if len(c.lists) > 0 {
			c.lists = c.lists[:len(c.lists)-1]
		}
		c.nl(2)
		return
	case atom.Li:
		prefix := "- "
		if len(c.lists) > 0 {
			top := &c.lists[len(c.lists)-1]
			if top.ordered {
				top.index++
				prefix = itoa(top.index) + ". "
			}
		}
		c.nl(1)
		c.write(strings.Repeat("  ", max(0, len(c.lists)-1)) + prefix)
		// task list checkboxes
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && ch.DataAtom == atom.Input && strings.EqualFold(attrOf(ch, "type"), "checkbox") {
				if hasAttr(ch, "checked") {
					c.write("[x] ")
				} else {
					c.write("[ ] ")
				}
			}
		}
		c.walkChildren(n, depth+1)
		c.nl(1)
		return
	case atom.A:
		href := getAttr("href")
		text := strings.TrimSpace(nodeText(n))
		if href == "" || strings.HasPrefix(href, "javascript:") {
			c.walkChildren(n, depth)
			return
		}
		if text == "" {
			text = href
		}
		c.write("[" + text + "](" + href + ")")
		return
	case atom.Img:
		src := getAttr("src")
		if src == "" {
			return
		}
		alt := getAttr("alt")
		c.write("![" + alt + "](" + src + ")")
		return
	case atom.Table:
		c.nl(2)
		c.write(tableToMarkdown(n))
		c.nl(2)
		return
	case atom.Input:
		return
	case atom.Span, atom.Font, atom.Small, atom.Mark, atom.Sub, atom.Sup, atom.Dl, atom.Dt, atom.Dd, atom.Body, atom.Html, atom.Head, atom.Title:
		c.walkChildren(n, depth)
		return
	default:
		if isBlock(n) {
			c.nl(2)
			c.walkChildren(n, depth)
			c.nl(2)
			return
		}
		c.walkChildren(n, depth)
	}
}

func (c *mdConv) inPre() bool { return false }

func (c *mdConv) walkChildren(n *html.Node, depth int) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.walk(ch, depth)
	}
}

func attrOf(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return true
		}
	}
	return false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return b.String()
}

func tableToMarkdown(n *html.Node) string {
	rows := [][]string{}
	var collect func(*html.Node, *[]string)
	collect = func(x *html.Node, cur *[]string) {
		if x.Type == html.ElementNode && (x.DataAtom == atom.Td || x.DataAtom == atom.Th) {
			*cur = append(*cur, strings.TrimSpace(nodeText(x)))
			return
		}
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			collect(ch, cur)
		}
	}
	var walkRows func(*html.Node)
	walkRows = func(x *html.Node) {
		if x.Type == html.ElementNode && x.DataAtom == atom.Tr {
			row := []string{}
			collect(x, &row)
			if len(row) > 0 {
				rows = append(rows, row)
			}
			return
		}
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			walkRows(ch)
		}
	}
	walkRows(n)
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	var b strings.Builder
	for i, r := range rows {
		for j := 0; j < cols; j++ {
			cell := ""
			if j < len(r) {
				cell = strings.ReplaceAll(strings.ReplaceAll(r[j], "|", "\\|"), "\n", " ")
			}
			b.WriteString("| " + cell + " ")
		}
		b.WriteString("|\n")
		if i == 0 {
			for j := 0; j < cols; j++ {
				b.WriteString("| --- ")
			}
			b.WriteString("|\n")
		}
	}
	return b.String()
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch r {
		case '<':
			in = true
		case '>':
			in = false
		default:
			if !in {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func tidyMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, line)
	}
	res := strings.Join(out, "\n")
	res = strings.TrimSpace(res)
	for strings.Contains(res, "\n\n\n") {
		res = strings.ReplaceAll(res, "\n\n\n", "\n\n")
	}
	return res
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
