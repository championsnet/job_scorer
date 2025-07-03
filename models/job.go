package models

import (
	"fmt"
	"regexp"
	"time"
)

type Job struct {
	JobID           string    `json:"jobId,omitempty"`           // LinkedIn job ID extracted from URL
	Position        string    `json:"position"`
	Company         string    `json:"company"`
	Location        string    `json:"location"`
	Date            string    `json:"date"`
	Salary          string    `json:"salary"`
	JobURL          string    `json:"jobUrl"`
	CompanyLogo     string    `json:"companyLogo"`
	AgoTime         string    `json:"agoTime"`
	Score           *float64  `json:"score,omitempty"`
	Reason          string    `json:"reason,omitempty"`       // For backward compatibility
	Reasons         []string  `json:"reasons,omitempty"`      // Array of reasons from initial evaluation
	JobDescription  string    `json:"jobDescription,omitempty"`
	FinalScore      *float64  `json:"finalScore,omitempty"`
	ShouldSendEmail bool      `json:"shouldSendEmail,omitempty"`
	FinalReason     string    `json:"finalReason,omitempty"`  // For backward compatibility
	FinalReasons    []string  `json:"finalReasons,omitempty"` // Array of reasons from final evaluation
	CreatedAt       time.Time `json:"createdAt"`
}

type JobEvaluation struct {
	Job    Job      `json:"job"`
	Score  *float64 `json:"score"`
	Reason string   `json:"reason"`
}

type FinalEvaluation struct {
	Job             Job      `json:"job"`
	FinalScore      *float64 `json:"finalScore"`
	ShouldSendEmail bool     `json:"shouldSendEmail"`
	FinalReason     string   `json:"finalReason"`
}

// ExtractJobID extracts the job ID from a LinkedIn job URL
func ExtractJobID(jobURL string) string {
	if jobURL == "" {
		return ""
	}
	
	// Pattern to match LinkedIn job IDs
	// Example: https://www.linkedin.com/jobs/view/global-category-manager-it-digitalization-at-mettler-toledo-international-inc-4225966954/
	// The ID is the last number before the query parameters
	pattern := regexp.MustCompile(`linkedin\.com/jobs/view/.*?-(\d+)`)
	matches := pattern.FindStringSubmatch(jobURL)
	
	if len(matches) > 1 {
		return matches[1]
	}
	
	// Fallback pattern for different URL formats
	pattern2 := regexp.MustCompile(`/jobs/view/.*?-(\d+)`)
	matches2 := pattern2.FindStringSubmatch(jobURL)
	
	if len(matches2) > 1 {
		return matches2[1]
	}
	
	return ""
}

func NewJob(position, company, location, date, salary, jobURL, companyLogo, agoTime string) (*Job, error) {
	if position == "" {
		return nil, fmt.Errorf("position is required")
	}
	if company == "" {
		return nil, fmt.Errorf("company is required")
	}
	if location == "" {
		return nil, fmt.Errorf("location is required")
	}

	// Extract job ID from URL
	jobID := ExtractJobID(jobURL)

	return &Job{
		JobID:       jobID,
		Position:    position,
		Company:     company,
		Location:    location,
		Date:        date,
		Salary:      salary,
		JobURL:      jobURL,
		CompanyLogo: companyLogo,
		AgoTime:     agoTime,
		CreatedAt:   time.Now(),
	}, nil
}

func (j *Job) Validate() error {
	if j.Position == "" {
		return fmt.Errorf("position is required")
	}
	if j.Company == "" {
		return fmt.Errorf("company is required")
	}
	if j.Location == "" {
		return fmt.Errorf("location is required")
	}
	return nil
}

func (j *Job) IsPromising(threshold float64) bool {
	return j.Score != nil && *j.Score >= threshold
}

func (j *Job) ShouldNotify() bool {
	return j.ShouldSendEmail && j.FinalScore != nil
} 