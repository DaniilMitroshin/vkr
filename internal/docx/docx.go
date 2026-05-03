package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
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
	groups := groupEnrollments(enrollments)
	if len(groups) == 0 {
		return fmt.Sprintf(documentTemplate, headerAndEmpty(student, now))
	}
	var body strings.Builder
	for i, group := range groups {
		if i > 0 {
			body.WriteString(pageBreak())
		}
		body.WriteString(statementPage(student, group, now))
	}
	return fmt.Sprintf(documentTemplate, body.String())
}

func statementPage(student domain.Student, group statementGroup, now time.Time) string {
	switch group.Choice.Type {
	case domain.ChoiceTypeMobility:
		return mobilityPage(student, group, now)
	case domain.ChoiceTypeElective:
		return electivePage(student, group, now)
	default:
		return requiredPage(student, group, now)
	}
}

type statementGroup struct {
	Choice      domain.Choice
	Enrollments []domain.Enrollment
}

func groupEnrollments(enrollments []domain.Enrollment) []statementGroup {
	byChoice := make(map[int64]*statementGroup)
	var order []int64
	for _, e := range enrollments {
		group, ok := byChoice[e.Choice.ID]
		if !ok {
			group = &statementGroup{Choice: e.Choice}
			byChoice[e.Choice.ID] = group
			order = append(order, e.Choice.ID)
		}
		group.Enrollments = append(group.Enrollments, e)
	}
	sort.Slice(order, func(i, j int) bool {
		return byChoice[order[i]].Choice.Code < byChoice[order[j]].Choice.Code
	})
	result := make([]statementGroup, 0, len(order))
	for _, id := range order {
		group := byChoice[id]
		sort.Slice(group.Enrollments, func(i, j int) bool {
			return group.Enrollments[i].Option.Title < group.Enrollments[j].Option.Title
		})
		result = append(result, *group)
	}
	return result
}

func requiredPage(student domain.Student, group statementGroup, now time.Time) string {
	var b strings.Builder
	b.WriteString(topBlock("Приложение 2", student, group.Choice))
	b.WriteString(spacer(4))
	b.WriteString(paragraph("ЗАЯВЛЕНИЕ", p{Bold: true, Align: "center", Size: 32}))
	b.WriteString(spacer(1))
	b.WriteString(paragraph("Прошу утвердить в качестве элективного модуля профильной направленности следующие дисциплины:", p{FirstLine: 720, Size: 28}))
	b.WriteString(requiredTable(group.Enrollments))
	b.WriteString(spacer(4))
	b.WriteString(signatureLine(now, student.FullName))
	return b.String()
}

func electivePage(student domain.Student, group statementGroup, now time.Time) string {
	var b strings.Builder
	b.WriteString(topBlock("Приложение 3", student, group.Choice))
	b.WriteString(spacer(4))
	b.WriteString(paragraph("ЗАЯВЛЕНИЕ", p{Bold: true, Align: "center", Size: 32}))
	b.WriteString(spacer(1))
	b.WriteString(paragraph("Прошу утвердить в качестве факультативного модуля следующие дисциплины:", p{FirstLine: 720, Size: 28}))
	b.WriteString(simpleDisciplineTable(group.Enrollments, false))
	b.WriteString(spacer(4))
	b.WriteString(signatureLine(now, student.FullName))
	b.WriteString(spacer(2))
	b.WriteString(paragraph("До меня доведена информация о том, что теперь эти факультативные дисциплины являются обязательными для изучения, несдача зачетов по ним влечет возникновение академической задолженности.", p{FirstLine: 720, Size: 28}))
	b.WriteString(spacer(3))
	b.WriteString(signatureLine(now, student.FullName))
	return b.String()
}

