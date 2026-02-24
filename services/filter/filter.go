package filter

import (
	"regexp"
	"strings"

	"job-scorer/config"
	"job-scorer/models"
	"job-scorer/utils"

	"github.com/pemistahl/lingua-go"
)

type Filter struct {
	unwantedLocations    []string
	unwantedWordsInTitle []string
	policy               config.FilterPolicy
	notificationPolicy   config.NotificationPolicy
	primaryLanguage      lingua.Language
	detectionLanguages   []lingua.Language
	logger               *utils.Logger
	languageDetector     lingua.LanguageDetector
}

func NewFilter(policy config.FilterPolicy, notificationPolicy config.NotificationPolicy, logger *utils.Logger) *Filter {
	detectionLanguages := toLinguaLanguages(policy.DetectionLanguages)
	if len(detectionLanguages) == 0 {
		detectionLanguages = []lingua.Language{lingua.English, lingua.German, lingua.French}
	}
	primaryLanguage := parseLanguage(policy.PrimaryLanguage)
	if primaryLanguage == lingua.Unknown {
		primaryLanguage = lingua.English
	}

	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(detectionLanguages...).
		WithMinimumRelativeDistance(0.9).
		Build()

	// Use provided logger or create a new one
	if logger == nil {
		logger = utils.NewLogger("Filter")
	}

	return &Filter{
		unwantedLocations:    policy.UnwantedLocations,
		unwantedWordsInTitle: policy.UnwantedWordsInTitle,
		policy:               policy,
		notificationPolicy:   notificationPolicy,
		primaryLanguage:      primaryLanguage,
		detectionLanguages:   detectionLanguages,
		logger:               logger,
		languageDetector:     detector,
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

	// Check if job title is in configured primary language.
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
	if len(cleanedText) < f.policy.MinTextLengthForLanguageDetect {
		return true // Too short to reliably detect
	}

	// Multi-layer language detection
	return f.enhancedLanguageDetection(cleanedText)
}

// Enhanced language detection with multiple validation layers
func (f *Filter) enhancedLanguageDetection(text string) bool {
	textLower := strings.ToLower(text)

	// Layer 1: Check for explicit red-flag language requirements (immediate rejection).
	for _, keyword := range f.policy.RedFlagLanguageKeywords {
		if strings.Contains(textLower, keyword) {
			f.logger.Debug("Rejected due to red-flag language keyword: %s", keyword)
			return false
		}
	}

	// Layer 2: Check for non-primary language indicators.
	nonPrimaryKeywordCount := 0
	for _, keyword := range f.policy.NonPrimaryLanguageKeywords {
		if strings.Contains(textLower, keyword) {
			nonPrimaryKeywordCount++
		}
	}

	if nonPrimaryKeywordCount >= f.policy.NonPrimaryKeywordMinCount {
		f.logger.Debug("Rejected due to %d non-primary language keywords", nonPrimaryKeywordCount)
		return false
	}

	// Layer 3: Primary language confidence vs strongest non-primary language.
	confidenceValues := f.languageDetector.ComputeLanguageConfidenceValues(text)
	confidenceByLang := map[lingua.Language]float64{}
	for _, conf := range confidenceValues {
		confidenceByLang[conf.Language()] = conf.Value()
	}
	primaryConfidence := confidenceByLang[f.primaryLanguage]
	maxOtherConfidence := 0.0
	for _, l := range f.detectionLanguages {
		if l == f.primaryLanguage {
			continue
		}
		if confidenceByLang[l] > maxOtherConfidence {
			maxOtherConfidence = confidenceByLang[l]
		}
	}

	if maxOtherConfidence > f.policy.NonPrimaryDominanceThreshold &&
		maxOtherConfidence > primaryConfidence*f.policy.NonPrimaryDominanceRatio {
		f.logger.Debug("Rejected due to non-primary confidence %.2f vs primary %.2f",
			maxOtherConfidence, primaryConfidence)
		return false
	}

	primaryIndicatorCount := 0
	for _, indicator := range f.policy.PrimaryLanguageIndicators {
		if strings.Contains(textLower, indicator) {
			primaryIndicatorCount++
		}
	}

	if primaryIndicatorCount >= f.policy.PrimaryIndicatorMinCount &&
		primaryConfidence >= f.policy.PrimaryIndicatorMinConfidence &&
		primaryConfidence >= maxOtherConfidence+f.policy.PrimaryVsNonPrimaryMinDelta {
		f.logger.Debug("Accepted due to %d primary-language indicators and %.2f confidence",
			primaryIndicatorCount, primaryConfidence)
		return true
	}

	result := primaryConfidence >= f.policy.DefaultPrimaryThreshold &&
		primaryConfidence >= maxOtherConfidence+f.policy.PrimaryVsNonPrimaryMinDelta

	if !result {
		f.logger.Debug("Rejected due to language confidence (Primary: %.2f, Non-primary: %.2f)", primaryConfidence, maxOtherConfidence)
	}

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
		if f.shouldNotify(job) {
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

func (f *Filter) shouldNotify(job *models.Job) bool {
	if f.notificationPolicy.RequireShouldSendEmail && !job.ShouldSendEmail {
		return false
	}
	if f.notificationPolicy.RequireFinalScore && job.FinalScore == nil {
		return false
	}
	if job.FinalScore != nil && *job.FinalScore < f.notificationPolicy.MinFinalScore {
		return false
	}
	return true
}

// FilterJobDescription filters out jobs that are not in primary language.
func (f *Filter) FilterJobDescription(job *models.Job) bool {
	if job.JobDescription == "" {
		f.logger.Debug("Kept job without description through language detection: %s at %s", job.Position, job.Company)
		return true // Allow jobs without descriptions
	}

	return f.isEnglishText(job.JobDescription)
}

func toLinguaLanguages(names []string) []lingua.Language {
	result := make([]lingua.Language, 0, len(names))
	for _, name := range names {
		lang := parseLanguage(name)
		if lang != lingua.Unknown {
			result = append(result, lang)
		}
	}
	return result
}

func parseLanguage(name string) lingua.Language {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "english", "en":
		return lingua.English
	case "german", "de":
		return lingua.German
	case "french", "fr":
		return lingua.French
	case "italian", "it":
		return lingua.Italian
	case "spanish", "es":
		return lingua.Spanish
	case "portuguese", "pt":
		return lingua.Portuguese
	case "dutch", "nl":
		return lingua.Dutch
	case "greek", "el":
		return lingua.Greek
	default:
		return lingua.Unknown
	}
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
