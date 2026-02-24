package cv

import (
	"strings"
	"testing"

	"job-scorer/config"
)

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
			input:    "Résumé – Zürich\nΚαλημέρα",
			expected: "Résumé – Zürich\nΚαλημέρα",
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

	// Debug test
	testText := "Marketing and business development experience"
	result := reader.isValidText(testText)
	t.Logf("Testing text: %q, result: %t", testText, result)
	t.Logf("Text length: %d", len(testText))

	// Debug the keyword matching
	expectedKeywords := []string{"experience", "education", "skills", "work", "job", "company", "university", "degree", "marketing", "business", "development", "operations", "administration"}
	lowerText := strings.ToLower(testText)
	keywordCount := 0
	for _, keyword := range expectedKeywords {
		if strings.Contains(lowerText, keyword) {
			keywordCount++
			t.Logf("Found keyword: %s", keyword)
		}
	}
	t.Logf("Total keywords found: %d", keywordCount)

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
