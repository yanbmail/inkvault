package exporter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Block types for the small DOCX builder.
type Block struct {
	Kind   string // h1 h2 h3 p meta quote bullet table pagebreak space
	Text   string
	Rows   [][]string
	Widths []int
	Header bool
}

type Doc struct {
	Title  string
	Blocks []Block
}

func (d *Doc) H1(t string)          { d.Blocks = append(d.Blocks, Block{Kind: "h1", Text: t}) }
func (d *Doc) H2(t string)          { d.Blocks = append(d.Blocks, Block{Kind: "h2", Text: t}) }
func (d *Doc) H3(t string)          { d.Blocks = append(d.Blocks, Block{Kind: "h3", Text: t}) }
func (d *Doc) P(t string)           { d.Blocks = append(d.Blocks, Block{Kind: "p", Text: t}) }
func (d *Doc) Meta(t string)        { d.Blocks = append(d.Blocks, Block{Kind: "meta", Text: t}) }
func (d *Doc) Quote(t string)       { d.Blocks = append(d.Blocks, Block{Kind: "quote", Text: t}) }
func (d *Doc) Bullet(t string)      { d.Blocks = append(d.Blocks, Block{Kind: "bullet", Text: t}) }
func (d *Doc) Space()               { d.Blocks = append(d.Blocks, Block{Kind: "space"}) }
func (d *Doc) PageBreak()           { d.Blocks = append(d.Blocks, Block{Kind: "pagebreak"}) }
func (d *Doc) Table(rows [][]string, widths []int) {
	d.Blocks = append(d.Blocks, Block{Kind: "table", Rows: rows, Widths: widths})
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const docxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const docxDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>
</Relationships>`

const docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults>
<w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="微软雅黑" w:cs="Calibri"/><w:sz w:val="21"/><w:szCs w:val="21"/><w:color w:val="23211E"/></w:rPr></w:rPrDefault>
<w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault>
</w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>
<w:style w:type="paragraph" w:styleId="DocTitle"><w:name w:val="Doc Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="0" w:after="120"/></w:pPr><w:rPr><w:b/><w:sz w:val="44"/><w:color w:val="1F2A26"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Sub"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="240"/></w:pPr><w:rPr><w:sz w:val="19"/><w:color w:val="6B6459"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="H1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="360" w:after="120"/><w:outlineLvl w:val="0"/><w:keepNext/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/><w:color w:val="1F2A26"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="H2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="300" w:after="100"/><w:outlineLvl w:val="1"/><w:keepNext/></w:pPr><w:rPr><w:b/><w:sz w:val="26"/><w:color w:val="2F4A44"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="H3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="240" w:after="80"/><w:outlineLvl w:val="2"/><w:keepNext/></w:pPr><w:rPr><w:b/><w:sz w:val="23"/><w:color w:val="23211E"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Meta"><w:name w:val="Meta"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="40"/></w:pPr><w:rPr><w:sz w:val="18"/><w:color w:val="8A8378"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="C9C2B4"/></w:pBdr><w:ind w:left="240"/></w:pPr><w:rPr><w:color w:val="6B6459"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="ListItem"><w:name w:val="List Item"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="360" w:hanging="200"/><w:spacing w:after="40"/></w:pPr></w:style>
</w:styles>`

const docxFooter = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:p><w:pPr><w:jc w:val="center"/></w:pPr>
<w:r><w:rPr><w:sz w:val="16"/><w:color w:val="9C9486"/></w:rPr><w:t xml:space="preserve">墨库 InkVault · 第 </w:t></w:r>
<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>
<w:r><w:rPr><w:sz w:val="16"/><w:color w:val="9C9486"/></w:rPr><w:t xml:space="preserve"> 页</w:t></w:r>
</w:p></w:ftr>`

func wText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteString("<w:br/>")
		}
		b.WriteString(`<w:t xml:space="preserve">` + xmlEsc(line) + `</w:t>`)
	}
	return b.String()
}

func wPara(text, style string) string {
	ps := ""
	if style != "" {
		ps = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + ps + `<w:r>` + wText(text) + `</w:r></w:p>`
}

func wBullet(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="ListItem"/></w:pPr><w:r><w:t xml:space="preserve">• </w:t></w:r><w:r>` + wText(text) + `</w:r></w:p>`
}

