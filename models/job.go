package models

import (
	"fmt"
	"regexp"
	"strings"
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

	// Strip query parameters and fragments
	cleanURL := strings.SplitN(jobURL, "?", 2)[0]
	cleanURL = strings.SplitN(cleanURL, "#", 2)[0]

	// Capture all numeric sequences and return the last one
	re := regexp.MustCompile(`\d+`)
	numbers := re.FindAllString(cleanURL, -1)
	if len(numbers) == 0 {
		return ""
	}
	return numbers[len(numbers)-1]
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