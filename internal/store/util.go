package store

import (
	"crypto/rand"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NewUID returns a short, URL-safe, collision-resistant identifier.
func NewUID(prefix string) string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

var (
	reCodeFence  = regexp.MustCompile("(?s)```.*?```")
	reImage      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
	reMark       = regexp.MustCompile("[*_~`>#]+")
	reSpaces     = regexp.MustCompile(`\s+`)
	reTaskBox    = regexp.MustCompile(`\[[ xX]\]`)
	reIllegalWin = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)
	reOrderedItem = regexp.MustCompile(`^\d+[.)]\s+`)
)

// CountWords counts CJK characters plus latin/number words. This matches how
// Chinese users think about "字数" much better than counting whitespace.
func CountWords(s string) int {
	if s == "" {
		return 0
	}
	count := 0
	inWord := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
			count++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				count++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return count
}

// Excerpt produces a readable plain-text summary of a markdown note. Headings,
// code blocks and table rules are skipped so the summary reads like prose.
func Excerpt(md string, limit int) string {
	var keep []string
	inCode := false
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode || t == "" || t == "---" || t == "***" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "|") {
			continue
		}
		if strings.HasPrefix(t, "- [ ] ") || strings.HasPrefix(t, "- [x] ") {
			t = strings.TrimSpace(t[6:])
		} else if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			t = strings.TrimSpace(t[2:])
		} else if reOrderedItem.MatchString(t) {
			t = reOrderedItem.ReplaceAllString(t, "")
		}
		keep = append(keep, t)
	}
	s := strings.Join(keep, " ")
	s = reImage.ReplaceAllString(s, " ")
	s = reLink.ReplaceAllString(s, "$1")
	s = reTaskBox.ReplaceAllString(s, " ")
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = reMark.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reSpaces.ReplaceAllString(strings.TrimSpace(s), " ")
	if limit <= 0 {
		limit = 180
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit])) + "…"
}

// PlainText converts markdown to a readable text body (used by TXT/DOCX export).
func PlainText(md string) string {
	lines := strings.Split(md, "\n")
	var b strings.Builder
	for _, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "```"):
			continue
		case strings.HasPrefix(t, "#"):
			t = strings.TrimSpace(strings.TrimLeft(t, "#"))
		case strings.HasPrefix(t, ">"):
			t = strings.TrimSpace(strings.TrimPrefix(t, ">"))
		case strings.HasPrefix(t, "- [ ] "), strings.HasPrefix(t, "- [x] "), strings.HasPrefix(t, "- [X] "):
			t = "☐ " + t[6:]
		}
		t = reImage.ReplaceAllString(t, "")
		t = reLink.ReplaceAllString(t, "$1")
		t = strings.ReplaceAll(t, "**", "")
		t = strings.ReplaceAll(t, "__", "")
		t = strings.ReplaceAll(t, "`", "")
		b.WriteString(t)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// TitleFromContent derives a title from the first non-empty line.
func TitleFromContent(md string) string {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		t = strings.TrimLeft(t, "#> ")
		t = reImage.ReplaceAllString(t, "")
		t = reLink.ReplaceAllString(t, "$1")
		t = strings.ReplaceAll(t, "**", "")
		t = strings.TrimSpace(t)
		if t != "" {
			if utf8.RuneCountInString(t) > 80 {
				t = string([]rune(t)[:80])
			}
			return t
		}
	}
	return "无标题笔记"
}

// SafeFileName makes a title usable as a cross-platform file name.
func SafeFileName(s string) string {
	s = strings.TrimSpace(s)
	s = reIllegalWin.ReplaceAllString(s, "-")
	s = strings.Trim(s, ". ")
	if s == "" {
		s = "未命名"
	}
	if utf8.RuneCountInString(s) > 60 {
		s = string([]rune(s)[:60])
	}
	return s
}

// NormalizeTag canonicalises user input like "#工作 , 项目" into tag names.
func NormalizeTag(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	s = strings.ReplaceAll(s, "，", ",")
	return strings.TrimSpace(s)
}

// SplitTags accepts comma/space separated input.
func SplitTags(s string) []string {
	s = strings.ReplaceAll(s, "，", ",")
	s = strings.ReplaceAll(s, "、", ",")
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '#' || r == ' '
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = NormalizeTag(p)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	return out
}
