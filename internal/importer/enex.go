package importer

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type enexExport struct {
	XMLName xml.Name    `xml:"en-export"`
	Notes   []enexNote  `xml:"note"`
}

type enexNote struct {
	Title      string         `xml:"title"`
	Content    string         `xml:"content"`
	Created    string         `xml:"created"`
	Updated    string         `xml:"updated"`
	Tags       []string       `xml:"tag"`
	Notebook   string         `xml:"notebook"`
	Attributes enexAttributes `xml:"note-attributes"`
	Resources  []enexResource `xml:"resource"`
}

type enexAttributes struct {
	SourceURL string `xml:"source-url"`
	Source    string `xml:"source"`
	Author    string `xml:"author"`
}

type enexResource struct {
	Data       string               `xml:"data"`
	Mime       string               `xml:"mime"`
	Attributes enexResourceAttrs    `xml:"resource-attributes"`
	Width      int                  `xml:"width"`
}

type enexResourceAttrs struct {
	Filename string `xml:"file-name"`
	Source   string `xml:"source-url"`
}

var reEnMedia = regexp.MustCompile(`(?i)<en-media[^>]*?hash="([0-9a-f]+)"[^>]*?/?>`)
var reEnCrypt = regexp.MustCompile(`(?is)<en-crypt[^>]*>.*?</en-crypt>`)

// importENEX parses Evernote / 印象笔记 .enex exports including attachments.
func importENEX(filename string, data []byte) (*Result, error) {
	raw := decodeText(data)
	var doc enexExport
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		// Some exports contain invalid control chars; retry after cleaning.
		cleaned := sanitizeXML(raw)
		if err2 := xml.Unmarshal([]byte(cleaned), &doc); err2 != nil {
			return nil, fmt.Errorf("ENEX 解析失败：%w", err)
		}
	}
	notebook := strings.TrimSuffix(path.Base(strings.ReplaceAll(filename, "\\", "/")), path.Ext(filename))
	if notebook == "" {
		notebook = "印象笔记导入"
	}
	res := &Result{Format: "enex"}
	for _, en := range doc.Notes {
		n := IncomingNote{
			Title:     strings.TrimSpace(en.Title),
			Notebook:  firstNonEmpty(en.Notebook, notebook),
			Tags:      en.Tags,
			CreatedAt: en.Created,
			UpdatedAt: en.Updated,
			Source:    firstNonEmpty(en.Attributes.Source, "evernote"),
			SourceURL: en.Attributes.SourceURL,
		}
		hashToIdx := map[string]int{}
		for _, r := range en.Resources {
			bin, err := base64.StdEncoding.DecodeString(strings.TrimSpace(r.Data))
			if err != nil || len(bin) == 0 {
				continue
			}
			name := r.Attributes.Filename
			if name == "" {
				name = "附件-" + shortHash(bin) + extForMime(r.Mime)
			}
			sum := md5.Sum(bin)
			hashToIdx[hex.EncodeToString(sum[:])] = len(n.Attachments)
			n.Attachments = append(n.Attachments, IncomingAttachment{Filename: name, Mime: r.Mime, Data: bin})
		}
		html := en.Content
		html = reEnCrypt.ReplaceAllString(html, "（加密内容已跳过）")
		html = reEnMedia.ReplaceAllStringFunc(html, func(tag string) string {
			m := reEnMedia.FindStringSubmatch(tag)
			if len(m) < 2 {
				return ""
			}
			hash := strings.ToLower(m[1])
			if idx, ok := hashToIdx[hash]; ok {
				return fmt.Sprintf("![附件]({{INKATT:%d}})", idx)
			}
			return ""
		})
		// en-media may carry a filename in the tag; fall back to matching by type
		html = strings.ReplaceAll(html, "<en-todo checked=\"true\"/>", "[x] ")
		html = strings.ReplaceAll(html, "<en-todo checked=\"false\"/>", "[ ] ")
		html = strings.ReplaceAll(html, "<en-todo/>", "[ ] ")
		n.Content = HTMLToMarkdown(html)
		if n.Title == "" {
			n.Title = firstLine(n.Content)
		}
		if n.Title == "" {
			n.Title = "无标题笔记"
		}
		res.Notes = append(res.Notes, n)
	}
	if len(res.Notes) == 0 {
		return nil, fmt.Errorf("ENEX 里没有笔记")
	}
	return finalize(res), nil
}

func sanitizeXML(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r != 0xFFFE && r != 0xFFFF) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shortHash(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])[:8]
}

func extForMime(m string) string {
	switch m {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "audio/mpeg":
		return ".mp3"
	}
	return ".bin"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*- "))
		if line != "" {
			return line
		}
	}
	return ""
}
