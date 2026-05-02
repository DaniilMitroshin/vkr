package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"time"

	"vkr/internal/domain"
)

func BuildApplication(student domain.Student, enrollments []domain.Enrollment, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":          contentTypes,
		"_rels/.rels":                  rels,
		"word/_rels/document.xml.rels": documentRels,
		"word/styles.xml":              styles,
		"docProps/core.xml":            fmt.Sprintf(coreProps, xmlEscape(now.Format(time.RFC3339))),
		"docProps/app.xml":             appProps,
		"word/document.xml":            documentXML(student, enrollments, now),
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func documentXML(student domain.Student, enrollments []domain.Enrollment, now time.Time) string {
	body := paragraph("Заявление", true, "center")
	body += paragraph("Прошу записать меня на выбранные элективные, факультативные дисциплины и модули мобильности.", false, "")
	body += paragraph("ФИО: "+student.FullName, false, "")
	body += paragraph("Группа: "+student.GroupCode, false, "")
	body += paragraph("Дата формирования: "+now.Format("02.01.2006"), false, "")
	body += paragraph("Выбранные дисциплины:", true, "")
	if len(enrollments) == 0 {
		body += paragraph("Выбранных дисциплин нет.", false, "")
	}
	for _, e := range enrollments {
		line := fmt.Sprintf("%s: %s", e.Choice.Title, e.Option.Title)
		if e.Option.Credits > 0 {
			line += fmt.Sprintf(" (%d з.е.)", e.Option.Credits)
		}
		body += paragraph(line, false, "")
	}
	body += paragraph("Подпись обучающегося: __________________ / "+student.FullName+" /", false, "")
	return fmt.Sprintf(documentTemplate, body)
}

func paragraph(text string, bold bool, align string) string {
	props := ""
	if align != "" {
		props = `<w:pPr><w:jc w:val="` + align + `"/></w:pPr>`
	}
	runProps := ""
	if bold {
		runProps = `<w:rPr><w:b/></w:rPr>`
	}
	return `<w:p>` + props + `<w:r>` + runProps + `<w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r></w:p>`
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

const documentTemplate = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    %s
    <w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="1134"/></w:sectPr>
  </w:body>
</w:document>`

const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const rels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const documentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`

const styles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
</w:styles>`

const coreProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/">
  <dc:title>Заявление о выборе дисциплин</dc:title>
  <dc:creator>VKR Choice Bot</dc:creator>
  <dcterms:created xsi:type="dcterms:W3CDTF" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">%s</dcterms:created>
</cp:coreProperties>`

const appProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>VKR Choice Bot</Application></Properties>`
