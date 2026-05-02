package domain

import "time"

type ChoiceType string

const (
	ChoiceTypeElective       ChoiceType = "elective"
	ChoiceTypeRequiredChoice ChoiceType = "required_choice"
	ChoiceTypeMobility       ChoiceType = "mobility"
)

type Student struct {
	ID         int64     `json:"id"`
	FullName   string    `json:"full_name"`
	GroupCode  string    `json:"group_code"`
	TelegramID *int64    `json:"telegram_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Choice struct {
	ID          int64      `json:"id"`
	Code        string     `json:"code"`
	Title       string     `json:"title"`
	Type        ChoiceType `json:"type"`
	GroupCodes  []string   `json:"group_codes"`
	Deadline    time.Time  `json:"deadline"`
	MinSelected int        `json:"min_selected"`
	MaxSelected int        `json:"max_selected"`
	CreatedAt   time.Time  `json:"created_at"`
}

type ChoiceOption struct {
	ID         int64  `json:"id"`
	ChoiceID   int64  `json:"choice_id"`
	Title      string `json:"title"`
	SeatsLimit int    `json:"seats_limit"`
	Credits    int    `json:"credits"`
	InfoURL    string `json:"info_url,omitempty"`
	Occupied   int    `json:"occupied"`
}

type Enrollment struct {
	ID        int64        `json:"id"`
	Student   Student      `json:"student"`
	Choice    Choice       `json:"choice"`
	Option    ChoiceOption `json:"option"`
	Source    string       `json:"source"`
	CreatedAt time.Time    `json:"created_at"`
}

type StudentImportRow struct {
	FullName  string `json:"full_name"`
	GroupCode string `json:"group_code"`
}

type ChoiceImportRow struct {
	ChoiceCode  string     `json:"choice_code"`
	ChoiceTitle string     `json:"choice_title"`
	ChoiceType  ChoiceType `json:"choice_type"`
	GroupCodes  []string   `json:"group_codes"`
	Deadline    time.Time  `json:"deadline"`
	MinSelected int        `json:"min_selected"`
	MaxSelected int        `json:"max_selected"`
	OptionTitle string     `json:"option_title"`
	SeatsLimit  int        `json:"seats_limit"`
	Credits     int        `json:"credits"`
	InfoURL     string     `json:"info_url"`
}
