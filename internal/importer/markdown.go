package importer

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var (
	reMDImage   = regexp.MustCompile(`!\[([^\]]*)\]\(\s*(<[^>]*>|[^)\s]+)(?:\s+"[^"]*")?\s*\)`)
	reHTMLImg   = regexp.MustCompile(`(?i)<img[^>]+src\s*=\s*["']([^"']+)["'][^>]*>`)
	reWikiEmbed = regexp.MustCompile(`!\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|([^\]]+))?\]\]`)
	reWikiLink  = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|([^\]]+))?\]\]`)
	reNotionID  = regexp.MustCompile(`\s+[0-9a-fA-F]{32}$`)
	reInlineTag = regexp.MustCompile(`(^|\s)#([\p{Han}\w\-/]{1,24})`)
)

func commonRoot(names []string) string {
	root := ""
	first := true
	for _, n := range names {
		parts := strings.Split(n, "/")
		if len(parts) < 2 {
			return ""
		}
		if first {
			root = parts[0]
			first = false
			continue
		}
		if parts[0] != root {
			return ""
		}
	}
	return root
}

func importArchiveMarkdown(filename string, a *archive, mdFiles []string) (*Result, error) {
	root := commonRoot(mdFiles)
	base := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	res := &Result{Format: "markdown"}
	lookup := newLookup(a)
	for _, name := range mdFiles {
		raw := decodeText(a.files[name])
		body, meta := splitFrontmatter(raw)
		rel := strings.TrimPrefix(name, root+"/")
		dir := path.Dir(rel)
		notebook := ""
		if dir != "." && dir != "" {
			notebook = dir
		} else if root != "" && root != base {
			notebook = root
		}
		title := meta["title"]
		fileBase := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		fileBase = reNotionID.ReplaceAllString(fileBase, "")
		if title == "" {
			title = strings.TrimSpace(fileBase)
		}
		tags := parseTagList(firstNonEmpty(meta["tags"], meta["tag"], meta["keywords"]))
		if len(tags) == 0 {
			tags = inlineTags(body)
		}
		note := IncomingNote{
			Title: title, Content: body, Notebook: firstNonEmpty(meta["notebook"], meta["folder"], notebook),
			Tags: tags, CreatedAt: firstNonEmpty(meta["created"], meta["date"], meta["createdat"]),
			UpdatedAt: firstNonEmpty(meta["updated"], meta["modified"], meta["updatedat"]),
			Source: firstNonEmpty(meta["source"], "markdown"), SourceURL: name,
		}
		note.Content = extractAttachments(note.Content, dir, lookup, &note.Attachments)
		note.Content = cleanWikiLinks(note.Content)
		if strings.TrimSpace(note.Content) == "" && len(note.Attachments) == 0 {
			continue
		}
		res.Notes = append(res.Notes, note)
	}
	if len(res.Notes) == 0 {
		return nil, fmt.Errorf("没有解析到 Markdown 笔记")
	}
	return finalize(res), nil
}

func importArchiveHTML(filename string, a *archive, htmlFiles []string) (*Result, error) {
	root := commonRoot(htmlFiles)
	base := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	res := &Result{Format: "html"}
	lookup := newLookup(a)
	for _, name := range htmlFiles {
		raw := decodeText(a.files[name])
		title, body := htmlTitleAndBody(raw)
		rel := strings.TrimPrefix(name, root+"/")
		dir := path.Dir(rel)
		notebook := ""
		if dir != "." && dir != "" {
			notebook = dir
		} else if root != "" && root != base {
			notebook = root
		}
		atts := []IncomingAttachment{}
		body = extractHTMLImages(body, dir, lookup, &atts)
		md := HTMLToMarkdown(body)
		if strings.TrimSpace(title) == "" {
			title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		}
		if strings.TrimSpace(md) == "" && len(atts) == 0 {
			continue
		}
		res.Notes = append(res.Notes, IncomingNote{
			Title: title, Content: md, Notebook: notebook, Tags: inlineTags(md),
			Source: "html", SourceURL: name, Attachments: atts,
		})
	}
	if len(res.Notes) == 0 {
		return nil, fmt.Errorf("没有解析到 HTML 笔记")
	}
	return finalize(res), nil
}