func wTable(rows [][]string, widths []int) string {
	if len(rows) == 0 {
		return ""
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	if total == 0 {
		total = 100
	}
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(`<w:` + side + ` w:val="single" w:sz="4" w:space="0" w:color="DDD8CE"/>`)
	}
	b.WriteString(`</w:tblBorders><w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:left w:w="100" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)
	for i := range rows[0] {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		if w == 0 {
			w = 2400
		}
		b.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, w))
	}
	b.WriteString(`</w:tblGrid>`)
	for ri, row := range rows {
		b.WriteString(`<w:tr>`)
		if ri == 0 {
			b.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for _, cell := range row {
			b.WriteString(`<w:tc><w:tcPr>`)
			if ri == 0 {
				b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F3F1EC"/>`)
			}
			b.WriteString(`</w:tcPr><w:p><w:r>`)
			if ri == 0 {
				b.WriteString(`<w:rPr><w:b/></w:rPr>`)
			}
			b.WriteString(wText(cell) + `</w:r></w:p></w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl><w:p><w:pPr><w:spacing w:after="0"/></w:pPr></w:p>`)
	return b.String()
}

// BuildDOCX renders a Doc into a .docx file.
func BuildDOCX(doc *Doc) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	write := func(name, body string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(body))
		return err
	}
	if err := write("[Content_Types].xml", docxContentTypes); err != nil {
		return nil, err
	}
	if err := write("_rels/.rels", docxRels); err != nil {
		return nil, err
	}
	if err := write("word/_rels/document.xml.rels", docxDocumentRels); err != nil {
		return nil, err
	}
	if err := write("word/styles.xml", docxStyles); err != nil {
		return nil, err
	}
	if err := write("word/footer1.xml", docxFooter); err != nil {
		return nil, err
	}

	var body strings.Builder
	for _, blk := range doc.Blocks {
		switch blk.Kind {
		case "h1":
			body.WriteString(wPara(blk.Text, "H1"))
		case "h2":
			body.WriteString(wPara(blk.Text, "H2"))
		case "h3":
			body.WriteString(wPara(blk.Text, "H3"))
		case "p":
			body.WriteString(wPara(blk.Text, ""))
		case "meta":
			body.WriteString(wPara(blk.Text, "Meta"))
		case "quote":
			body.WriteString(wPara(blk.Text, "Quote"))
		case "bullet":
			body.WriteString(wBullet(blk.Text))
		case "space":
			body.WriteString(`<w:p><w:pPr><w:spacing w:after="0"/></w:pPr></w:p>`)
		case "pagebreak":
			body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		case "table":
			body.WriteString(wTable(blk.Rows, blk.Widths))
		}
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>` + body.String() + `
<w:sectPr><w:footerReference w:type="default" r:id="rId2"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1247" w:bottom="1440" w:left="1247" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr>
</w:body></w:document>`
	if err := write("word/document.xml", document); err != nil {
		return nil, err
	}
	core := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<dc:title>` + xmlEsc(doc.Title) + `</dc:title><dc:creator>InkVault</dc:creator><cp:lastModifiedBy>InkVault</cp:lastModifiedBy>
<dcterms:created xsi:type="dcterms:W3CDTF">` + time.Now().UTC().Format(time.RFC3339) + `</dcterms:created></cp:coreProperties>`
	if err := write("docProps/core.xml", core); err != nil {
		return nil, err
	}
	if err := write("docProps/app.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>InkVault</Application></Properties>`); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
