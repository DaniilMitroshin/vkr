package repository

import (
	"testing"

	"vkr/internal/domain"
)

func TestValidateSubmissionRequiredChoice(t *testing.T) {
	choice := domain.Choice{Type: domain.ChoiceTypeRequiredChoice, MinSelected: 1, MaxSelected: 1}
	if err := ValidateSubmission(choice, []domain.ChoiceOption{{ID: 1}}); err != nil {
		t.Fatalf("expected valid submission: %v", err)
	}
	if err := ValidateSubmission(choice, nil); err == nil {
		t.Fatal("expected empty submission to fail")
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
