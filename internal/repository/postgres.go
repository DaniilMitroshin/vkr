package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vkr/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Postgres struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return err
		}
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", file, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1) ON CONFLICT DO NOTHING`, version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func migrationVersion(file string) (int64, error) {
	base := filepath.Base(file)
	prefix := strings.SplitN(base, "_", 2)[0]
	return strconv.ParseInt(prefix, 10, 64)
}

func (p *Postgres) ImportStudents(ctx context.Context, rows []domain.StudentImportRow) (int, error) {
	count := 0
	for _, row := range rows {
		if _, err := p.UpsertStudent(ctx, row.FullName, row.GroupCode); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (p *Postgres) UpsertStudent(ctx context.Context, fullName, groupCode string) (domain.Student, error) {
	var s domain.Student
	err := p.pool.QueryRow(ctx, `
		INSERT INTO students(full_name, group_code)
		VALUES ($1, $2)
		ON CONFLICT (full_name, group_code)
		DO UPDATE SET updated_at=now()
		RETURNING id, full_name, group_code, telegram_id, created_at, updated_at
	`, fullName, groupCode).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (p *Postgres) RegisterStudent(ctx context.Context, telegramID int64, fullName, groupCode string) (domain.Student, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Student{}, err
	}
	defer rollback(ctx, tx)

	var s domain.Student
	err = tx.QueryRow(ctx, `
		SELECT id, full_name, group_code, telegram_id, created_at, updated_at
		FROM students
		WHERE lower(full_name)=lower($1) AND group_code=$2
		FOR UPDATE
	`, fullName, groupCode).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Student{}, ErrNotFound
	}
	if err != nil {
		return domain.Student{}, err
	}
	if s.TelegramID != nil && *s.TelegramID != telegramID {
		return domain.Student{}, fmt.Errorf("student is already linked to another telegram account")
	}
	err = tx.QueryRow(ctx, `
		UPDATE students
		SET telegram_id=$1, updated_at=now()
		WHERE id=$2
		RETURNING id, full_name, group_code, telegram_id, created_at, updated_at
	`, telegramID, s.ID).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return domain.Student{}, err
	}
	return s, tx.Commit(ctx)
}

func (p *Postgres) StudentByTelegram(ctx context.Context, telegramID int64) (domain.Student, error) {
	var s domain.Student
	err := p.pool.QueryRow(ctx, `
		SELECT id, full_name, group_code, telegram_id, created_at, updated_at
		FROM students WHERE telegram_id=$1
	`, telegramID).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Student{}, ErrNotFound
	}
	return s, err
}

func (p *Postgres) StudentByID(ctx context.Context, id int64) (domain.Student, error) {
	var s domain.Student
	err := p.pool.QueryRow(ctx, `
		SELECT id, full_name, group_code, telegram_id, created_at, updated_at
		FROM students WHERE id=$1
	`, id).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Student{}, ErrNotFound
	}
	return s, err
}

