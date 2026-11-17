package exporter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Sheet is a simple, dependency-free spreadsheet model.
type Sheet struct {
	Name      string
	Header    []string
	Widths    []float64
	Rows      [][]string
	Freeze    bool
	AutoFilter bool
	TitleRow  string
}

const xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
%s
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
%s
<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="4">
<font><sz val="11"/><name val="Calibri"/><color rgb="FF1F2937"/></font>
<font><b/><sz val="11"/><name val="Calibri"/><color rgb="FFFFFFFF"/></font>
<font><b/><sz val="14"/><name val="Calibri"/><color rgb="FF1F2937"/></font>
<font><b/><sz val="11"/><name val="Calibri"/><color rgb="FF1F2937"/></font>
</fonts>
<fills count="4">
<fill><patternFill patternType="none"/></fill>
<fill><patternFill patternType="gray125"/></fill>
<fill><patternFill patternType="solid"><fgColor rgb="FF2F4A44"/><bgColor indexed="64"/></patternFill></fill>
<fill><patternFill patternType="solid"><fgColor rgb="FFF3F1EC"/><bgColor indexed="64"/></patternFill></fill>
</fills>
<borders count="2">
<border><left/><right/><top/><bottom/><diagonal/></border>
<border><left/><right/><top/><bottom style="thin"><color rgb="FFD8D4CC"/></bottom><diagonal/></border>
</borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="5">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="1" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyAlignment="1"><alignment horizontal="left" vertical="center"/></xf>
<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf>
<xf numFmtId="0" fontId="3" fillId="0" borderId="1" xfId="0" applyFont="1" applyBorder="1" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

// BuildXLSX renders multiple sheets into an xlsx workbook.
func BuildXLSX(sheets []Sheet, meta map[string]string) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("没有可导出的数据")
	}
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

	overrides := ""
	for i := range sheets {
		overrides += fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`+"\n", i+1)
	}
	if err := write("[Content_Types].xml", fmt.Sprintf(xlsxContentTypes, overrides)); err != nil {
		return nil, err
	}
	if err := write("_rels/.rels", xlsxRootRels); err != nil {
		return nil, err
	}
	rels := ""
	shetTags := ""
	for i := range sheets {
		rels += fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`+"\n", i+1, i+1)
		name := sheets[i].Name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		if len([]rune(name)) > 30 {
			name = string([]rune(name)[:30])
		}
		shetTags += fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEsc(name), i+1, i+1)
	}
	workbook := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets>` + shetTags + `</sheets>
</workbook>`
	if err := write("xl/workbook.xml", workbook); err != nil {
		return nil, err
	}
	if err := write("xl/_rels/workbook.xml.rels", fmt.Sprintf(xlsxWorkbookRels, rels)); err != nil {
		return nil, err
	}
	if err := write("xl/styles.xml", xlsxStyles); err != nil {
		return nil, err
	}
	for i := range sheets {
		if err := write(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), buildSheetXML(sheets[i])); err != nil {
			return nil, err
		}
	}
	title := meta["title"]
	if title == "" {
		title = "InkVault 导出"
	}
	core := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<dc:title>` + xmlEsc(title) + `</dc:title>
<dc:creator>InkVault</dc:creator>
<cp:lastModifiedBy>InkVault</cp:lastModifiedBy>
<dcterms:created xsi:type="dcterms:W3CDTF">` + time.Now().UTC().Format(time.RFC3339) + `</dcterms:created>
</cp:coreProperties>`
	if err := write("docProps/core.xml", core); err != nil {
		return nil, err
	}
	app := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>InkVault</Application></Properties>`
	if err := write("docProps/app.xml", app); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildSheetXML(s Sheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	totalCols := len(s.Header)
	for _, r := range s.Rows {
		if len(r) > totalCols {
			totalCols = len(r)
		}
	}
	if totalCols == 0 {
		totalCols = 1
	}
	lastCol := colName(totalCols - 1)
	totalRows := len(s.Rows)
	if len(s.Header) > 0 {
		totalRows++
	}
	if s.TitleRow != "" {
		totalRows++
	}
	b.WriteString(fmt.Sprintf(`<dimension ref="A1:%s%d"/>`, lastCol, maxInt(totalRows, 1)))

	if s.Freeze {
		split := "1"
		if s.TitleRow != "" {
			split = "2"
		}
		b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="` + split + `" topLeftCell="A` + fmt.Sprint(atoiSafe(split)+1) + `" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	}
	b.WriteString(`<sheetFormatPr defaultRowHeight="18"/>`)

	if len(s.Widths) > 0 {
		b.WriteString("<cols>")
		for i, w := range s.Widths {
			if i >= totalCols {
				break
			}
			if w <= 0 {
				w = 18
			}
			b.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%.1f" customWidth="1"/>`, i+1, i+1, w))
		}
		b.WriteString("</cols>")
	}
	b.WriteString("<sheetData>")
	rowNum := 1
	if s.TitleRow != "" {
		b.WriteString(fmt.Sprintf(`<row r="%d" ht="26" customHeight="1">`, rowNum))
		b.WriteString(fmt.Sprintf(`<c r="A%d" s="2" t="inlineStr"><is><t>%s</t></is></c>`, rowNum, xmlEsc(s.TitleRow)))
		b.WriteString("</row>")
		rowNum++
	}
	if len(s.Header) > 0 {
		b.WriteString(fmt.Sprintf(`<row r="%d" ht="24" customHeight="1">`, rowNum))
		for i, h := range s.Header {
			ref := colName(i) + fmt.Sprint(rowNum)
			b.WriteString(fmt.Sprintf(`<c r="%s" s="1" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlEsc(h)))
		}
		b.WriteString("</row>")
		rowNum++
	}
	for _, r := range s.Rows {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, rowNum))
		for i, cell := range r {
			if i >= 64 {
				break
			}
			ref := colName(i) + fmt.Sprint(rowNum)
			style := 3
			if i == 0 {
				style = 4
			}
			if cell == "" {
				continue
			}
			if isNumeric(cell) && len(cell) < 15 {
				b.WriteString(fmt.Sprintf(`<c r="%s" s="%d"><v>%s</v></c>`, ref, style, cell))
			} else {
				b.WriteString(fmt.Sprintf(`<c r="%s" s="%d" t="inlineStr"><is><t>%s</t></is></c>`, ref, style, xmlEsc(cell)))
			}
		}
		b.WriteString("</row>")
		rowNum++
	}
	b.WriteString("</sheetData>")
	if s.AutoFilter && totalRows > 0 && len(s.Header) > 0 {
		start := 1
		if s.TitleRow != "" {
			start = 2
		}
		b.WriteString(fmt.Sprintf(`<autoFilter ref="A%d:%s%d"/>`, start, lastCol, totalRows))
	}
	b.WriteString(`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>`)
	b.WriteString("</worksheet>")
	return b.String()
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	dot := false
	for i, r := range s {
		if r == '-' && i == 0 {
			continue
		}
		if r == '.' && !dot {
			dot = true
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func colName(idx int) string {
	name := ""
	for idx >= 0 {
		name = string(rune('A'+idx%26)) + name
		idx = idx/26 - 1
	}
	return name
}

func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
