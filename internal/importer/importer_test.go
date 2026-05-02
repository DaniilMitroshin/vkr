package importer

import (
	"os"
	"testing"
)

func TestParseStudentsJSONish(t *testing.T) {
	data, err := os.ReadFile("../../input/Контингент_5130904_201.json")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseStudents("students.json", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 50 {
		t.Fatalf("expected many students, got %d", len(rows))
	}
	if rows[0].FullName == "" || rows[0].GroupCode == "" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
}

func TestParseChoicesSample(t *testing.T) {
	data, err := os.ReadFile("../../input/Дисциплины_пример.json")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseChoices("choices.json", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 7 {
		t.Fatalf("expected 7 choice option rows, got %d", len(rows))
	}
	if rows[0].ChoiceCode == "" || len(rows[0].GroupCodes) == 0 {
		t.Fatalf("bad first row: %+v", rows[0])
	}
}
