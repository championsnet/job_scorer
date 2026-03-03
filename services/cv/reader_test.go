package cv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"job-scorer/config"
)

func TestLoadCV_FromMarkdownFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cvPath := filepath.Join(dir, "candidate.md")
	content := "# Candidate\n\nExperienced in growth, operations, and strategy."
	if err := os.WriteFile(cvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create markdown CV: %v", err)
	}

	reader := NewCVReader(cvPath, config.CVPolicy{
		FallbackText:       "fallback cv",
		MinValidTextLength: 10,
		MinLetterRatio:     0.3,
	})

	text, err := reader.LoadCV()
	if err != nil {
		t.Fatalf("LoadCV returned error: %v", err)
	}

	if text == "" {
		t.Fatal("expected CV text to be loaded from markdown file")
	}
	if text == "fallback cv" {
		t.Fatal("expected markdown content, got fallback CV")
	}
}

func TestLoadCV_FromMarkdownFileUsesFallbackWhenInvalid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cvPath := filepath.Join(dir, "candidate.md")
	if err := os.WriteFile(cvPath, []byte("##"), 0644); err != nil {
		t.Fatalf("failed to create markdown CV: %v", err)
	}

	reader := NewCVReader(cvPath, config.CVPolicy{
		FallbackText:       "fallback cv",
		MinValidTextLength: 10,
		MinLetterRatio:     0.3,
	})

	text, err := reader.LoadCV()
	if err != nil {
		t.Fatalf("LoadCV returned error: %v", err)
	}
	if text != "fallback cv" {
		t.Fatalf("expected fallback CV, got %q", text)
	}
}

func TestCleanText(t *testing.T) {
	reader := &CVReader{
		policy: config.CVPolicy{MinValidTextLength: 40, MinLetterRatio: 0.40},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "  Hello   World  \n\n\nTest  ",
			expected: "Hello   World  \n\nTest",
		},
		{
			input:    "Normal text with\n\n\nmultiple\n\n\nnewlines",
			expected: "Normal text with\n\nmultiple\n\nnewlines",
		},
		{
			input:    "Text with \x00\x01\x02 control chars",
			expected: "Text with  control chars",
		},
		{
			input:    "Resume - Zurich\nKalimera",
			expected: "Resume - Zurich\nKalimera",
		},
	}

	for _, test := range tests {
		result := reader.cleanText(test.input)
		if result != test.expected {
			t.Errorf("cleanText(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestIsValidText(t *testing.T) {
	reader := &CVReader{
		policy: config.CVPolicy{MinValidTextLength: 40, MinLetterRatio: 0.40},
	}

	tests := []struct {
		input    string
		expected bool
	}{
		{
			input:    "This is a valid CV with experience and education mentioned",
			expected: true,
		},
		{
			input:    "Marketing and business development experience",
			expected: true,
		},
		{
			input:    "Short text",
			expected: false,
		},
		{
			input:    "Garbled text: !:»³ùäýýB\"",
			expected: false,
		},
		{
			input:    "%%%% #### **** !!!! ???? //// ---- ++++ ==== ____ @@@@",
			expected: false,
		},
	}

	for _, test := range tests {
		result := reader.isValidText(test.input)
		if result != test.expected {
			t.Errorf("isValidText(%q) = %t, want %t", test.input, result, test.expected)
		}
	}
}

func TestGetFallbackCV(t *testing.T) {
	reader := &CVReader{
		policy: config.CVPolicy{
			FallbackText: "Fallback CV text with skills and experience details for testing purposes.",
		},
	}
	cv := reader.getFallbackCV()

	if !strings.Contains(cv, "Fallback CV text") {
		t.Errorf("unexpected fallback CV: %s", cv)
	}

	if len(cv) < 20 {
		t.Errorf("Fallback CV too short: %d characters", len(cv))
	}
}
