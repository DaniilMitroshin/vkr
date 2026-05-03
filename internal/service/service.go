package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vkr/internal/docx"
	"vkr/internal/domain"
	"vkr/internal/importer"
)

type Store interface {
	ImportStudents(context.Context, []domain.StudentImportRow) (int, error)
	ImportChoices(context.Context, []domain.ChoiceImportRow) (int, error)
	RegisterStudent(context.Context, int64, string, string) (domain.Student, error)
	StudentByTelegram(context.Context, int64) (domain.Student, error)
	StudentByID(context.Context, int64) (domain.Student, error)
	ListStudents(context.Context, int) ([]domain.Student, error)
	AllStudents(context.Context) ([]domain.Student, error)
	RegisteredStudentsWithEnrollments(context.Context) ([]domain.StudentWithEnrollments, error)
	ListChoices(context.Context) ([]domain.Choice, error)
	ChoicesForStudent(context.Context, int64) ([]domain.Choice, error)
	ChoiceByCode(context.Context, string) (domain.Choice, error)
	OptionsByChoiceCode(context.Context, string) ([]domain.ChoiceOption, error)
	ReplaceStudentChoiceEnrollments(context.Context, int64, string, []int64, string, bool) ([]domain.Enrollment, error)
	AutoAssignRequired(context.Context, string) (int, error)
	EnrollmentsForStudent(context.Context, int64) ([]domain.Enrollment, error)
	AllEnrollments(context.Context) ([]domain.Enrollment, error)
	SeedAdmins(context.Context, map[int64]struct{}) error
	IsAdmin(context.Context, int64) (bool, error)
	AddAdmin(context.Context, int64, int64) error
	RemoveAdmin(context.Context, int64) error
	ListAdmins(context.Context) ([]int64, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) ImportStudentsFile(ctx context.Context, filename string, data []byte) (int, error) {
	rows, err := importer.ParseStudents(filename, data)
	if err != nil {
		return 0, err
	}
	return s.store.ImportStudents(ctx, rows)
}

func (s *Service) ImportChoicesFile(ctx context.Context, filename string, data []byte) (int, error) {
	rows, err := importer.ParseChoices(filename, data)
	if err != nil {
		return 0, err
	}
	return s.store.ImportChoices(ctx, rows)
}

func (s *Service) RegisterStudent(ctx context.Context, telegramID int64, fullName, groupCode string) (domain.Student, error) {
	return s.store.RegisterStudent(ctx, telegramID, fullName, normalizeGroup(groupCode))
}

func (s *Service) CurrentStudent(ctx context.Context, telegramID int64) (domain.Student, error) {
	return s.store.StudentByTelegram(ctx, telegramID)
}

func (s *Service) StudentChoices(ctx context.Context, studentID int64) ([]domain.Choice, error) {
	return s.store.ChoicesForStudent(ctx, studentID)
}

func (s *Service) SubmitStudentChoice(ctx context.Context, studentID int64, choiceCode string, optionIDs []int64) ([]domain.Enrollment, error) {
	return s.store.ReplaceStudentChoiceEnrollments(ctx, studentID, choiceCode, optionIDs, "student", true)
}

func (s *Service) AdminSubmitChoice(ctx context.Context, studentID int64, choiceCode string, optionIDs []int64) ([]domain.Enrollment, error) {
	return s.store.ReplaceStudentChoiceEnrollments(ctx, studentID, choiceCode, optionIDs, "manual", false)
}

func (s *Service) SeedAdmins(ctx context.Context, ids map[int64]struct{}) error {
	return s.store.SeedAdmins(ctx, ids)
}

func (s *Service) IsAdmin(ctx context.Context, telegramID int64) bool {
	ok, err := s.store.IsAdmin(ctx, telegramID)
	return err == nil && ok
}

func (s *Service) AddAdmin(ctx context.Context, telegramID, addedBy int64) error {
	return s.store.AddAdmin(ctx, telegramID, addedBy)
}

func (s *Service) RemoveAdmin(ctx context.Context, telegramID int64) error {
	return s.store.RemoveAdmin(ctx, telegramID)
}

func (s *Service) ListAdmins(ctx context.Context) ([]int64, error) {
	return s.store.ListAdmins(ctx)
}

func (s *Service) ApplicationDocx(ctx context.Context, studentID int64) ([]byte, error) {
	student, err := s.store.StudentByID(ctx, studentID)
	if err != nil {
		return nil, err
	}
	enrollments, err := s.store.EnrollmentsForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return docx.BuildApplication(student, enrollments, s.now())
}

func (s *Service) ExportResultsJSON(ctx context.Context) ([]byte, error) {
	enrollments, err := s.store.AllEnrollments(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(enrollments, "", "  ")
}

func (s *Service) ExportResultsCSV(ctx context.Context) ([]byte, error) {
	enrollments, err := s.store.AllEnrollments(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"choice_code", "choice_title", "option_title", "student_full_name", "group_code", "credits", "source", "created_at"})
	for _, e := range enrollments {
		_ = w.Write([]string{
			e.Choice.Code,
			e.Choice.Title,
			e.Option.Title,
			e.Student.FullName,
			e.Student.GroupCode,
			strconv.Itoa(e.Option.Credits),
			e.Source,
			e.CreatedAt.Format(time.RFC3339),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Service) ExportStudentsJSON(ctx context.Context) ([]byte, error) {
	students, err := s.store.AllStudents(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(students, "", "  ")
}

func (s *Service) ExportStudentsCSV(ctx context.Context) ([]byte, error) {
	students, err := s.store.AllStudents(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"id", "full_name", "group_code", "telegram_id", "created_at", "updated_at"})
	for _, student := range students {
		telegramID := ""
		if student.TelegramID != nil {
			telegramID = strconv.FormatInt(*student.TelegramID, 10)
		}
		_ = w.Write([]string{
			strconv.FormatInt(student.ID, 10),
			student.FullName,
			student.GroupCode,
			telegramID,
			student.CreatedAt.Format(time.RFC3339),
			student.UpdatedAt.Format(time.RFC3339),
		})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func (s *Service) ExportRegisteredJSON(ctx context.Context) ([]byte, error) {
	students, err := s.store.RegisteredStudentsWithEnrollments(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(students, "", "  ")
}

func (s *Service) ExportRegisteredCSV(ctx context.Context) ([]byte, error) {
	students, err := s.store.RegisteredStudentsWithEnrollments(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"student_id", "full_name", "group_code", "telegram_id", "choice_code", "choice_title", "choice_type", "option_title", "credits", "source", "enrolled_at"})
	for _, item := range students {
		telegramID := ""
		if item.Student.TelegramID != nil {
			telegramID = strconv.FormatInt(*item.Student.TelegramID, 10)
		}
		if len(item.Enrollments) == 0 {
			_ = w.Write([]string{strconv.FormatInt(item.Student.ID, 10), item.Student.FullName, item.Student.GroupCode, telegramID, "", "", "", "", "", "", ""})
			continue
		}
		for _, e := range item.Enrollments {
			_ = w.Write([]string{
				strconv.FormatInt(item.Student.ID, 10),
				item.Student.FullName,
				item.Student.GroupCode,
				telegramID,
				e.Choice.Code,
				e.Choice.Title,
				string(e.Choice.Type),
				e.Option.Title,
				strconv.Itoa(e.Option.Credits),
				e.Source,
				e.CreatedAt.Format(time.RFC3339),
			})
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func (s *Service) ListStudents(ctx context.Context, limit int) ([]domain.Student, error) {
	return s.store.ListStudents(ctx, limit)
}

func (s *Service) ListChoices(ctx context.Context) ([]domain.Choice, error) {
	return s.store.ListChoices(ctx)
}

func (s *Service) Choice(ctx context.Context, code string) (domain.Choice, error) {
	return s.store.ChoiceByCode(ctx, code)
}

func (s *Service) ChoiceOptions(ctx context.Context, code string) ([]domain.ChoiceOption, error) {
	return s.store.OptionsByChoiceCode(ctx, code)
}

func (s *Service) AutoAssignRequired(ctx context.Context, code string) (int, error) {
	if code == "" {
		return 0, fmt.Errorf("choice code is required")
	}
	return s.store.AutoAssignRequired(ctx, code)
}

func (s *Service) EnrollmentsForStudent(ctx context.Context, studentID int64) ([]domain.Enrollment, error) {
	return s.store.EnrollmentsForStudent(ctx, studentID)
}

func normalizeGroup(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] == '/' {
		return raw
	}
	return "/" + raw
}
