package models

import (
	"testing"
	"time"
)

func TestNewJob(t *testing.T) {
	tests := []struct {
		name        string
		position    string
		company     string
		location    string
		date        string
		salary      string
		jobURL      string
		companyLogo string
		agoTime     string
		wantErr     bool
	}{
		{
			name:        "Valid job creation",
			position:    "Software Engineer",
			company:     "Tech Corp",
			location:    "Basel, Switzerland",
			date:        "2024-01-01",
			salary:      "80000 CHF",
			jobURL:      "https://linkedin.com/jobs/123",
			companyLogo: "https://example.com/logo.png",
			agoTime:     "2 hours ago",
			wantErr:     false,
		},
		{
			name:     "Missing position",
			position: "",
			company:  "Tech Corp",
			location: "Basel",
			wantErr:  true,
		},
		{
			name:     "Missing company",
			position: "Engineer",
			company:  "",
			location: "Basel",
			wantErr:  true,
		},
		{
			name:     "Missing location",
			position: "Engineer",
			company:  "Tech Corp",
			location: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := NewJob(tt.position, tt.company, tt.location, tt.date, tt.salary, tt.jobURL, tt.companyLogo, tt.agoTime)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewJob() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("NewJob() unexpected error: %v", err)
				return
			}
			
			if job.Position != tt.position {
				t.Errorf("Position = %v, want %v", job.Position, tt.position)
			}
			if job.Company != tt.company {
				t.Errorf("Company = %v, want %v", job.Company, tt.company)
			}
			if job.Location != tt.location {
				t.Errorf("Location = %v, want %v", job.Location, tt.location)
			}
			
			// Check that CreatedAt is recent
			if time.Since(job.CreatedAt) > time.Minute {
				t.Errorf("CreatedAt should be recent, got %v", job.CreatedAt)
			}
		})
	}
}

func TestJobValidate(t *testing.T) {
	tests := []struct {
		name    string
		job     Job
		wantErr bool
	}{
		{
			name: "Valid job",
			job: Job{
				Position: "Engineer",
				Company:  "Tech Corp",
				Location: "Basel",
			},
			wantErr: false,
		},
		{
			name: "Missing position",
			job: Job{
				Company:  "Tech Corp",
				Location: "Basel",
			},
			wantErr: true,
		},
		{
			name: "Missing company",
			job: Job{
				Position: "Engineer",
				Location: "Basel",
			},
			wantErr: true,
		},
		{
			name: "Missing location",
			job: Job{
				Position: "Engineer",
				Company:  "Tech Corp",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			
			if tt.wantErr && err == nil {
				t.Errorf("Validate() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestJobIsPromising(t *testing.T) {
	tests := []struct {
		name      string
		score     *float64
		threshold float64
		want      bool
	}{
		{
			name:      "Score above threshold",
			score:     floatPtr(8.0),
			threshold: 7.0,
			want:      true,
		},
		{
			name:      "Score equal to threshold",
			score:     floatPtr(7.0),
			threshold: 7.0,
			want:      true,
		},
		{
			name:      "Score below threshold",
			score:     floatPtr(6.0),
			threshold: 7.0,
			want:      false,
		},
		{
			name:      "Nil score",
			score:     nil,
			threshold: 7.0,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{Score: tt.score}
			got := job.IsPromising(tt.threshold)
			
			if got != tt.want {
				t.Errorf("IsPromising() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobShouldNotify(t *testing.T) {
	tests := []struct {
		name            string
		shouldSendEmail bool
		finalScore      *float64
		want            bool
	}{
		{
			name:            "Should send email with score",
			shouldSendEmail: true,
			finalScore:      floatPtr(8.0),
			want:            true,
		},
		{
			name:            "Should send email but no final score",
			shouldSendEmail: true,
			finalScore:      nil,
			want:            false,
		},
		{
			name:            "Should not send email",
			shouldSendEmail: false,
			finalScore:      floatPtr(8.0),
			want:            false,
		},
		{
			name:            "Should not send email and no score",
			shouldSendEmail: false,
			finalScore:      nil,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{
				ShouldSendEmail: tt.shouldSendEmail,
				FinalScore:      tt.finalScore,
			}
			got := job.ShouldNotify()
			
			if got != tt.want {
				t.Errorf("ShouldNotify() = %v, want %v", got, tt.want)
			}
		})
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

func TestExtractJobID(t *testing.T) {
	tests := []struct {
		name     string
		jobURL   string
		expected string
	}{
		{
			name:     "Standard LinkedIn job URL",
			jobURL:   "https://www.linkedin.com/jobs/view/global-category-manager-it-digitalization-at-mettler-toledo-international-inc-4225966954/?position=9&pageNum=1&refId=%2Birz7rU71ObIIjA4omHZbQ%3D%3D&trackingId=ZQESxSnS0zgkvAuNhQs4vw%3D%3D&originalSubdomain=ch",
			expected: "4225966954",
		},
		{
			name:     "LinkedIn job URL without query parameters",
			jobURL:   "https://www.linkedin.com/jobs/view/software-engineer-at-google-inc-1234567890/",
			expected: "1234567890",
		},
		{
			name:     "LinkedIn job URL with different format",
			jobURL:   "https://linkedin.com/jobs/view/senior-developer-at-microsoft-9876543210",
			expected: "9876543210",
		},
		{
			name:     "Empty URL",
			jobURL:   "",
			expected: "",
		},
		{
			name:     "Non-LinkedIn URL",
			jobURL:   "https://indeed.com/jobs/12345",
			expected: "12345",
		},
		{
			name:     "URL without job ID",
			jobURL:   "https://www.linkedin.com/jobs/view/software-engineer-at-google-inc/",
			expected: "",
		},
		{
			name:     "LinkedIn job URL without query parameters",
			jobURL:   "https://www.linkedin.com/jobs/view/software-engineer-at-google-80-100-inc-1234567890/",
			expected: "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractJobID(tt.jobURL)
			if result != tt.expected {
				t.Errorf("ExtractJobID(%q) = %q, want %q", tt.jobURL, result, tt.expected)
			}
		})
	}
}

func TestNewJobWithJobID(t *testing.T) {
	jobURL := "https://www.linkedin.com/jobs/view/global-category-manager-it-digitalization-at-mettler-toledo-international-inc-4225966954/?position=9&pageNum=1&refId=%2Birz7rU71ObIIjA4omHZbQ%3D%3D&trackingId=ZQESxSnS0zgkvAuNhQs4vw%3D%3D&originalSubdomain=ch"
	
	job, err := NewJob("Test Position", "Test Company", "Test Location", "2025-01-01", "100k", jobURL, "logo.png", "2 hours ago")
	if err != nil {
		t.Fatalf("NewJob failed: %v", err)
	}
	
	if job.JobID != "4225966954" {
		t.Errorf("Expected JobID to be '4225966954', got '%s'", job.JobID)
	}
	
	if job.JobURL != jobURL {
		t.Errorf("Expected JobURL to be preserved, got '%s'", job.JobURL)
	}
} 