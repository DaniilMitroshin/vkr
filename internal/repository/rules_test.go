package repository

import (
	"testing"

	"vkr/internal/domain"
)

func TestNormalizeGroupDirectionGroup(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "5130904/20101", want: "5130904/20101"},
		{in: " 5130904 / 20101 ", want: "5130904/20101"},
		{in: "/20101", want: ""},
		{in: "5130904", want: ""},
		{in: "5130904/20101/1", want: ""},
	}
	for _, tc := range tests {
		if got := NormalizeGroup(tc.in); got != tc.want {
			t.Fatalf("NormalizeGroup(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateSubmissionRequiredChoice(t *testing.T) {
	choice := domain.Choice{Type: domain.ChoiceTypeRequiredChoice, MinSelected: 1, MaxSelected: 1}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{ID: 1}}); err != nil {
		t.Fatalf("expected valid submission: %v", err)
	}
	if err := ValidateSubmission(choice, nil); err == nil {
		t.Fatal("expected empty submission to fail")
	}
}

func TestValidateSubmissionElectiveIgnoresMinMax(t *testing.T) {
	choice := domain.Choice{Type: domain.ChoiceTypeElective, MinSelected: 1, MaxSelected: 3}
	if err := ValidateSubmission(choice, nil); err != nil {
		t.Fatalf("expected empty elective submission to pass: %v", err)
	}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}); err != nil {
		t.Fatalf("expected elective submission over max_selected to pass: %v", err)
	}
}

func TestValidateSubmissionMobilityCredits(t *testing.T) {
	choice := domain.Choice{Type: domain.ChoiceTypeMobility, MinSelected: 6, MaxSelected: 9}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{Credits: 3}, {Credits: 3}}); err != nil {
		t.Fatalf("expected valid mobility submission: %v", err)
	}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{Credits: 3}}); err == nil {
		t.Fatal("expected too few credits to fail")
	}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{Credits: 6}, {Credits: 6}}); err == nil {
		t.Fatal("expected too many credits to fail")
	}
}
