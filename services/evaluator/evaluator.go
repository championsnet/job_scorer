package evaluator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"job-scorer/models"
	"job-scorer/services/cv"
	"job-scorer/services/filter"
	"job-scorer/services/scraper"
	"job-scorer/utils"
)

type Evaluator struct {
	groqClient *GroqClient
	cvReader   *cv.CVReader
	scraper    *scraper.LinkedInScraper
	filter     *filter.Filter
	rateLimiter *utils.RateLimiter
	logger     *utils.Logger
}

func NewEvaluator(groqClient *GroqClient, cvReader *cv.CVReader, linkedInScraper *scraper.LinkedInScraper, rateLimiter *utils.RateLimiter, logger *utils.Logger) *Evaluator {
	// Use provided logger or create a new one
	if logger == nil {
		logger = utils.NewLogger("Evaluator")
	}

	return &Evaluator{
		groqClient:  groqClient,
		cvReader:    cvReader,
		scraper:     linkedInScraper,
		filter:      filter.NewFilter(logger),
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

func (e *Evaluator) EvaluateJob(job *models.Job) (*models.Job, error) {
	// Apply rate limiting before API call
	if err := e.rateLimiter.Acquire(); err != nil {
		e.logger.Error("Rate limiter error: %v", err)
		return job, err
	}
	
	prompt := e.getInitialPrompt(job)
	
	e.logger.Debug("Evaluating job: %s at %s", job.Position, job.Company)
	
	response, err := e.groqClient.ChatCompletion(prompt, 256)
	if err != nil {
		e.logger.Error("Error evaluating job: %v", err)
		job.Score = nil
		job.Reason = "Failed to evaluate job due to an error."
		return job, nil
	}

	e.logger.Debug("Raw evaluation response for %s: %s", job.Position, response)
	score, reason, reasons := e.parseEvaluationResponse(response)
	job.Score = score
	job.Reason = reason      // For backward compatibility
	job.Reasons = reasons    // Store all reasons

	if score != nil {
		if len(reasons) > 0 {
			e.logger.Info("Job '%s' at '%s' scored: %.1f - Reasons: %v", 
				job.Position, job.Company, *score, reasons)
		} else {
			e.logger.Info("Job '%s' at '%s' scored: %.1f - %s", 
				job.Position, job.Company, *score, reason)
		}
	} else {
		e.logger.Warning("Failed to parse score for job '%s' at '%s'", job.Position, job.Company)
	}

	return job, nil
}

func (e *Evaluator) EvaluateJobWithCV(job *models.Job) (*models.Job, error) {
	// First, get the job description
	jobDescription, err := e.scraper.FetchJobDescription(job.JobURL)
	if err != nil {
		e.logger.Error("Error fetching job description: %v", err)
		jobDescription = "Job description not available"
	}
	job.JobDescription = jobDescription

	// Check if job description is in English using the filter service
	if jobDescription != "Job description not available" {
		if !e.filter.FilterJobDescription(job) {
			e.logger.Warning("Filtered out non-English job description: %s at %s", job.Position, job.Company)
			
			// Set a low score and don't send email for non-English job descriptions
			zeroScore := 0.0
			job.FinalScore = &zeroScore
			job.ShouldSendEmail = false
			job.FinalReason = "Job description is not in English"
			job.FinalReasons = []string{"Job description is not in English"}
			
			return job, nil
		} else {
			e.logger.Debug("Job description passed language filter: %s at %s", job.Position, job.Company)
		}
	}

	// Get CV text
	cvText, err := e.cvReader.GetCV()
	if err != nil {
		e.logger.Error("Error loading CV: %v", err)
		return job, err
	}

	// Apply rate limiting before API call
	if err := e.rateLimiter.Acquire(); err != nil {
		e.logger.Error("Rate limiter error: %v", err)
		return job, err
	}
	
	prompt := e.getFinalPrompt(job, jobDescription, cvText)
	
	e.logger.Debug("Final evaluation for job: %s at %s", job.Position, job.Company)
	
	response, err := e.groqClient.ChatCompletion(prompt, 512)
	if err != nil {
		e.logger.Error("Error in final evaluation: %v", err)
		return job, err
	}

	e.logger.Debug("Raw final evaluation response for %s: %s", job.Position, response)
	finalScore, shouldSendEmail, finalReason, finalReasons := e.parseFinalEvaluationResponse(response)
	job.FinalScore = finalScore
	job.ShouldSendEmail = shouldSendEmail
	job.FinalReason = finalReason    // For backward compatibility
	job.FinalReasons = finalReasons  // Store all reasons

	if finalScore != nil {
		if len(finalReasons) > 0 {
			e.logger.Info("Final score for '%s' at '%s': %.1f, Send email: %t, Reasons: %v", 
				job.Position, job.Company, *finalScore, shouldSendEmail, finalReasons)
		} else {
			e.logger.Info("Final score for '%s' at '%s': %.1f, Send email: %t, Reason: %s", 
				job.Position, job.Company, *finalScore, shouldSendEmail, finalReason)
		}
	}

	return job, nil
}

func (e *Evaluator) getInitialPrompt(job *models.Job) string {
  // System prompt
  system := `
You are an AI Career Advisor that rates jobs for an entry-level marketing/BD/ops/admin candidate.
Always reply with JSON following this schema:
{
  "score": int,         // 0 (worst) to 10 (best)
  "recommend": bool,    // true if candidate should apply
  "reasons": [string]   // list of 1–3 brief rationale bullets
}`
  // User prompt
  user := fmt.Sprintf(`
CandidateProfile:
  - Citizenship: EU (Greek)
  - Location: commutable to Basel or Zurich, CH
  - Skills: Marketing, BusinessDev, Operations, Admin
  - Level: Junior/Intern/Entry
  - Language: English only (German low)
  - Priority: Growth & Stability > Salary

Job:
  Title: %s
  Company: %s
  Location: %s

Criteria:
  1. language (critical): English role?
  2. fieldFit (critical): Marketing/BD/Ops/Admin?
  3. location (high): Basel/Zurich/commutable?
  4. company (medium): Stable corp/SME?
`, job.Position, job.Company, job.Location)

  return system + "\n\n" + user
}

func (e *Evaluator) getFinalPrompt(job *models.Job, jobDesc, cv string) string {
  system := `
You are an AI Career Advisor performing a **CV vs Job** match.
Output only valid JSON:
{
  "finalScore": int,	// 0 (worst) to 10 (best)
  "shouldApply": bool,	// true if candidate should apply
  "reasons": [string]	// list of 1–3 brief rationale bullets
}`
  user := fmt.Sprintf(`
CandidateCV:
%s

Job:
  Title: %s
  Company: %s
  Location: %s
  Description: |
%s

FinalCriteria:
  1. cvMatch (critical)
  2. careerGrowth (high)
  3. permit & location (high): commutable to Basel or Zurich, CH
  4. languageReq (critical): must be English, no German
  5. companyRep (medium)
  6. seniority (medium): no senior roles
  7. experience (medium): up to 3 years of experience required

IMPORTANT:
- If jobDesc is non-English → return finalScore=0.
`, cv, job.Position, job.Company, job.Location, jobDesc)

  return system + "\n\n" + user
}

func (e *Evaluator) cleanResponse(response string) string {
	// Remove markdown code blocks (```json ... ```)
	codeBlockPattern := regexp.MustCompile(`\x60{3}json\s*\n?(.*?)\n?\x60{3}`)
	if match := codeBlockPattern.FindStringSubmatch(response); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	
	// Remove regular code blocks (``` ... ```)
	codeBlockPattern2 := regexp.MustCompile(`\x60{3}\s*\n?(.*?)\n?\x60{3}`)
	if match := codeBlockPattern2.FindStringSubmatch(response); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	
	return strings.TrimSpace(response)
}

func (e *Evaluator) parseEvaluationResponse(response string) (*float64, string, []string) {
	// Clean the response first
	cleanedResponse := e.cleanResponse(response)
	e.logger.Debug("Parsing evaluation response: %s", cleanedResponse)
	
	// Try to parse as JSON first (keep as fallback)
	type evalResp struct {
		Score     float64  `json:"score"`
		Recommend bool     `json:"recommend"`
		Reasons   []string `json:"reasons"`
	}
	var parsed evalResp
	if err := json.Unmarshal([]byte(cleanedResponse), &parsed); err == nil {
		reason := "No reason provided."
		var reasons []string = parsed.Reasons
		
		if len(reasons) > 0 {
			reason = reasons[0] // For backward compatibility
			e.logger.Debug("Successfully parsed JSON response with %d reasons: %v", len(reasons), reasons)
		} else {
			reasons = []string{reason}
			e.logger.Debug("JSON parsed but no reasons found")
		}
		
		return &parsed.Score, reason, reasons
	} else {
		e.logger.Debug("JSON parsing failed: %v", err)
	}

	// Enhanced regex parsing for various formats
	var score *float64
	var reason string
	var reasons []string

	// Multiple score patterns to catch different formats
	scorePatterns := []*regexp.Regexp{
		regexp.MustCompile(`"score"\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`Score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`"finalScore"\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`finalScore\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`Final Score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`(\d+\.?\d*)\s*\/\s*10`), // Score out of 10 format
	}

	for _, pattern := range scorePatterns {
		scoreMatch := pattern.FindStringSubmatch(cleanedResponse)
		if len(scoreMatch) > 1 {
			if parsedScore, err := strconv.ParseFloat(scoreMatch[1], 64); err == nil {
				score = &parsedScore
				e.logger.Debug("Found score using pattern %s: %.1f", pattern.String(), *score)
				break
			}
		}
	}

	// Improved regex to extract reasons array - handle both single and multiple reasons
	reasonsArrayPattern := regexp.MustCompile(`"reasons"\s*:\s*\[\s*((?:"[^"]*"(?:\s*,\s*"[^"]*")*)?)\s*\]`)
	if match := reasonsArrayPattern.FindStringSubmatch(cleanedResponse); len(match) > 1 {
		// Extract individual reasons from the array
		reasonsStr := match[1]
		reasonPattern := regexp.MustCompile(`"([^"]+)"`)
		reasonMatches := reasonPattern.FindAllStringSubmatch(reasonsStr, -1)
		
		for _, m := range reasonMatches {
			if len(m) > 1 {
				reasons = append(reasons, strings.TrimSpace(m[1]))
			}
		}
		
		e.logger.Debug("Extracted %d reasons from array using regex: %v", len(reasons), reasons)
	}
	
	// If we couldn't extract reasons array, try individual reason patterns
	if len(reasons) == 0 {
		// Multiple reason patterns
		reasonPatterns := []*regexp.Regexp{
			regexp.MustCompile(`"reason"\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`reason\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`Reason\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`reason\s*:\s*([^,\n]+)`),
			regexp.MustCompile(`Reason\s*:\s*([^,\n]+)`),
		}

		for _, pattern := range reasonPatterns {
			reasonMatch := pattern.FindStringSubmatch(cleanedResponse)
			if len(reasonMatch) > 1 {
				reason = strings.TrimSpace(reasonMatch[1])
				reasons = []string{reason}
				e.logger.Debug("Found reason using pattern %s: %s", pattern.String(), reason)
				break
			}
		}
	} else if len(reasons) > 0 {
		// Set the first reason for backward compatibility
		reason = reasons[0]
	}

	if len(reasons) == 0 {
		reason = "No reason provided."
		reasons = []string{reason}
	}

	e.logger.Debug("Parsed - Score: %v, Reason: %s, Reasons: %v", score, reason, reasons)
	return score, reason, reasons
}

func (e *Evaluator) parseFinalEvaluationResponse(response string) (*float64, bool, string, []string) {
	// Clean the response first
	cleanedResponse := e.cleanResponse(response)
	e.logger.Debug("Parsing final evaluation response: %s", cleanedResponse)
	
	// Try to parse as JSON first (keep as fallback)
	type finalResp struct {
		FinalScore   float64  `json:"finalScore"`
		ShouldApply  bool     `json:"shouldApply"`
		Reasons      []string `json:"reasons"`
	}
	var parsed finalResp
	if err := json.Unmarshal([]byte(cleanedResponse), &parsed); err == nil {
		reason := "No reason provided."
		var reasons []string = parsed.Reasons
		
		if len(reasons) > 0 {
			reason = reasons[0] // For backward compatibility
			e.logger.Debug("Successfully parsed JSON response with %d reasons", len(reasons))
		} else {
			reasons = []string{reason}
		}
		
		return &parsed.FinalScore, parsed.ShouldApply, reason, reasons
	}

	// Enhanced regex parsing for final evaluation
	var finalScore *float64
	var shouldSendEmail bool
	var finalReason string
	var finalReasons []string

	// Multiple score patterns for final evaluation
	scorePatterns := []*regexp.Regexp{
		regexp.MustCompile(`"finalScore"\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`finalScore\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`Final Score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`"score"\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`Score\s*:\s*(\d+\.?\d*)`),
		regexp.MustCompile(`(\d+\.?\d*)\s*\/\s*10`), // Score out of 10 format
	}

	for _, pattern := range scorePatterns {
		scoreMatch := pattern.FindStringSubmatch(cleanedResponse)
		if len(scoreMatch) > 1 {
			if parsedScore, err := strconv.ParseFloat(scoreMatch[1], 64); err == nil {
				finalScore = &parsedScore
				e.logger.Debug("Found final score using pattern %s: %.1f", pattern.String(), *finalScore)
				break
			}
		}
	}

	// Multiple patterns for should send email
	emailPatterns := []*regexp.Regexp{
		regexp.MustCompile(`"shouldApply"\s*:\s*(true|false)`),
		regexp.MustCompile(`shouldApply\s*:\s*(true|false)`),
		regexp.MustCompile(`"shouldSendEmail"\s*:\s*(true|false)`),
		regexp.MustCompile(`shouldSendEmail\s*:\s*(true|false)`),
		regexp.MustCompile(`Should Send Email\s*:\s*(YES|NO|Yes|No|yes|no)`),
		regexp.MustCompile(`Send Email\s*:\s*(YES|NO|Yes|No|yes|no)`),
		regexp.MustCompile(`Should Apply\s*:\s*(YES|NO|Yes|No|yes|no)`),
		regexp.MustCompile(`Apply\s*:\s*(YES|NO|Yes|No|yes|no)`),
	}

	for _, pattern := range emailPatterns {
		emailMatch := pattern.FindStringSubmatch(cleanedResponse)
		if len(emailMatch) > 1 {
			value := strings.ToLower(emailMatch[1])
			shouldSendEmail = value == "true" || value == "yes"
			e.logger.Debug("Found email pattern %s: %s, result: %t", pattern.String(), emailMatch[1], shouldSendEmail)
			break
		}
	}

	// Try to extract reasons array - use the same improved pattern as parseEvaluationResponse
	reasonsArrayPattern := regexp.MustCompile(`"reasons"\s*:\s*\[\s*((?:"[^"]*"(?:\s*,\s*"[^"]*")*)?)\s*\]`)
	if match := reasonsArrayPattern.FindStringSubmatch(cleanedResponse); len(match) > 1 {
		// Extract individual reasons from the array
		reasonsStr := match[1]
		reasonPattern := regexp.MustCompile(`"([^"]+)"`)
		reasonMatches := reasonPattern.FindAllStringSubmatch(reasonsStr, -1)
		
		for _, m := range reasonMatches {
			if len(m) > 1 {
				finalReasons = append(finalReasons, strings.TrimSpace(m[1]))
			}
		}
		
		e.logger.Debug("Extracted %d final reasons from array", len(finalReasons))
	}
	
	// If we couldn't extract reasons array, try individual reason patterns
	if len(finalReasons) == 0 {
		// Multiple reason patterns
		reasonPatterns := []*regexp.Regexp{
			regexp.MustCompile(`"reason"\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`reason\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`Reason\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`reason\s*:\s*([^,\n]+)`),
			regexp.MustCompile(`Reason\s*:\s*([^,\n]+)`),
		}

		for _, pattern := range reasonPatterns {
			reasonMatch := pattern.FindStringSubmatch(cleanedResponse)
			if len(reasonMatch) > 1 {
				finalReason = strings.TrimSpace(reasonMatch[1])
				finalReasons = []string{finalReason}
				e.logger.Debug("Found final reason using pattern %s: %s", pattern.String(), finalReason)
				break
			}
		}
	} else if len(finalReasons) > 0 {
		// Set the first reason for backward compatibility
		finalReason = finalReasons[0]
	}

	if len(finalReasons) == 0 {
		finalReason = "No reason provided."
		finalReasons = []string{finalReason}
	}

	e.logger.Debug("Parsed - Final Score: %v, Should Send Email: %t, Reason: %s, Reasons: %v", 
		finalScore, shouldSendEmail, finalReason, finalReasons)
	return finalScore, shouldSendEmail, finalReason, finalReasons
} 