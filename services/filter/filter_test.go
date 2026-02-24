package filter

import (
	"testing"

	"job-scorer/config"
	"job-scorer/models"
	"job-scorer/utils"
)

func testPolicies() (config.FilterPolicy, config.NotificationPolicy) {
	return config.FilterPolicy{
		UnwantedLocations: []string{"EMEA", "DACH", "Switzerland (Remote)", "Europe", "EU"},
		UnwantedWordsInTitle: []string{"Head", "Senior", "Director", "Sr."},
		PrimaryLanguage:    "english",
		DetectionLanguages: []string{"english", "german", "french"},
		RedFlagLanguageKeywords: []string{
			"deutsch erforderlich", "german required", "german fluency", "fluent german",
		},
		NonPrimaryLanguageKeywords: []string{
			"stellenausschreibung", "arbeitsplatz", "bewerbung", "lebenslauf",
			"anstellung", "mitarbeiter", "unternehmen", "gesellschaft",
		},
		PrimaryLanguageIndicators: []string{
			"english", "international", "global", "you will", "we are", "skills", "requirements",
		},
		NonPrimaryKeywordMinCount:       2,
		NonPrimaryDominanceThreshold:    0.60,
		NonPrimaryDominanceRatio:        1.50,
		PrimaryIndicatorMinCount:        2,
		PrimaryIndicatorMinConfidence:   0.40,
		DefaultPrimaryThreshold:         0.40,
		PrimaryVsNonPrimaryMinDelta:     0.05,
		MinTextLengthForLanguageDetect: 8,
	}, config.NotificationPolicy{
		MinFinalScore:          0,
		RequireShouldSendEmail: true,
		RequireFinalScore:      true,
	}
}

func TestNewFilter(t *testing.T) {
	logger := utils.NewLogger("Test")
	filterPolicy, notificationPolicy := testPolicies()
	filter := NewFilter(filterPolicy, notificationPolicy, logger)
	if filter == nil {
		t.Errorf("NewFilter() returned nil")
	}
}

func TestFilter_PrefilterJobs(t *testing.T) {
	logger := utils.NewLogger("Test")
	filterPolicy, notificationPolicy := testPolicies()
	filter := NewFilter(filterPolicy, notificationPolicy, logger)
	
	// Create test jobs
	jobs := []*models.Job{
		{
			Position: "Software Engineer",
			Company:  "Tech Corp",
			Location: "Basel, Switzerland",
		},
		{
			Position: "Entwickler", // German word
			Company:  "German Corp",
			Location: "Zurich, Switzerland",
		},
		{
			Position: "Marketing Manager",
			Company:  "Marketing Corp",
			Location: "EMEA", // Unwanted location
		},
		{
			Position: "Data Scientist",
			Company:  "AI Corp",
			Location: "Basel, Switzerland",
		},
	}
	
	filtered := filter.PrefilterJobs(jobs)
	
	// Should filter out German job and EMEA location job
	expectedCount := 2
	if len(filtered) != expectedCount {
		t.Errorf("PrefilterJobs() returned %d jobs, want %d", len(filtered), expectedCount)
	}
	
	// Check that the right jobs were kept
	expectedPositions := []string{"Software Engineer", "Data Scientist"}
	for i, expectedPos := range expectedPositions {
		if i >= len(filtered) {
			t.Errorf("PrefilterJobs() missing expected position: %s", expectedPos)
			continue
		}
		if filtered[i].Position != expectedPos {
			t.Errorf("PrefilterJobs() position[%d] = %s, want %s", i, filtered[i].Position, expectedPos)
		}
	}
}