func mobilityPage(student domain.Student, group statementGroup, now time.Time) string {
	var b strings.Builder
	b.WriteString(topBlock("Приложение 5", student, group.Choice))
	b.WriteString(spacer(4))
	b.WriteString(paragraph("ЗАЯВЛЕНИЕ", p{Bold: true, Align: "center", Size: 32}))
	b.WriteString(spacer(1))
	b.WriteString(paragraph("Прошу разрешить включить в модуль мобильности следующие дисциплины:", p{FirstLine: 720, Size: 28}))
	b.WriteString(simpleDisciplineTable(group.Enrollments, true))
	b.WriteString(spacer(1))
	b.WriteString(paragraph("Все дисциплины входят в пул дисциплин модуля мобильности для моего направления обучения.", p{FirstLine: 720, Size: 28}))
	b.WriteString(spacer(2))
	b.WriteString(signatureLine(now, student.FullName))
	b.WriteString(spacer(2))
	b.WriteString(paragraph("До меня доведена информация о том, что в случае отсутствия или несвоевременного предоставления документа, подтверждающего оценку результатов обучения, у меня возникнет академическая задолженность.", p{FirstLine: 720, Size: 28}))
	b.WriteString(spacer(3))
	b.WriteString(signatureLine(now, student.FullName))
	return b.String()
}

func headerAndEmpty(student domain.Student, now time.Time) string {
	return topBlock("Заявление", student, domain.Choice{}) +
		spacer(5) +
		paragraph("ЗАЯВЛЕНИЕ", p{Bold: true, Align: "center", Size: 32}) +
		paragraph("Выбранных дисциплин пока нет.", p{Size: 28}) +
		spacer(4) +
		signatureLine(now, student.FullName)
}

func topBlock(app string, student domain.Student, choice domain.Choice) string {
	program := valueOrLine(choice.ProgramName)
	head := valueOrLine(choice.ProgramHead)
	return paragraph(app, p{Align: "right", Size: 28}) +
		paragraph("Руководителю ОП "+program, p{IndentLeft: 5200, Size: 28}) +
		paragraph("Наименование ООП", p{IndentLeft: 7600, Size: 16}) +
		paragraph(head, p{IndentLeft: 5200, Size: 24}) +
		paragraph("ФИО руководителя", p{IndentLeft: 7600, Size: 16}) +
		paragraph("студента группы "+student.GroupCode, p{IndentLeft: 5200, Size: 28}) +
		paragraph(student.FullName, p{IndentLeft: 5200, Size: 24}) +
		paragraph("(ФИО студента полностью)", p{IndentLeft: 7000, Size: 16})
}

func requiredTable(enrollments []domain.Enrollment) string {
	rows := [][]cell{
		{{Text: "Наименование дисциплины", Width: 5200}, {Text: "Трудоемкость,\nз.е.", Width: 1850}, {Text: "Период\nизучения\n(семестр)", Width: 1700}, {Text: "Отметка\nо выборе", Width: 1550}},
	}
	for _, e := range enrollments {
		rows = append(rows, []cell{
			{Text: e.Option.Title, Width: 5200, Italic: true},
			{Text: creditsText(e.Option.Credits), Width: 1850, Align: "center"},
			{Text: optionSemester(e.Option), Width: 1700, Align: "center"},
			{Text: "выбрано", Width: 1550, Align: "center"},
		})
	}
	return table(rows)
}

func simpleDisciplineTable(enrollments []domain.Enrollment, withLink bool) string {
	if withLink {
		rows := [][]cell{{{Text: "Наименование дисциплины", Width: 3300}, {Text: "Трудоемкость, з.е.", Width: 1850}, {Text: "Период\nизучения\n(семестр)", Width: 1700}, {Text: "Ссылка на курс\n(МООК-курс / дисциплинарный\nмодуль с электронным ресурсом)", Width: 3400}}}
		total := 0
		for _, e := range enrollments {
			total += e.Option.Credits
			rows = append(rows, []cell{{Text: e.Option.Title, Width: 3300}, {Text: creditsText(e.Option.Credits), Width: 1850, Align: "center"}, {Text: optionSemester(e.Option), Width: 1700, Align: "center"}, {Text: valueOrLine(e.Option.InfoURL), Width: 3400}})
		}
		rows = append(rows, []cell{{Text: "ИТОГО", Width: 3300, Bold: true}, {Text: creditsText(total), Width: 1850, Align: "center"}, {Text: "", Width: 1700}, {Text: "", Width: 3400}})
		return table(rows)
	}
	rows := [][]cell{{{Text: "Наименование дисциплины", Width: 6500}, {Text: "Трудоемкость,\nз.е.", Width: 1850}, {Text: "Период\nизучения\n(семестр)", Width: 1700}}}
	for _, e := range enrollments {
		rows = append(rows, []cell{{Text: e.Option.Title, Width: 6500}, {Text: creditsText(e.Option.Credits), Width: 1850, Align: "center"}, {Text: optionSemester(e.Option), Width: 1700, Align: "center"}})
	}
	return table(rows)
}