type fileLookup struct {
	a    *archive
	byLN map[string]string
}

func newLookup(a *archive) *fileLookup {
	l := &fileLookup{a: a, byLN: map[string]string{}}
	for name := range a.files {
		key := strings.ToLower(name)
		l.byLN[key] = name
		base := strings.ToLower(path.Base(name))
		if _, exists := l.byLN[base]; !exists {
			l.byLN[base] = name
		}
	}
	return l
}

func (l *fileLookup) find(dir, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return "", false
	}
	if u, err := url.QueryUnescape(ref); err == nil {
		ref = u
	}
	ref = strings.TrimPrefix(ref, "./")
	candidates := []string{}
	if dir != "" && dir != "." {
		candidates = append(candidates, path.Join(dir, ref))
	}
	candidates = append(candidates, ref, path.Base(ref))
	for _, c := range candidates {
		c = strings.TrimPrefix(c, "/")
		if _, ok := l.a.files[c]; ok {
			return c, true
		}
		if real, ok := l.byLN[strings.ToLower(c)]; ok {
			return real, true
		}
	}
	return "", false
}

func (l *fileLookup) read(name string) []byte { return l.a.files[name] }

// extractAttachments rewrites markdown image links that point into the archive
// so they become import placeholders.
func extractAttachments(md, dir string, lookup *fileLookup, atts *[]IncomingAttachment) string {
	md = reMDImage.ReplaceAllStringFunc(md, func(m string) string {
		sub := reMDImage.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		alt := sub[1]
		ref := strings.Trim(sub[2], "<>")
		if name, ok := lookup.find(dir, ref); ok {
			idx := len(*atts)
			*atts = append(*atts, IncomingAttachment{Filename: path.Base(name), Mime: guessMimeByName(name), Data: lookup.read(name)})
			return fmt.Sprintf("![%s]({{INKATT:%d}})", alt, idx)
		}
		return m
	})
	md = reWikiEmbed.ReplaceAllStringFunc(md, func(m string) string {
		sub := reWikiEmbed.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		ref := strings.TrimSpace(sub[1])
		alt := ref
		if len(sub) > 2 && sub[2] != "" {
			alt = sub[2]
		}
		if name, ok := lookup.find(dir, ref); ok {
			idx := len(*atts)
			*atts = append(*atts, IncomingAttachment{Filename: path.Base(name), Mime: guessMimeByName(name), Data: lookup.read(name)})
			return fmt.Sprintf("![%s]({{INKATT:%d}})", alt, idx)
		}
		return "![" + alt + "](" + ref + ")"
	})
	return md
}

func extractHTMLImages(body, dir string, lookup *fileLookup, atts *[]IncomingAttachment) string {
	return reHTMLImg.ReplaceAllStringFunc(body, func(tag string) string {
		sub := reHTMLImg.FindStringSubmatch(tag)
		if len(sub) < 2 {
			return tag
		}
		if name, ok := lookup.find(dir, sub[1]); ok {
			idx := len(*atts)
			*atts = append(*atts, IncomingAttachment{Filename: path.Base(name), Mime: guessMimeByName(name), Data: lookup.read(name)})
			return fmt.Sprintf(`<img src="{{INKATT:%d}}" alt="">`, idx)
		}
		return tag
	})
}

func cleanWikiLinks(md string) string {
	return reWikiLink.ReplaceAllStringFunc(md, func(m string) string {
		sub := reWikiLink.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if len(sub) > 2 && sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
}

func inlineTags(md string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, m := range reInlineTag.FindAllStringSubmatch(md, 40) {
		tag := strings.TrimSpace(m[2])
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}
