package importer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vkr/internal/domain"
)

func ParseStudents(filename string, data []byte) ([]domain.StudentImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return parseStudentsCSV(data)
	case ".json":
		return parseStudentsJSON(data)
	default:
		return nil, fmt.Errorf("unsupported students file extension %q", ext)
	}
}

func ParseChoices(filename string, data []byte) ([]domain.ChoiceImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return parseChoicesCSV(data)
	case ".json":
		return parseChoicesJSON(data)
	default:
		return nil, fmt.Errorf("unsupported choices file extension %q", ext)
	}
}

func parseStudentsCSV(data []byte) ([]domain.StudentImportRow, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("empty csv")
	}
	idx := headerIndex(records[0])
	var rows []domain.StudentImportRow
	for _, rec := range records[1:] {
		fullName := field(rec, idx, "фио", "full_name")
		groupCode := field(rec, idx, "группа", "group_code")
		appendStudent(&rows, fullName, groupCode)
	}
	return rows, nil
}

func parseStudentsJSON(data []byte) ([]domain.StudentImportRow, error) {
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fixed := normalizeJSONish(data)
		if fixedErr := json.Unmarshal(fixed, &raw); fixedErr != nil {
			return nil, fmt.Errorf("parse json: %w; parse normalized json: %v", err, fixedErr)
		}
	}
	var rows []domain.StudentImportRow
	for _, item := range raw {
		fullName := anyString(item["full_name"], item["ФИО"], item["fio"])
		groupCode := anyString(item["group_code"], item["Группа"], item["group"])
		appendStudent(&rows, fullName, groupCode)
	}
	return rows, nil
}

func parseChoicesCSV(data []byte) ([]domain.ChoiceImportRow, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("empty csv")
	}
	idx := headerIndex(records[0])
	var rows []domain.ChoiceImportRow
	for _, rec := range records[1:] {
		row, err := choiceRowFromFields(func(names ...string) string { return field(rec, idx, names...) })
		if err != nil {
			return nil, err
		}
		if row.ChoiceCode != "" && row.OptionTitle != "" {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func parseChoicesJSON(data []byte) ([]domain.ChoiceImportRow, error) {
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var rows []domain.ChoiceImportRow
	for _, item := range raw {
		get := func(names ...string) string {
			values := make([]any, 0, len(names))
			for _, name := range names {
				values = append(values, item[name])
			}
			return anyString(values...)
		}
		row, err := choiceRowFromFields(get)
		if err != nil {
			return nil, err
		}
		if row.ChoiceCode != "" && row.OptionTitle != "" {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func choiceRowFromFields(get func(...string) string) (domain.ChoiceImportRow, error) {
	deadline, err := parseTime(get("deadline", "дедлайн"))
	if err != nil {
		return domain.ChoiceImportRow{}, err
	}
	return domain.ChoiceImportRow{
		ChoiceCode:  strings.TrimSpace(get("choice_code", "код_выбора")),
		ChoiceTitle: strings.TrimSpace(get("choice_title", "название_выбора")),
		ChoiceType:  domain.ChoiceType(strings.TrimSpace(get("choice_type", "тип_выбора"))),
		ProgramName: normalizeSpace(get("program_name", "ооп", "наименование_ооп")),
		ProgramHead: normalizeSpace(get("program_head", "руководитель_оп", "фио_руководителя")),
		GroupCodes:  splitList(get("group_codes", "группы")),
		Deadline:    deadline,
		MinSelected: parseInt(get("min_selected", "минимум"), 0),
		MaxSelected: parseInt(get("max_selected", "максимум"), 1),
		OptionTitle: strings.TrimSpace(get("option_title", "дисциплина")),
		SeatsLimit:  parseInt(get("seats_limit", "мест"), 0),
		Credits:     parseInt(get("credits", "зачетные_единицы"), 0),
		Semester:    normalizeSpace(get("semester", "period", "семестр", "период")),
		TeacherName: normalizeSpace(get("teacher_name", "руководитель_дисциплины", "преподаватель")),
		InfoURL:     strings.TrimSpace(get("info_url", "ссылка")),
	}, nil
}

func appendStudent(rows *[]domain.StudentImportRow, fullName, groupCode string) {
	fullName = normalizeSpace(fullName)
	groupCode = normalizeGroup(groupCode)
	if fullName == "" || groupCode == "" || strings.EqualFold(fullName, "nan") || strings.EqualFold(groupCode, "nan") {
		return
	}
	*rows = append(*rows, domain.StudentImportRow{FullName: fullName, GroupCode: groupCode})
}

func normalizeJSONish(data []byte) []byte {
	s := string(data)
	s = strings.ReplaceAll(s, ";", ",")
	s = regexp.MustCompile(`(?i)\bNaN\b`).ReplaceAllString(s, "null")
	s = regexp.MustCompile(`,\s*([}\]])`).ReplaceAllString(s, "$1")
	return []byte(s)
}

func headerIndex(header []string) map[string]int {
	idx := make(map[string]int)
	for i, name := range header {
		idx[strings.ToLower(strings.TrimSpace(name))] = i
	}
	return idx
}

func field(rec []string, idx map[string]int, names ...string) string {
	for _, name := range names {
		if i, ok := idx[strings.ToLower(name)]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
	}
	return ""
}

func anyString(values ...any) string {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case nil:
		default:
			return fmt.Sprint(v)
		}
	}
	return ""
}

func splitList(raw string) []string {
	var result []string
	for _, part := range regexp.MustCompile(`[|,;]`).Split(raw, -1) {
		part = normalizeGroup(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("deadline is required")
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02", "02.01.2006"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid deadline %q", raw)
}

func normalizeGroup(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "/" + strings.TrimLeft(raw, "/")
}

func normalizeSpace(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}
