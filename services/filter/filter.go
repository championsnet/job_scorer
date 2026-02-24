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
	unwantedWordsInTitle []string
	logger            *utils.Logger
	languageDetector  lingua.LanguageDetector
}

func NewFilter(logger *utils.Logger) *Filter {
	unwantedLocations := []string{
		"EMEA", "DACH", "Switzerland (Remote)", "Europe", "EU",
	}

	unwantedWordsInTitle := []string{
		"Head", "Senior", "Director", "Sr.",
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
		unwantedWordsInTitle: unwantedWordsInTitle,
		logger:            logger,
		languageDetector:  detector,
	}
}

func (f *Filter) PrefilterJobs(jobs []*models.Job) []*models.Job {
	var filteredJobs []*models.Job
	
	for _, job := range jobs {
		if f.shouldIncludeJob(job) {
			filteredJobs = append(filteredJobs, job)
		}
	}
	
	return filteredJobs
}

func (f *Filter) shouldIncludeJob(job *models.Job) bool {
	// Check for unwanted locations
	for _, unwantedLocation := range f.unwantedLocations {
		if strings.Contains(strings.ToLower(job.Location), strings.ToLower(unwantedLocation)) {
			return false
		}
	}

	// Check for unwanted words in title
	for _, unwantedWord := range f.unwantedWordsInTitle {
		if strings.Contains(strings.ToLower(job.Position), strings.ToLower(unwantedWord)) {
			return false
		}
	}
	
	// Check if job title is in German
	if !f.isEnglishText(job.Position) {
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

	// Multi-layer language detection
	return f.enhancedLanguageDetection(cleanedText)
}

// Enhanced language detection with multiple validation layers
func (f *Filter) enhancedLanguageDetection(text string) bool {
	textLower := strings.ToLower(text)
	
	// Layer 1: Check for obvious German requirements (immediate rejection)
	germanRequirementKeywords := []string{
		"deutsch erforderlich", "german required", "german fluency", "fluent german",
		"muttersprache deutsch", "german native", "deutschkenntnisse", 
		"fließend deutsch", "fluent in german", "german language skills",
		"deutsch als muttersprache", "german c1", "german c2", "deutsch c1", "deutsch c2",
		"deutsche sprachkenntnisse", "verhandlungssicher deutsch", "german proficiency",
	}
	
	for _, keyword := range germanRequirementKeywords {
		if strings.Contains(textLower, keyword) {
			f.logger.Debug("Rejected due to German requirement keyword: %s", keyword)
			return false
		}
	}
	
	// Layer 2: Check for German job posting indicators
	germanJobKeywords := []string{
		"stellenausschreibung", "arbeitsplatz", "bewerbung", "lebenslauf",
		"anstellung", "mitarbeiter", "unternehmen", "gesellschaft",
		"verantwortung", "aufgaben", "anforderungen", "qualifikation",
		"berufserfahrung", "abschluss", "studium", "ausbildung",
	}
	
	germanKeywordCount := 0
	for _, keyword := range germanJobKeywords {
		if strings.Contains(textLower, keyword) {
			germanKeywordCount++
		}
	}
	
	// If we find 2+ German job keywords, likely German posting
	if germanKeywordCount >= 2 {
		f.logger.Debug("Rejected due to %d German job keywords", germanKeywordCount)
		return false
	}
	
	// Layer 3: Language library confidence check (more lenient for edge cases)
	confidenceValues := f.languageDetector.ComputeLanguageConfidenceValues(text)
	
	var englishConfidence, germanConfidence float64
	for _, conf := range confidenceValues {
		switch conf.Language() {
		case lingua.English:
			englishConfidence = conf.Value()
		case lingua.German:
			germanConfidence = conf.Value()
		}
	}
	
	// If German confidence is significantly higher than English, reject
	if germanConfidence > 0.6 && germanConfidence > englishConfidence*1.5 {
		f.logger.Debug("Rejected due to German confidence %.2f vs English %.2f", 
			germanConfidence, englishConfidence)
		return false
	}
	
	// Layer 4: English indicators (positive signals)
	englishIndicators := []string{
		"english", "international", "multicultural", "global", "worldwide",
		"european", "eu", "remote", "hybrid", "flexible", "agile",
		"you will", "we are", "join our", "opportunity", "experience",
		"skills", "requirements", "responsibilities", "benefits",
	}
	
	englishIndicatorCount := 0
	for _, indicator := range englishIndicators {
		if strings.Contains(textLower, indicator) {
			englishIndicatorCount++
		}
	}
	
	// If we have English indicators and reasonable confidence, accept
	if englishIndicatorCount >= 2 && englishConfidence >= 0.15 {
		f.logger.Debug("Accepted due to %d English indicators and %.2f confidence", 
			englishIndicatorCount, englishConfidence)
		return true
	}
	
	// Default: accept if English confidence is reasonable
	englishThreshold := 0.25 // Slightly higher than before for better accuracy
	result := englishConfidence >= englishThreshold
	
	f.logger.Debug("Language detection result: %t (English: %.2f, German: %.2f)", 
		result, englishConfidence, germanConfidence)
	
	return result
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
	seenJobIDs := make(map[string]bool) // Track jobs we've already seen
	var duplicateCount int
	
	for _, job := range jobs {
		if job.ShouldNotify() {
			// Skip if we've already seen this job ID
			if job.JobID != "" && seenJobIDs[job.JobID] {
				duplicateCount++
				f.logger.Debug("⚠️ Skipping duplicate job in notifications: %s at %s (ID: %s)", 
					job.Position, job.Company, job.JobID)
				continue
			}
			
			// Mark this job ID as seen and add to notifications
			if job.JobID != "" {
				seenJobIDs[job.JobID] = true
			}
			notificationJobs = append(notificationJobs, job)
		}
	}
	
	if duplicateCount > 0 {
		f.logger.Warning("⚠️ Removed %d duplicate jobs from notifications", duplicateCount)
	}
	
	f.logger.Info("Found %d unique jobs that should trigger notifications", len(notificationJobs))
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