func signatureLine(now time.Time, fullName string) string {
	return table([][]cell{
		{{Text: now.Format("02.01.2006"), Width: 2500, Align: "center", TopBorder: true}, {Text: "", Width: 700, NoBorder: true}, {Text: "", Width: 2800, Align: "center", TopBorder: true}, {Text: "", Width: 700, NoBorder: true}, {Text: fullName, Width: 3300, Align: "center", TopBorder: true}},
		{{Text: "(дата)", Width: 2500, Align: "center", NoBorder: true}, {Text: "", Width: 700, NoBorder: true}, {Text: "(подпись)", Width: 2800, Align: "center", NoBorder: true}, {Text: "", Width: 700, NoBorder: true}, {Text: "(ФИО)", Width: 3300, Align: "center", NoBorder: true}},
	})
}

type p struct {
	Bold       bool
	Align      string
	Size       int
	IndentLeft int
	FirstLine  int
}

func paragraph(text string, props p) string {
	size := props.Size
	if size == 0 {
		size = 24
	}
	var ppr strings.Builder
	if props.Align != "" {
		ppr.WriteString(`<w:jc w:val="` + props.Align + `"/>`)
	}
	if props.IndentLeft != 0 || props.FirstLine != 0 {
		ppr.WriteString(fmt.Sprintf(`<w:ind w:left="%d" w:firstLine="%d"/>`, props.IndentLeft, props.FirstLine))
	}
	runProps := fmt.Sprintf(`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="%d"/>`, size)
	if props.Bold {
		runProps += `<w:b/>`
	}
	runProps += `</w:rPr>`
	lines := strings.Split(text, "\n")
	var runs strings.Builder
	for i, line := range lines {
		if i > 0 {
			runs.WriteString(`<w:br/>`)
		}
		runs.WriteString(`<w:t xml:space="preserve">` + xmlEscape(line) + `</w:t>`)
	}
	return `<w:p><w:pPr>` + ppr.String() + `</w:pPr><w:r>` + runProps + runs.String() + `</w:r></w:p>`
}

type cell struct {
	Text      string
	Width     int
	Bold      bool
	Italic    bool
	Align     string
	NoBorder  bool
	TopBorder bool
}

func table(rows [][]cell) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblLook w:val="04A0"/></w:tblPr>`)
	for _, row := range rows {
		b.WriteString(`<w:tr>`)
		for _, c := range row {
			b.WriteString(tableCell(c))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

func tableCell(c cell) string {
	borders := `<w:tcBorders><w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/><w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/></w:tcBorders>`
	if c.NoBorder {
		borders = `<w:tcBorders><w:top w:val="nil"/><w:left w:val="nil"/><w:bottom w:val="nil"/><w:right w:val="nil"/></w:tcBorders>`
	}
	if c.TopBorder {
		borders = `<w:tcBorders><w:top w:val="single" w:sz="4"/><w:left w:val="nil"/><w:bottom w:val="nil"/><w:right w:val="nil"/></w:tcBorders>`
	}
	runProps := `<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="24"/>`
	if c.Bold {
		runProps += `<w:b/>`
	}
	if c.Italic {
		runProps += `<w:i/>`
	}
	runProps += `</w:rPr>`
	align := c.Align
	if align == "" {
		align = "left"
	}
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr><w:p><w:pPr><w:jc w:val="%s"/></w:pPr><w:r>%s<w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, c.Width, borders, align, runProps, xmlEscape(c.Text))
}

func spacer(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString(`<w:p/>`)
	}
	return b.String()
}

func pageBreak() string {
	return `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`
}

func creditsText(credits int) string {
	if credits <= 0 {
		return ""
	}
	return fmt.Sprintf("%d з.е.", credits)
}

func optionSemester(option domain.ChoiceOption) string {
	if strings.TrimSpace(option.Semester) != "" {
		return option.Semester
	}
	return ""
}

func valueOrLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "____________________________"
	}
	return value
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
    <w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="850"/></w:sectPr>
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
