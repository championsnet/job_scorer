package evaluator

import (
	"job-scorer/utils"
	"testing"
)

func TestParseFinalEvaluationResponse(t *testing.T) {
	evaluator := &Evaluator{
		logger: utils.NewLogger("TestEvaluator"),
	}
	
	tests := []struct {
		response        string
		expectedScore   *float64
		expectedReason  string
		expectedReasons []string
	}{
		{
			response: `Final Score: 8
Reason: This is a great match`,
			expectedScore:   float64Ptr(8.0),
			expectedReason:  "This is a great match",
			expectedReasons: []string{"This is a great match"},
		},
		{
			response: `Final Score: 4
Reason: Not a good fit`,
			expectedScore:   float64Ptr(4.0),
			expectedReason:  "Not a good fit",
			expectedReasons: []string{"Not a good fit"},
		},
		{
			response: `Final Score: 7
Reason: Good opportunity`,
			expectedScore:   float64Ptr(7.0),
			expectedReason:  "Good opportunity",
			expectedReasons: []string{"Good opportunity"},
		},
		{
			response: `Final Score: 9
Reason: Perfect match`,
			expectedScore:   float64Ptr(9.0),
			expectedReason:  "Perfect match",
			expectedReasons: []string{"Perfect match"},
		},
		{
			response: `{
  "finalScore": 8,
  "reasons": ["Great match for skills", "Good company culture", "Opportunity for growth"]
}`,
			expectedScore:   float64Ptr(8.0),
			expectedReason:  "Great match for skills",
			expectedReasons: []string{"Great match for skills", "Good company culture", "Opportunity for growth"},
		},
	}
	
	for i, test := range tests {
		score, reason, reasons := evaluator.parseFinalEvaluationResponse(test.response)
		
		if !float64Equal(score, test.expectedScore) {
			t.Errorf("Test %d: Expected score %v, got %v", i, test.expectedScore, score)
		}
		
		if reason != test.expectedReason {
			t.Errorf("Test %d: Expected reason %q, got %q", i, test.expectedReason, reason)
		}
		
		if len(reasons) != len(test.expectedReasons) {
			t.Errorf("Test %d: Expected %d reasons, got %d", i, len(test.expectedReasons), len(reasons))
		} else {
			for j, r := range reasons {
				if j < len(test.expectedReasons) && r != test.expectedReasons[j] {
					t.Errorf("Test %d: Expected reason %d to be %q, got %q", i, j, test.expectedReasons[j], r)
				}
			}
		}
	}
}

func TestParseEvaluationResponseWithReasonsArray(t *testing.T) {
	logger := utils.NewLogger("TestEvaluator")
	
	// Test the specific JSON format that was failing in the logs
	jsonResponse := `{
  "score": 2,
  "recommend": false,
  "reasons": [
    "Role is IT Project Management, not aligned with candidate's marketing/BD/Ops/Admin skills.",
    "Contract position may not offer the growth and stability sought by the candidate."
  ]
}`

	evaluator := &Evaluator{
		logger: logger,
	}

	score, reason, reasons := evaluator.parseEvaluationResponse(jsonResponse)

	// Verify score
	if score == nil {
		t.Fatal("Expected score to be parsed, got nil")
	}
	if *score != 2.0 {
		t.Errorf("Expected score 2.0, got %.1f", *score)
	}

	// Verify reasons array
	expectedReasons := []string{
		"Role is IT Project Management, not aligned with candidate's marketing/BD/Ops/Admin skills.",
		"Contract position may not offer the growth and stability sought by the candidate.",
	}
	
	if len(reasons) != len(expectedReasons) {
		t.Fatalf("Expected %d reasons, got %d: %v", len(expectedReasons), len(reasons), reasons)
	}

	for i, expectedReason := range expectedReasons {
		if reasons[i] != expectedReason {
			t.Errorf("Expected reason[%d] = %s, got %s", i, expectedReason, reasons[i])
		}
	}

	// Verify backward compatibility reason (should be first reason)
	if reason != expectedReasons[0] {
		t.Errorf("Expected reason = %s, got %s", expectedReasons[0], reason)
	}
}

func TestParseEvaluationResponseWithMarkdownCodeBlock(t *testing.T) {
	logger := utils.NewLogger("TestEvaluator")
	
	// Test the specific format that was failing in the logs - JSON wrapped in markdown code blocks
	jsonResponseWithMarkdown := "```json\n{\n  \"score\": 7,\n  \"recommend\": true,\n  \"reasons\": [\n    \"Role appears to be in Business Development, a relevant field.\",\n    \"Company is a stable multinational corporation.\",\n    \"Location is commutable from Basel/Zurich.\"\n  ]\n}\n```"

	evaluator := &Evaluator{
		logger: logger,
	}

	score, reason, reasons := evaluator.parseEvaluationResponse(jsonResponseWithMarkdown)

	// Verify score
	if score == nil {
		t.Fatal("Expected score to be parsed, got nil")
	}
	if *score != 7.0 {
		t.Errorf("Expected score 7.0, got %.1f", *score)
	}

	// Verify reasons array
	expectedReasons := []string{
		"Role appears to be in Business Development, a relevant field.",
		"Company is a stable multinational corporation.",
		"Location is commutable from Basel/Zurich.",
	}
	
	if len(reasons) != len(expectedReasons) {
		t.Fatalf("Expected %d reasons, got %d: %v", len(expectedReasons), len(reasons), reasons)
	}

	for i, expectedReason := range expectedReasons {
		if reasons[i] != expectedReason {
			t.Errorf("Expected reason[%d] = %s, got %s", i, expectedReason, reasons[i])
		}
	}

	// Verify backward compatibility reason (should be first reason)
	if reason != expectedReasons[0] {
		t.Errorf("Expected reason = %s, got %s", expectedReasons[0], reason)
	}
}

func TestParseFinalEvaluationResponseWithMarkdownCodeBlock(t *testing.T) {
	logger := utils.NewLogger("TestEvaluator")
	
	// Test the specific format that was failing in the logs - JSON wrapped in markdown code blocks
	jsonResponseWithMarkdown := "```json\n{\n  \"finalScore\": 7,\n  \"reasons\": [\n    \"Strong business development, marketing, and operations experience aligns well with job requirements.\",\n    \"Proven experience in client relationship management and pipeline development.\",\n    \"Fluency in English matches the language requirement.\"\n  ]\n}\n```"

	evaluator := &Evaluator{
		logger: logger,
	}

	score, reason, reasons := evaluator.parseFinalEvaluationResponse(jsonResponseWithMarkdown)

	// Verify score
	if score == nil {
		t.Fatal("Expected score to be parsed, got nil")
	}
	if *score != 7.0 {
		t.Errorf("Expected score 7.0, got %.1f", *score)
	}

	// Verify reasons array
	expectedReasons := []string{
		"Strong business development, marketing, and operations experience aligns well with job requirements.",
		"Proven experience in client relationship management and pipeline development.",
		"Fluency in English matches the language requirement.",
	}
	
	if len(reasons) != len(expectedReasons) {
		t.Fatalf("Expected %d reasons, got %d: %v", len(expectedReasons), len(reasons), reasons)
	}

	for i, expectedReason := range expectedReasons {
		if reasons[i] != expectedReason {
			t.Errorf("Expected reason[%d] = %s, got %s", i, expectedReason, reasons[i])
		}
	}

	// Verify backward compatibility reason (should be first reason)
	if reason != expectedReasons[0] {
		t.Errorf("Expected reason = %s, got %s", expectedReasons[0], reason)
	}
}

func float64Ptr(f float64) *float64 {
	return &f
}

func float64Equal(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
} 