func (p *Postgres) ListStudents(ctx context.Context, limit int) ([]domain.Student, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `
		SELECT id, full_name, group_code, telegram_id, created_at, updated_at
		FROM students
		ORDER BY group_code, full_name
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStudents(rows)
}

func (p *Postgres) ImportChoices(ctx context.Context, rows []domain.ChoiceImportRow) (int, error) {
	grouped := make(map[string][]domain.ChoiceImportRow)
	for _, row := range rows {
		grouped[row.ChoiceCode] = append(grouped[row.ChoiceCode], row)
	}
	for _, choiceRows := range grouped {
		if err := p.upsertChoice(ctx, choiceRows); err != nil {
			return 0, err
		}
	}
	return len(grouped), nil
}

func (p *Postgres) upsertChoice(ctx context.Context, rows []domain.ChoiceImportRow) error {
	if len(rows) == 0 {
		return nil
	}
	first := rows[0]
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var choiceID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO choices(code, title, type, deadline, min_selected, max_selected)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (code) DO UPDATE
		SET title=excluded.title, type=excluded.type, deadline=excluded.deadline,
		    min_selected=excluded.min_selected, max_selected=excluded.max_selected
		RETURNING id
	`, first.ChoiceCode, first.ChoiceTitle, first.ChoiceType, first.Deadline, first.MinSelected, first.MaxSelected).Scan(&choiceID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM choice_groups WHERE choice_id=$1`, choiceID); err != nil {
		return err
	}
	for _, group := range first.GroupCodes {
		if _, err = tx.Exec(ctx, `INSERT INTO choice_groups(choice_id, group_code) VALUES ($1, $2) ON CONFLICT DO NOTHING`, choiceID, group); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if _, err = tx.Exec(ctx, `
			INSERT INTO choice_options(choice_id, title, seats_limit, credits, info_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (choice_id, title) DO UPDATE
			SET seats_limit=excluded.seats_limit, credits=excluded.credits, info_url=excluded.info_url
		`, choiceID, row.OptionTitle, row.SeatsLimit, row.Credits, row.InfoURL); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListChoices(ctx context.Context) ([]domain.Choice, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, code, title, type, deadline, min_selected, max_selected, created_at
		FROM choices ORDER BY deadline, code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices, err := scanChoices(rows)
	if err != nil {
		return nil, err
	}
	for i := range choices {
		choices[i].GroupCodes, err = p.choiceGroups(ctx, choices[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return choices, nil
}

func (p *Postgres) ChoicesForStudent(ctx context.Context, studentID int64) ([]domain.Choice, error) {
	student, err := p.StudentByID(ctx, studentID)
	if err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT c.id, c.code, c.title, c.type, c.deadline, c.min_selected, c.max_selected, c.created_at
		FROM choices c
		JOIN choice_groups cg ON cg.choice_id=c.id
		WHERE cg.group_code=$1
		ORDER BY c.deadline, c.code
	`, student.GroupCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices, err := scanChoices(rows)
	if err != nil {
		return nil, err
	}
	for i := range choices {
		choices[i].GroupCodes, err = p.choiceGroups(ctx, choices[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return choices, nil
}

func (p *Postgres) ChoiceByCode(ctx context.Context, code string) (domain.Choice, error) {
	var c domain.Choice
	err := p.pool.QueryRow(ctx, `
		SELECT id, code, title, type, deadline, min_selected, max_selected, created_at
		FROM choices WHERE code=$1
	`, code).Scan(&c.ID, &c.Code, &c.Title, &c.Type, &c.Deadline, &c.MinSelected, &c.MaxSelected, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Choice{}, ErrNotFound
	}
	if err != nil {
		return domain.Choice{}, err
	}
	c.GroupCodes, err = p.choiceGroups(ctx, c.ID)
	return c, err
}

func (p *Postgres) OptionsByChoiceCode(ctx context.Context, code string) ([]domain.ChoiceOption, error) {
	choice, err := p.ChoiceByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return p.optionsByChoiceID(ctx, choice.ID)
}

func (p *Postgres) optionsByChoiceID(ctx context.Context, choiceID int64) ([]domain.ChoiceOption, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT o.id, o.choice_id, o.title, o.seats_limit, o.credits, o.info_url,
		       COUNT(e.id)::int AS occupied
		FROM choice_options o
		LEFT JOIN enrollments e ON e.option_id=o.id
		WHERE o.choice_id=$1
		GROUP BY o.id
		ORDER BY o.title
	`, choiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOptions(rows)
}

func (p *Postgres) ReplaceStudentChoiceEnrollments(ctx context.Context, studentID int64, choiceCode string, optionIDs []int64, source string, enforceDeadline bool) ([]domain.Enrollment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(ctx, tx)

	student, choice, err := p.lockStudentChoice(ctx, tx, studentID, choiceCode)
	if err != nil {
		return nil, err
	}
	if enforceDeadline && time.Now().After(choice.Deadline) {
		return nil, fmt.Errorf("deadline has passed")
	}
	if !contains(choice.GroupCodes, student.GroupCode) {
		return nil, fmt.Errorf("choice is not available for group %s", student.GroupCode)
	}
	options, err := p.lockOptions(ctx, tx, choice.ID, optionIDs)
	if err != nil {
		return nil, err
	}
	if err = ValidateSubmission(choice, options); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM enrollments e
		USING choice_options o
		WHERE e.option_id=o.id AND e.student_id=$1 AND o.choice_id=$2
	`, studentID, choice.ID); err != nil {
		return nil, err
	}
	for _, option := range options {
		var occupied int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM enrollments WHERE option_id=$1`, option.ID).Scan(&occupied); err != nil {
			return nil, err
		}
		if occupied >= option.SeatsLimit {
			return nil, fmt.Errorf("no seats left for %q", option.Title)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO enrollments(student_id, option_id, source) VALUES ($1, $2, $3)`, studentID, option.ID, source); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return p.EnrollmentsForStudent(ctx, studentID)
}

func (p *Postgres) AutoAssignRequired(ctx context.Context, choiceCode string) (int, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(ctx, tx)

	choice, err := p.choiceByCodeTx(ctx, tx, choiceCode, true)
	if err != nil {
		return 0, err
	}
	if choice.Type != domain.ChoiceTypeRequiredChoice {
		return 0, fmt.Errorf("auto assignment is available only for required_choice")
	}
	studentRows, err := tx.Query(ctx, `
		SELECT s.id, s.full_name, s.group_code, s.telegram_id, s.created_at, s.updated_at
		FROM students s
		JOIN choice_groups cg ON cg.group_code=s.group_code
		WHERE cg.choice_id=$1
		  AND NOT EXISTS (
		    SELECT 1
		    FROM enrollments e
		    JOIN choice_options o ON o.id=e.option_id
		    WHERE e.student_id=s.id AND o.choice_id=$1
		  )
		ORDER BY s.group_code, s.full_name
	`, choice.ID)
	if err != nil {
		return 0, err
	}
	students, err := scanStudents(studentRows)
	if err != nil {
		return 0, err
	}

	options, err := p.optionsByChoiceIDTx(ctx, tx, choice.ID, true)
	if err != nil {
		return 0, err
	}
	assigned := 0
	for _, student := range students {
		sort.SliceStable(options, func(i, j int) bool {
			if options[i].Occupied == options[j].Occupied {
				return options[i].ID < options[j].ID
			}
			return options[i].Occupied < options[j].Occupied
		})
		for i := range options {
			if options[i].Occupied >= options[i].SeatsLimit {
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO enrollments(student_id, option_id, source) VALUES ($1, $2, 'auto')`, student.ID, options[i].ID); err != nil {
				return 0, err
			}
			options[i].Occupied++
			assigned++
			break
		}
	}
	return assigned, tx.Commit(ctx)
}

func (p *Postgres) EnrollmentsForStudent(ctx context.Context, studentID int64) ([]domain.Enrollment, error) {
	rows, err := p.pool.Query(ctx, enrollmentSelect()+` WHERE s.id=$1 ORDER BY c.code, o.title`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEnrollments(rows)
}

func (p *Postgres) AllEnrollments(ctx context.Context) ([]domain.Enrollment, error) {
	rows, err := p.pool.Query(ctx, enrollmentSelect()+` ORDER BY c.code, o.title, s.group_code, s.full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEnrollments(rows)
}

func (p *Postgres) choiceGroups(ctx context.Context, choiceID int64) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT group_code FROM choice_groups WHERE choice_id=$1 ORDER BY group_code`, choiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (p *Postgres) lockStudentChoice(ctx context.Context, tx pgx.Tx, studentID int64, choiceCode string) (domain.Student, domain.Choice, error) {
	var s domain.Student
	err := tx.QueryRow(ctx, `
		SELECT id, full_name, group_code, telegram_id, created_at, updated_at
		FROM students WHERE id=$1 FOR UPDATE
	`, studentID).Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Student{}, domain.Choice{}, ErrNotFound
	}
	if err != nil {
		return domain.Student{}, domain.Choice{}, err
	}
	choice, err := p.choiceByCodeTx(ctx, tx, choiceCode, true)
	return s, choice, err
}

func (p *Postgres) choiceByCodeTx(ctx context.Context, tx pgx.Tx, code string, lock bool) (domain.Choice, error) {
	sql := `
		SELECT id, code, title, type, deadline, min_selected, max_selected, created_at
		FROM choices WHERE code=$1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var c domain.Choice
	err := tx.QueryRow(ctx, sql, code).Scan(&c.ID, &c.Code, &c.Title, &c.Type, &c.Deadline, &c.MinSelected, &c.MaxSelected, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Choice{}, ErrNotFound
	}
	if err != nil {
		return domain.Choice{}, err
	}
	rows, err := tx.Query(ctx, `SELECT group_code FROM choice_groups WHERE choice_id=$1 ORDER BY group_code`, c.ID)
	if err != nil {
		return domain.Choice{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return domain.Choice{}, err
		}
		c.GroupCodes = append(c.GroupCodes, group)
	}
	return c, rows.Err()
}

func (p *Postgres) lockOptions(ctx context.Context, tx pgx.Tx, choiceID int64, optionIDs []int64) ([]domain.ChoiceOption, error) {
	if len(optionIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id, choice_id, title, seats_limit, credits, info_url
		FROM choice_options
		WHERE choice_id=$1 AND id=ANY($2)
		FOR UPDATE
	`, choiceID, optionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []domain.ChoiceOption
	for rows.Next() {
		var o domain.ChoiceOption
		if err := rows.Scan(&o.ID, &o.ChoiceID, &o.Title, &o.SeatsLimit, &o.Credits, &o.InfoURL); err != nil {
			return nil, err
		}
		options = append(options, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(options) != len(uniqueInt64(optionIDs)) {
		return nil, fmt.Errorf("one or more options do not belong to choice")
	}
	return options, nil
}

func (p *Postgres) optionsByChoiceIDTx(ctx context.Context, tx pgx.Tx, choiceID int64, lock bool) ([]domain.ChoiceOption, error) {
	if !lock {
		rows, err := tx.Query(ctx, `
			SELECT o.id, o.choice_id, o.title, o.seats_limit, o.credits, o.info_url,
			       COUNT(e.id)::int AS occupied
			FROM choice_options o
			LEFT JOIN enrollments e ON e.option_id=o.id
			WHERE o.choice_id=$1
			GROUP BY o.id
		`, choiceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanOptions(rows)
	}
	sql := `
		SELECT id, choice_id, title, seats_limit, credits, info_url
		FROM choice_options
		WHERE choice_id=$1`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, sql, choiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChoiceOption
	for rows.Next() {
		var o domain.ChoiceOption
		if err := rows.Scan(&o.ID, &o.ChoiceID, &o.Title, &o.SeatsLimit, &o.Credits, &o.InfoURL); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range result {
		if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM enrollments WHERE option_id=$1`, result[i].ID).Scan(&result[i].Occupied); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func ValidateSubmission(choice domain.Choice, options []domain.ChoiceOption) error {
	count := len(options)
	switch choice.Type {
	case domain.ChoiceTypeMobility:
		credits := 0
		for _, option := range options {
			credits += option.Credits
		}
		if credits < choice.MinSelected || credits > choice.MaxSelected {
			return fmt.Errorf("mobility choice requires %d-%d credits, got %d", choice.MinSelected, choice.MaxSelected, credits)
		}
	case domain.ChoiceTypeElective, domain.ChoiceTypeRequiredChoice:
		if count < choice.MinSelected || count > choice.MaxSelected {
			return fmt.Errorf("choice requires %d-%d options, got %d", choice.MinSelected, choice.MaxSelected, count)
		}
	default:
		return fmt.Errorf("unknown choice type %q", choice.Type)
	}
	return nil
}

func scanStudents(rows pgx.Rows) ([]domain.Student, error) {
	defer rows.Close()
	var result []domain.Student
	for rows.Next() {
		var s domain.Student
		if err := rows.Scan(&s.ID, &s.FullName, &s.GroupCode, &s.TelegramID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func scanChoices(rows pgx.Rows) ([]domain.Choice, error) {
	var result []domain.Choice
	for rows.Next() {
		var c domain.Choice
		if err := rows.Scan(&c.ID, &c.Code, &c.Title, &c.Type, &c.Deadline, &c.MinSelected, &c.MaxSelected, &c.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func scanOptions(rows pgx.Rows) ([]domain.ChoiceOption, error) {
	var result []domain.ChoiceOption
	for rows.Next() {
		var o domain.ChoiceOption
		if err := rows.Scan(&o.ID, &o.ChoiceID, &o.Title, &o.SeatsLimit, &o.Credits, &o.InfoURL, &o.Occupied); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func scanEnrollments(rows pgx.Rows) ([]domain.Enrollment, error) {
	var result []domain.Enrollment
	for rows.Next() {
		var e domain.Enrollment
		if err := rows.Scan(
			&e.ID, &e.Source, &e.CreatedAt,
			&e.Student.ID, &e.Student.FullName, &e.Student.GroupCode, &e.Student.TelegramID, &e.Student.CreatedAt, &e.Student.UpdatedAt,
			&e.Choice.ID, &e.Choice.Code, &e.Choice.Title, &e.Choice.Type, &e.Choice.Deadline, &e.Choice.MinSelected, &e.Choice.MaxSelected, &e.Choice.CreatedAt,
			&e.Option.ID, &e.Option.ChoiceID, &e.Option.Title, &e.Option.SeatsLimit, &e.Option.Credits, &e.Option.InfoURL, &e.Option.Occupied,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func enrollmentSelect() string {
	return `
		SELECT e.id, e.source, e.created_at,
		       s.id, s.full_name, s.group_code, s.telegram_id, s.created_at, s.updated_at,
		       c.id, c.code, c.title, c.type, c.deadline, c.min_selected, c.max_selected, c.created_at,
		       o.id, o.choice_id, o.title, o.seats_limit, o.credits, o.info_url,
		       (SELECT COUNT(*)::int FROM enrollments oe WHERE oe.option_id=o.id) AS occupied
		FROM enrollments e
		JOIN students s ON s.id=e.student_id
		JOIN choice_options o ON o.id=e.option_id
		JOIN choices c ON c.id=o.choice_id`
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func uniqueInt64(values []int64) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
