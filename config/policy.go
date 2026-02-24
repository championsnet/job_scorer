package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Policy struct {
	CandidateProfile CandidateProfilePolicy `json:"candidateProfile"`
	App              AppPolicy              `json:"app"`
	Filters          FilterPolicy           `json:"filters"`
	Evaluation       EvaluationPolicy       `json:"evaluation"`
	Pipeline         PipelinePolicy         `json:"pipeline"`
	Notification     NotificationPolicy     `json:"notification"`
	CV               CVPolicy               `json:"cv"`
	Scraper          ScraperPolicy          `json:"scraper"`
}

type CandidateProfilePolicy struct {
	TargetLocations  []string `json:"targetLocations"`
	CommuteLocations []string `json:"commuteLocations"`
	Languages        []string `json:"languages"`
	DesiredFields    []string `json:"desiredFields"`
	Seniority        []string `json:"seniority"`
}

type AppPolicy struct {
	CronSchedule string `json:"cronSchedule"`
}

type FilterPolicy struct {
	UnwantedLocations    []string `json:"unwantedLocations"`
	UnwantedWordsInTitle []string `json:"unwantedWordsInTitle"`

	PrimaryLanguage          string   `json:"primaryLanguage"`
	DetectionLanguages       []string `json:"detectionLanguages"`
	RedFlagLanguageKeywords  []string `json:"redFlagLanguageKeywords"`
	RequiredLanguageKeywords []string `json:"requiredLanguageKeywords"` // Deprecated alias for backwards compatibility
	NonPrimaryLanguageKeywords []string `json:"nonPrimaryLanguageKeywords"`
	PrimaryLanguageIndicators  []string `json:"primaryLanguageIndicators"`

	NonPrimaryKeywordMinCount int     `json:"nonPrimaryKeywordMinCount"`
	NonPrimaryDominanceThreshold float64 `json:"nonPrimaryDominanceThreshold"`
	NonPrimaryDominanceRatio     float64 `json:"nonPrimaryDominanceRatio"`

	PrimaryIndicatorMinCount       int     `json:"primaryIndicatorMinCount"`
	PrimaryIndicatorMinConfidence  float64 `json:"primaryIndicatorMinConfidence"`
	DefaultPrimaryThreshold        float64 `json:"defaultPrimaryThreshold"`
	PrimaryVsNonPrimaryMinDelta    float64 `json:"primaryVsNonPrimaryMinDelta"`
	MinTextLengthForLanguageDetect int     `json:"minTextLengthForLanguageDetect"`
}

type EvaluationPolicy struct {
	InitialPromptTemplate    string      `json:"initialPromptTemplate"`
	FinalPromptTemplate      string      `json:"finalPromptTemplate"`
	BatchPromptTemplate      string      `json:"batchPromptTemplate"`
	ValidationPromptTemplate string      `json:"validationPromptTemplate"`
	MaxTokens                MaxTokenMap `json:"maxTokens"`
	CVPromptTruncation       CVPromptCfg `json:"cvPromptTruncation"`
}

type MaxTokenMap struct {
	Initial    int `json:"initial"`
	Final      int `json:"final"`
	Batch      int `json:"batch"`
	Validation int `json:"validation"`
}

type CVPromptCfg struct {
	MaxLength         int `json:"maxLength"`
	HeadLength        int `json:"headLength"`
	TailLength        int `json:"tailLength"`
	ValidationMaxSize int `json:"validationMaxSize"`
}

type PipelinePolicy struct {
	PromisingScoreThreshold      float64  `json:"promisingScoreThreshold"`
	BatchSize                    int      `json:"batchSize"`
	IndividualFallbackMaxJobs    int      `json:"individualFallbackMaxJobs"`
	EnableFinalValidation        bool     `json:"enableFinalValidation"`
	RejectEmptyDescriptions      bool     `json:"rejectEmptyDescriptions"`
	RejectPlaceholderDescription bool     `json:"rejectPlaceholderDescription"`
	RedFlagPhrases               []string `json:"redFlagPhrases"`
}