func TestFilter_FilterPromisingJobs(t *testing.T) {
	logger := utils.NewLogger("Test")
	filterPolicy, notificationPolicy := testPolicies()
	filter := NewFilter(filterPolicy, notificationPolicy, logger)
	
	score1 := 8.0
	score2 := 5.0
	score3 := 9.0
	
	jobs := []*models.Job{
		{
			Position:  "Software Engineer",
			Company:   "Tech Corp",
			Location:  "Basel, Switzerland",
			Score:     &score1,
		},
		{
			Position:  "Data Scientist",
			Company:   "AI Corp",
			Location:  "Zurich, Switzerland",
			Score:     &score2,
		},
		{
			Position:  "Marketing Manager",
			Company:   "Marketing Corp",
			Location:  "Basel, Switzerland",
			Score:     &score3,
		},
	}
	
	// Filter with threshold 7.0
	filtered := filter.FilterPromisingJobs(jobs, 7.0)
	
	expectedCount := 2 // Only jobs with score >= 7.0
	if len(filtered) != expectedCount {
		t.Errorf("FilterPromisingJobs() returned %d jobs, want %d", len(filtered), expectedCount)
	}
	
	// Check that the right jobs were kept
	expectedPositions := []string{"Software Engineer", "Marketing Manager"}
	for i, expectedPos := range expectedPositions {
		if i >= len(filtered) {
			t.Errorf("FilterPromisingJobs() missing expected position: %s", expectedPos)
			continue
		}
		if filtered[i].Position != expectedPos {
			t.Errorf("FilterPromisingJobs() position[%d] = %s, want %s", i, filtered[i].Position, expectedPos)
		}
	}
}

func TestFilter_FilterNotificationJobs(t *testing.T) {
	logger := utils.NewLogger("Test")
	filterPolicy, notificationPolicy := testPolicies()
	filter := NewFilter(filterPolicy, notificationPolicy, logger)
	
	score1 := 8.0
	score2 := 5.0
	
	jobs := []*models.Job{
		{
			Position:        "Software Engineer",
			Company:         "Tech Corp",
			Location:        "Basel, Switzerland",
			Score:           &score1,
			FinalScore:      &score1,
			ShouldSendEmail: true,
		},
		{
			Position:        "Data Scientist",
			Company:         "AI Corp",
			Location:        "Zurich, Switzerland",
			Score:           &score2,
			FinalScore:      &score2,
			ShouldSendEmail: false,
		},
		{
			Position:        "Marketing Manager",
			Company:         "Marketing Corp",
			Location:        "Basel, Switzerland",
			Score:           &score1,
			FinalScore:      &score1,
			ShouldSendEmail: true,
		},
	}
	
	filtered := filter.FilterNotificationJobs(jobs)
	
	expectedCount := 2 // Only jobs with ShouldSendEmail = true
	if len(filtered) != expectedCount {
		t.Errorf("FilterNotificationJobs() returned %d jobs, want %d", len(filtered), expectedCount)
	}
	
	// Check that the right jobs were kept
	expectedPositions := []string{"Software Engineer", "Marketing Manager"}
	for i, expectedPos := range expectedPositions {
		if i >= len(filtered) {
			t.Errorf("FilterNotificationJobs() missing expected position: %s", expectedPos)
			continue
		}
		if filtered[i].Position != expectedPos {
			t.Errorf("FilterNotificationJobs() position[%d] = %s, want %s", i, filtered[i].Position, expectedPos)
		}
	}
}

func TestFilter_FilterJobDescription(t *testing.T) {
	logger := utils.NewLogger("Test")
	filterPolicy, notificationPolicy := testPolicies()
	filter := NewFilter(filterPolicy, notificationPolicy, logger)
	
	tests := []struct {
		name        string
		description string
		want        bool
	}{
		{
			name:        "English description",
			description: "We are looking for a software engineer with Python experience",
			want:        true,
		},
		{
			name:        "German description",
			description: "Wir suchen einen Softwareentwickler mit Python-Erfahrung",
			want:        false,
		},
		{
			name:        "Empty description",
			description: "",
			want:        true,
		},
		{
			name:        "Mixed language",
			description: "Software Engineer mit Python experience",
			want:        true, // Language detector may classify as English due to majority English content
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &models.Job{
				Position:       "Test Position",
				Company:        "Test Company",
				Location:       "Test Location",
				JobDescription: tt.description,
			}
			
			result := filter.FilterJobDescription(job)
			if result != tt.want {
				t.Errorf("FilterJobDescription() = %v, want %v for description: %s", result, tt.want, tt.description)
			}
		})
	}
} 