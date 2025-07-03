package filter

import (
	"regexp"
	"strings"

	"job-scorer/models"
	"job-scorer/utils"

	"github.com/pemistahl/lingua-go"
)

type Filter struct {
	unwantedLocations []string
	logger            *utils.Logger
	languageDetector  lingua.LanguageDetector
}

func NewFilter(logger *utils.Logger) *Filter {
	unwantedLocations := []string{
		"EMEA", "DACH", "Switzerland (Remote)", "Europe", "EU",
	}

	// Initialize language detector for German, English, and French
	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(lingua.German, lingua.English, lingua.French).
		WithMinimumRelativeDistance(0.9).
		Build()

	// Use provided logger or create a new one
	if logger == nil {
		logger = utils.NewLogger("Filter")
	}

	return &Filter{
		unwantedLocations: unwantedLocations,
		logger:            logger,
		languageDetector:  detector,
	}
}

func (f *Filter) PrefilterJobs(jobs []*models.Job) []*models.Job {
	f.logger.Info("Prefiltering %d jobs", len(jobs))
	
	var filteredJobs []*models.Job
	
	for _, job := range jobs {
		if f.shouldIncludeJob(job) {
			filteredJobs = append(filteredJobs, job)
		} else {
			f.logger.Debug("Filtered out job: %s at %s (reason: German content or unwanted location)", 
				job.Position, job.Company)
		}
	}
	
	f.logger.Info("After prefiltering: %d jobs remaining", len(filteredJobs))
	return filteredJobs
}

func (f *Filter) shouldIncludeJob(job *models.Job) bool {
	// Check for unwanted locations
	for _, unwantedLocation := range f.unwantedLocations {
		if strings.Contains(strings.ToLower(job.Location), strings.ToLower(unwantedLocation)) {
			return false
		}
	}
	
	// Check if job title is in German
	if !f.isEnglishText(job.Position) {
		f.logger.Debug("Filtered out non-English job title: %s", job.Position)
		return false
	}
	
	return true
}

func (f *Filter) isEnglishText(text string) bool {
	if strings.TrimSpace(text) == "" {
		return true // Allow empty text
	}

	cleanedText := f.cleanTextForDetection(text)
	if len(cleanedText) < 8 {
		return true // Too short to reliably detect
	}

	// Use confidence values instead of strict detection
	confidenceValues := f.languageDetector.ComputeLanguageConfidenceValues(cleanedText)
	
	// Log confidence values for debugging
	f.logger.Debug("Language confidence values for text: %s", cleanedText[:min(50, len(cleanedText))])
	for _, conf := range confidenceValues {
		f.logger.Debug("  %s: %.2f", conf.Language(), conf.Value())
	}
	
	// Check if English confidence is above threshold (20%)
	englishThreshold := 0.2 // 20%
	for _, conf := range confidenceValues {
		if conf.Language() == lingua.English && conf.Value() >= englishThreshold {
			f.logger.Debug("Text accepted as English with confidence %.2f", conf.Value())
			return true
		}
	}
	
	// If we get here, English confidence was below threshold
	f.logger.Debug("Text rejected: English confidence below threshold")
	return false
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (f *Filter) FilterPromisingJobs(jobs []*models.Job, threshold float64) []*models.Job {
	var promisingJobs []*models.Job
	
	for _, job := range jobs {
		if job.IsPromising(threshold) {
			promisingJobs = append(promisingJobs, job)
		}
	}
	
	f.logger.Info("Found %d promising jobs (score >= %.1f)", len(promisingJobs), threshold)
	return promisingJobs
}

func (f *Filter) FilterNotificationJobs(jobs []*models.Job) []*models.Job {
	var notificationJobs []*models.Job
	
	for _, job := range jobs {
		if job.ShouldNotify() {
			notificationJobs = append(notificationJobs, job)
		}
	}
	
	f.logger.Info("Found %d jobs that should trigger notifications", len(notificationJobs))
	return notificationJobs
}

// FilterJobDescription filters out jobs with German descriptions
func (f *Filter) FilterJobDescription(job *models.Job) bool {
	if job.JobDescription == "" {
		f.logger.Debug("Kept job without description through language detection: %s at %s", job.Position, job.Company)
		return true // Allow jobs without descriptions
	}

	return f.isEnglishText(job.JobDescription)
}

func (f *Filter) cleanTextForDetection(text string) string {
	// Remove common job posting artifacts that confuse language detection
	cleaned := text

	// Remove percentages, numbers in parentheses, etc.
	cleaned = regexp.MustCompile(`\(\d+%?\)`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`\d+%`).ReplaceAllString(cleaned, "")

	// Remove special characters that might confuse detection
	cleaned = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(cleaned, " ")

	// Remove extra whitespace
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
} 