type NotificationPolicy struct {
	MinFinalScore              float64 `json:"minFinalScore"`
	RequireShouldSendEmail     bool    `json:"requireShouldSendEmail"`
	RequireFinalScore          bool    `json:"requireFinalScore"`
	EmailSubjectTemplate       string  `json:"emailSubjectTemplate"`
	HeaderTitle                string  `json:"headerTitle"`
	HeaderSubtitle             string  `json:"headerSubtitle"`
	SummaryTemplate            string  `json:"summaryTemplate"`
	DescriptionTitle           string  `json:"descriptionTitle"`
	ReasonTitle                string  `json:"reasonTitle"`
	ApplyButtonText            string  `json:"applyButtonText"`
	PersonalizedTitle          string  `json:"personalizedTitle"`
	PersonalizedSubtitle       string  `json:"personalizedSubtitle"`
	FooterLineOne              string  `json:"footerLineOne"`
	FooterLineTwoTemplate      string  `json:"footerLineTwoTemplate"`
	FooterLineThree            string  `json:"footerLineThree"`
	JobDescriptionPreviewLimit int     `json:"jobDescriptionPreviewLimit"`
}

type CVPolicy struct {
	Path                        string   `json:"path"`
	ParserOrder                 []string `json:"parserOrder"`
	EnableUniPDF                bool     `json:"enableUniPDF"`
	FallbackText                string   `json:"fallbackText"`
	MinValidTextLength          int      `json:"minValidTextLength"`
	MinLetterRatio              float64  `json:"minLetterRatio"`
	AllowEnvOverrideUniPDF      bool     `json:"allowEnvOverrideUniPDF"`
	LogParserUsed               bool     `json:"logParserUsed"`
	ValidateCandidateByHeuristic bool     `json:"validateCandidateByHeuristic"`
}

type ScraperPolicy struct {
	DateSincePosted           string `json:"dateSincePosted"`
	PaginationBatchSize        int   `json:"paginationBatchSize"`
	MaxConsecutiveErrors       int   `json:"maxConsecutiveErrors"`
	MaxRequestRetries          int   `json:"maxRequestRetries"`
	ConsecutiveBackoffBaseSecs int   `json:"consecutiveBackoffBaseSeconds"`
	RetryBackoffBaseSecs       int   `json:"retryBackoffBaseSeconds"`
	RetryJitterMaxMs           int   `json:"retryJitterMaxMs"`
	InterBatchDelayMinMs       int   `json:"interBatchDelayMinMs"`
	InterBatchDelayMaxMs       int   `json:"interBatchDelayMaxMs"`
	DescriptionDelayMinMs      int   `json:"descriptionDelayMinMs"`
	DescriptionDelayMaxMs      int   `json:"descriptionDelayMaxMs"`
	DescriptionMaxLength       int   `json:"descriptionMaxLength"`
	ErrorBodyPreviewLength     int   `json:"errorBodyPreviewLength"`
	VerboseParseLogs           bool  `json:"verboseParseLogs"`
	Selectors                  []string `json:"selectors"`
}

func loadPolicy(path string) (Policy, error) {
	policy := defaultPolicy()
	path = strings.TrimSpace(path)
	if path == "" {
		return policy, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return policy, fmt.Errorf("failed reading policy file %s: %w", path, err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return policy, nil
	}

	if err := json.Unmarshal(data, &policy); err != nil {
		return policy, fmt.Errorf("failed parsing policy JSON %s: %w", path, err)
	}
	applyPolicyDefaults(&policy)
	if err := validatePolicy(policy); err != nil {
		return policy, err
	}
	return policy, nil
}

func defaultPolicy() Policy {
	return Policy{
		CandidateProfile: CandidateProfilePolicy{
			TargetLocations:  []string{"Basel", "Zurich"},
			CommuteLocations: []string{"Basel", "Zurich"},
			Languages:        []string{"English"},
			DesiredFields:    []string{"Marketing", "Business Development", "Operations", "Administration", "Strategy", "HR"},
			Seniority:        []string{"Entry", "Junior", "Intern", "Graduate", "Trainee", "Assistant"},
		},
		App: AppPolicy{
			CronSchedule: "0 */1 * * *",
		},
		Filters: FilterPolicy{
			UnwantedLocations: []string{
				"EMEA", "DACH", "Switzerland (Remote)", "Europe", "EU",
			},
			UnwantedWordsInTitle: []string{
				"Head", "Senior", "Director", "Sr.",
			},
			PrimaryLanguage:    "english",
			DetectionLanguages: []string{"english", "german", "french"},
			RedFlagLanguageKeywords: []string{
				"deutsch erforderlich", "german required", "german fluency", "fluent german",
				"muttersprache deutsch", "german native", "deutschkenntnisse",
				"flieBend deutsch", "fluent in german", "german language skills",
				"deutsch als muttersprache", "german c1", "german c2", "deutsch c1", "deutsch c2",
				"deutsche sprachkenntnisse", "verhandlungssicher deutsch", "german proficiency",
			},
			NonPrimaryLanguageKeywords: []string{
				"stellenausschreibung", "arbeitsplatz", "bewerbung", "lebenslauf",
				"anstellung", "mitarbeiter", "unternehmen", "gesellschaft",
				"verantwortung", "aufgaben", "anforderungen", "qualifikation",
				"berufserfahrung", "abschluss", "studium", "ausbildung",
			},
			PrimaryLanguageIndicators: []string{
				"english", "international", "multicultural", "global", "worldwide",
				"european", "eu", "remote", "hybrid", "flexible", "agile",
				"you will", "we are", "join our", "opportunity", "experience",
				"skills", "requirements", "responsibilities", "benefits",
			},
			NonPrimaryKeywordMinCount:       2,
			NonPrimaryDominanceThreshold:    0.60,
			NonPrimaryDominanceRatio:        1.50,
			PrimaryIndicatorMinCount:        2,
			PrimaryIndicatorMinConfidence:   0.40,
			DefaultPrimaryThreshold:         0.45,
			PrimaryVsNonPrimaryMinDelta:     0.10,
			MinTextLengthForLanguageDetect: 8,
		},
		Evaluation: EvaluationPolicy{
			InitialPromptTemplate: `Rate job 0-10 for EU candidate (Basel/Zurich, entry-level).

GOOD FIELDS: Marketing, Business Dev, Operations, Admin, Strategy, HR
BAD FIELDS: Engineering, Data Science, Finance, Legal, Medical
GOOD LEVEL: Entry, Junior, Intern, Graduate, Trainee, Assistant
BAD LEVEL: Senior, Lead, Director, 5+ years

Job: "{{POSITION}}" at {{COMPANY}} ({{LOCATION}})

Respond JSON only:
{"score": X, "recommend": true/false, "reasons": ["brief reason"]}`,
			FinalPromptTemplate: `CV vs Job match analysis (0-10).

RED FLAGS (score=0): German required, Deutsch erforderlich, non-English fluency
MATCH: Skills, experience level (0-2yrs), Basel/Zurich location
SKILLS: Marketing, Business Dev, Operations, Admin, Strategy, HR

CV:
{{CV}}

Job: "{{POSITION}}" at {{COMPANY}} ({{LOCATION}})
Description: {{JOB_DESCRIPTION}}

JSON only:
{"finalScore": X, "shouldApply": true/false, "reasons": ["key reason"]}`,
			BatchPromptTemplate: `Rate jobs 0-10 for EU candidate (Basel/Zurich, entry-level).

GOOD FIELDS: Marketing, Business Dev, Operations, Admin, Strategy, HR
BAD FIELDS: Engineering, Data Science, Finance, Legal, Medical
GOOD LEVEL: Entry, Junior, Intern, Graduate, Trainee, Assistant
BAD LEVEL: Senior, Lead, Director, 5+ years

{{JOBS}}
Respond with JSON array:
[{"jobId": 1, "score": X, "recommend": true/false, "reasons": ["brief reason"]}, ...]`,
			ValidationPromptTemplate: `Validate job recommendation. Check for: wrong field, seniority, German requirements, skill mismatch.

CV: {{CV}}

Job: {{JOB_DESCRIPTION}}
Previous: score={{FINAL_SCORE}}, apply={{SHOULD_APPLY}}, reasons={{FINAL_REASONS}}

JSON only: {"valid": true/false, "reason": "brief"}`,
			MaxTokens: MaxTokenMap{
				Initial:    256,
				Final:      512,
				Batch:      512,
				Validation: 256,
			},
			CVPromptTruncation: CVPromptCfg{
				MaxLength:         2000,
				HeadLength:        1500,
				TailLength:        500,
				ValidationMaxSize: 1000,
			},
		},
		Pipeline: PipelinePolicy{
			PromisingScoreThreshold:      5.0,
			BatchSize:                    3,
			IndividualFallbackMaxJobs:    5,
			EnableFinalValidation:        true,
			RejectEmptyDescriptions:      true,
			RejectPlaceholderDescription: true,
			RedFlagPhrases: []string{
				"german required", "fluency in german", "fluent german", "deutsch erforderlich",
				"job description not available", "not available", "german is required",
				"german language requirement", "german as a requirement", "german as primary language",
			},
		},
		Notification: NotificationPolicy{
			MinFinalScore:              0,
			RequireShouldSendEmail:     true,
			RequireFinalScore:          true,
			EmailSubjectTemplate:       "Job Alert: {{COUNT}} New Recommended Jobs Found!",
			HeaderTitle:                "New Job Recommendations",
			HeaderSubtitle:             "Your personalized job matches are ready!",
			SummaryTemplate:            "Summary: Found {{COUNT}} highly recommended jobs that match your CV and criteria.",
			DescriptionTitle:           "Job Description:",
			ReasonTitle:                "Why this matches your profile:",
			ApplyButtonText:            "Apply Now",
			PersonalizedTitle:          "Good luck with your applications!",
			PersonalizedSubtitle:       "Keep building your career momentum.",
			FooterLineOne:              "This email was generated by your Job Scorer system",
			FooterLineTwoTemplate:      "Generated on {{TIMESTAMP}}",
			FooterLineThree:            "Keep building your career, one opportunity at a time!",
			JobDescriptionPreviewLimit: 300,
		},
		CV: CVPolicy{
			Path:                       "CV_Vasiliki Ploumistou_22_05.pdf",
			ParserOrder:                 []string{"unipdf", "fitz", "ledongthuc"},
			EnableUniPDF:                false,
			FallbackText:                "CV not available. Candidate background unavailable.",
			MinValidTextLength:          40,
			MinLetterRatio:              0.40,
			AllowEnvOverrideUniPDF:      true,
			LogParserUsed:               true,
			ValidateCandidateByHeuristic: true,
		},
		Scraper: ScraperPolicy{
			DateSincePosted:           "past hour",
			PaginationBatchSize:        10,
			MaxConsecutiveErrors:       3,
			MaxRequestRetries:          3,
			ConsecutiveBackoffBaseSecs: 2,
			RetryBackoffBaseSecs:       2,
			RetryJitterMaxMs:           1000,
			InterBatchDelayMinMs:       1000,
			InterBatchDelayMaxMs:       3000,
			DescriptionDelayMinMs:      500,
			DescriptionDelayMaxMs:      2500,
			DescriptionMaxLength:       5000,
			ErrorBodyPreviewLength:     500,
			VerboseParseLogs:           false,
			Selectors: []string{
				".job-search-card",
				".jobs-search__results-list li",
				"li[data-entity-urn]",
				".result-card",
				".job-result-card",
				"li",
			},
		},
	}
}

func applyPolicyDefaults(p *Policy) {
	defaults := defaultPolicy()

	if len(p.Filters.UnwantedLocations) == 0 {
		p.Filters.UnwantedLocations = defaults.Filters.UnwantedLocations
	}
	if strings.TrimSpace(p.App.CronSchedule) == "" {
		p.App.CronSchedule = defaults.App.CronSchedule
	}
	if len(p.Filters.UnwantedWordsInTitle) == 0 {
		p.Filters.UnwantedWordsInTitle = defaults.Filters.UnwantedWordsInTitle
	}
	if strings.TrimSpace(p.Filters.PrimaryLanguage) == "" {
		p.Filters.PrimaryLanguage = defaults.Filters.PrimaryLanguage
	}
	if len(p.Filters.DetectionLanguages) == 0 {
		p.Filters.DetectionLanguages = defaults.Filters.DetectionLanguages
	}
	if len(p.Filters.RedFlagLanguageKeywords) == 0 {
		if len(p.Filters.RequiredLanguageKeywords) > 0 {
			p.Filters.RedFlagLanguageKeywords = p.Filters.RequiredLanguageKeywords
		} else {
			p.Filters.RedFlagLanguageKeywords = defaults.Filters.RedFlagLanguageKeywords
		}
	}
	if len(p.Filters.NonPrimaryLanguageKeywords) == 0 {
		p.Filters.NonPrimaryLanguageKeywords = defaults.Filters.NonPrimaryLanguageKeywords
	}
	if len(p.Filters.PrimaryLanguageIndicators) == 0 {
		p.Filters.PrimaryLanguageIndicators = defaults.Filters.PrimaryLanguageIndicators
	}
	if p.Filters.NonPrimaryKeywordMinCount <= 0 {
		p.Filters.NonPrimaryKeywordMinCount = defaults.Filters.NonPrimaryKeywordMinCount
	}
	if p.Filters.NonPrimaryDominanceThreshold <= 0 {
		p.Filters.NonPrimaryDominanceThreshold = defaults.Filters.NonPrimaryDominanceThreshold
	}
	if p.Filters.NonPrimaryDominanceRatio <= 0 {
		p.Filters.NonPrimaryDominanceRatio = defaults.Filters.NonPrimaryDominanceRatio
	}
	if p.Filters.PrimaryIndicatorMinCount <= 0 {
		p.Filters.PrimaryIndicatorMinCount = defaults.Filters.PrimaryIndicatorMinCount
	}
	if p.Filters.PrimaryIndicatorMinConfidence <= 0 {
		p.Filters.PrimaryIndicatorMinConfidence = defaults.Filters.PrimaryIndicatorMinConfidence
	}
	if p.Filters.DefaultPrimaryThreshold <= 0 {
		p.Filters.DefaultPrimaryThreshold = defaults.Filters.DefaultPrimaryThreshold
	}
	if p.Filters.PrimaryVsNonPrimaryMinDelta <= 0 {
		p.Filters.PrimaryVsNonPrimaryMinDelta = defaults.Filters.PrimaryVsNonPrimaryMinDelta
	}
	if p.Filters.MinTextLengthForLanguageDetect <= 0 {
		p.Filters.MinTextLengthForLanguageDetect = defaults.Filters.MinTextLengthForLanguageDetect
	}

	if strings.TrimSpace(p.Evaluation.InitialPromptTemplate) == "" {
		p.Evaluation.InitialPromptTemplate = defaults.Evaluation.InitialPromptTemplate
	}
	if strings.TrimSpace(p.Evaluation.FinalPromptTemplate) == "" {
		p.Evaluation.FinalPromptTemplate = defaults.Evaluation.FinalPromptTemplate
	}
	if strings.TrimSpace(p.Evaluation.BatchPromptTemplate) == "" {
		p.Evaluation.BatchPromptTemplate = defaults.Evaluation.BatchPromptTemplate
	}
	if strings.TrimSpace(p.Evaluation.ValidationPromptTemplate) == "" {
		p.Evaluation.ValidationPromptTemplate = defaults.Evaluation.ValidationPromptTemplate
	}
	if p.Evaluation.MaxTokens.Initial <= 0 {
		p.Evaluation.MaxTokens.Initial = defaults.Evaluation.MaxTokens.Initial
	}
	if p.Evaluation.MaxTokens.Final <= 0 {
		p.Evaluation.MaxTokens.Final = defaults.Evaluation.MaxTokens.Final
	}
	if p.Evaluation.MaxTokens.Batch <= 0 {
		p.Evaluation.MaxTokens.Batch = defaults.Evaluation.MaxTokens.Batch
	}
	if p.Evaluation.MaxTokens.Validation <= 0 {
		p.Evaluation.MaxTokens.Validation = defaults.Evaluation.MaxTokens.Validation
	}
	if p.Evaluation.CVPromptTruncation.MaxLength <= 0 {
		p.Evaluation.CVPromptTruncation.MaxLength = defaults.Evaluation.CVPromptTruncation.MaxLength
	}
	if p.Evaluation.CVPromptTruncation.HeadLength <= 0 {
		p.Evaluation.CVPromptTruncation.HeadLength = defaults.Evaluation.CVPromptTruncation.HeadLength
	}
	if p.Evaluation.CVPromptTruncation.TailLength <= 0 {
		p.Evaluation.CVPromptTruncation.TailLength = defaults.Evaluation.CVPromptTruncation.TailLength
	}
	if p.Evaluation.CVPromptTruncation.ValidationMaxSize <= 0 {
		p.Evaluation.CVPromptTruncation.ValidationMaxSize = defaults.Evaluation.CVPromptTruncation.ValidationMaxSize
	}

	if p.Pipeline.PromisingScoreThreshold < 0 {
		p.Pipeline.PromisingScoreThreshold = defaults.Pipeline.PromisingScoreThreshold
	}
	if p.Pipeline.BatchSize <= 0 {
		p.Pipeline.BatchSize = defaults.Pipeline.BatchSize
	}
	if p.Pipeline.IndividualFallbackMaxJobs <= 0 {
		p.Pipeline.IndividualFallbackMaxJobs = defaults.Pipeline.IndividualFallbackMaxJobs
	}
	if len(p.Pipeline.RedFlagPhrases) == 0 {
		p.Pipeline.RedFlagPhrases = defaults.Pipeline.RedFlagPhrases
	}

	if strings.TrimSpace(p.Notification.EmailSubjectTemplate) == "" {
		p.Notification.EmailSubjectTemplate = defaults.Notification.EmailSubjectTemplate
	}
	if strings.TrimSpace(p.Notification.HeaderTitle) == "" {
		p.Notification.HeaderTitle = defaults.Notification.HeaderTitle
	}
	if strings.TrimSpace(p.Notification.HeaderSubtitle) == "" {
		p.Notification.HeaderSubtitle = defaults.Notification.HeaderSubtitle
	}
	if strings.TrimSpace(p.Notification.SummaryTemplate) == "" {
		p.Notification.SummaryTemplate = defaults.Notification.SummaryTemplate
	}
	if strings.TrimSpace(p.Notification.DescriptionTitle) == "" {
		p.Notification.DescriptionTitle = defaults.Notification.DescriptionTitle
	}
	if strings.TrimSpace(p.Notification.ReasonTitle) == "" {
		p.Notification.ReasonTitle = defaults.Notification.ReasonTitle
	}
	if strings.TrimSpace(p.Notification.ApplyButtonText) == "" {
		p.Notification.ApplyButtonText = defaults.Notification.ApplyButtonText
	}
	if strings.TrimSpace(p.Notification.PersonalizedTitle) == "" {
		p.Notification.PersonalizedTitle = defaults.Notification.PersonalizedTitle
	}
	if strings.TrimSpace(p.Notification.PersonalizedSubtitle) == "" {
		p.Notification.PersonalizedSubtitle = defaults.Notification.PersonalizedSubtitle
	}
	if strings.TrimSpace(p.Notification.FooterLineOne) == "" {
		p.Notification.FooterLineOne = defaults.Notification.FooterLineOne
	}
	if strings.TrimSpace(p.Notification.FooterLineTwoTemplate) == "" {
		p.Notification.FooterLineTwoTemplate = defaults.Notification.FooterLineTwoTemplate
	}
	if strings.TrimSpace(p.Notification.FooterLineThree) == "" {
		p.Notification.FooterLineThree = defaults.Notification.FooterLineThree
	}
	if p.Notification.JobDescriptionPreviewLimit <= 0 {
		p.Notification.JobDescriptionPreviewLimit = defaults.Notification.JobDescriptionPreviewLimit
	}

	if len(p.CV.ParserOrder) == 0 {
		p.CV.ParserOrder = defaults.CV.ParserOrder
	}
	if strings.TrimSpace(p.CV.Path) == "" {
		p.CV.Path = defaults.CV.Path
	}
	if strings.TrimSpace(p.CV.FallbackText) == "" {
		p.CV.FallbackText = defaults.CV.FallbackText
	}
	if p.CV.MinValidTextLength <= 0 {
		p.CV.MinValidTextLength = defaults.CV.MinValidTextLength
	}
	if p.CV.MinLetterRatio <= 0 {
		p.CV.MinLetterRatio = defaults.CV.MinLetterRatio
	}

	if p.Scraper.PaginationBatchSize <= 0 {
		p.Scraper.PaginationBatchSize = defaults.Scraper.PaginationBatchSize
	}
	if strings.TrimSpace(p.Scraper.DateSincePosted) == "" {
		p.Scraper.DateSincePosted = defaults.Scraper.DateSincePosted
	}
	if p.Scraper.MaxConsecutiveErrors <= 0 {
		p.Scraper.MaxConsecutiveErrors = defaults.Scraper.MaxConsecutiveErrors
	}
	if p.Scraper.MaxRequestRetries <= 0 {
		p.Scraper.MaxRequestRetries = defaults.Scraper.MaxRequestRetries
	}
	if p.Scraper.ConsecutiveBackoffBaseSecs <= 0 {
		p.Scraper.ConsecutiveBackoffBaseSecs = defaults.Scraper.ConsecutiveBackoffBaseSecs
	}
	if p.Scraper.RetryBackoffBaseSecs <= 0 {
		p.Scraper.RetryBackoffBaseSecs = defaults.Scraper.RetryBackoffBaseSecs
	}
	if p.Scraper.RetryJitterMaxMs <= 0 {
		p.Scraper.RetryJitterMaxMs = defaults.Scraper.RetryJitterMaxMs
	}
	if p.Scraper.InterBatchDelayMinMs <= 0 {
		p.Scraper.InterBatchDelayMinMs = defaults.Scraper.InterBatchDelayMinMs
	}
	if p.Scraper.InterBatchDelayMaxMs <= p.Scraper.InterBatchDelayMinMs {
		p.Scraper.InterBatchDelayMaxMs = defaults.Scraper.InterBatchDelayMaxMs
	}
	if p.Scraper.DescriptionDelayMinMs <= 0 {
		p.Scraper.DescriptionDelayMinMs = defaults.Scraper.DescriptionDelayMinMs
	}
	if p.Scraper.DescriptionDelayMaxMs <= p.Scraper.DescriptionDelayMinMs {
		p.Scraper.DescriptionDelayMaxMs = defaults.Scraper.DescriptionDelayMaxMs
	}
	if p.Scraper.DescriptionMaxLength <= 0 {
		p.Scraper.DescriptionMaxLength = defaults.Scraper.DescriptionMaxLength
	}
	if p.Scraper.ErrorBodyPreviewLength <= 0 {
		p.Scraper.ErrorBodyPreviewLength = defaults.Scraper.ErrorBodyPreviewLength
	}
	if len(p.Scraper.Selectors) == 0 {
		p.Scraper.Selectors = defaults.Scraper.Selectors
	}
}

func validatePolicy(p Policy) error {
	if p.Pipeline.PromisingScoreThreshold < 0 || p.Pipeline.PromisingScoreThreshold > 10 {
		return fmt.Errorf("policy.pipeline.promisingScoreThreshold must be between 0 and 10")
	}
	if p.Notification.MinFinalScore < 0 || p.Notification.MinFinalScore > 10 {
		return fmt.Errorf("policy.notification.minFinalScore must be between 0 and 10")
	}
	return nil
}
