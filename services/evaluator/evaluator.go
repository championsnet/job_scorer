package evaluator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"job-scorer/config"
	"job-scorer/models"
	"job-scorer/services/cv"
	"job-scorer/services/filter"
	"job-scorer/services/scraper"
	"job-scorer/utils"
)

type Evaluator struct {
	openAIClient *OpenAIClient
	cvReader     *cv.CVReader
	scraper      *scraper.LinkedInScraper
	filter       *filter.Filter
	policy       config.Policy
	rateLimiter  *utils.RateLimiter
	logger       *utils.Logger
}

func NewEvaluator(openAIClient *OpenAIClient, cvReader *cv.CVReader, linkedInScraper *scraper.LinkedInScraper, rateLimiter *utils.RateLimiter, policy config.Policy, logger *utils.Logger) *Evaluator {
	// Use provided logger or create a new one
	if logger == nil {
		logger = utils.NewLogger("Evaluator")
	}

	return &Evaluator{
		openAIClient: openAIClient,
		cvReader:     cvReader,
		scraper:      linkedInScraper,
		filter:       filter.NewFilter(policy.Filters, policy.Notification, logger),
		policy:       policy,
		rateLimiter:  rateLimiter,
		logger:       logger,
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

	response, err := e.openAIClient.ChatCompletion(prompt, e.policy.Evaluation.MaxTokens.Initial)
	if err != nil {
		e.logger.Error("Error evaluating job: %v", err)
		job.Score = nil
		job.Reason = "Failed to evaluate job due to an error."
		return job, nil
	}

	e.logger.Debug("Raw evaluation response for %s: %s", job.Position, response)
	score, reason, reasons := e.parseEvaluationResponse(response)
	job.Score = score
	job.Reason = reason   // For backward compatibility
	job.Reasons = reasons // Store all reasons

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

	response, err := e.openAIClient.ChatCompletion(prompt, e.policy.Evaluation.MaxTokens.Final)
	if err != nil {
		e.logger.Error("Error in final evaluation: %v", err)
		return job, err
	}

	e.logger.Debug("Raw final evaluation response for %s: %s", job.Position, response)
	finalScore, shouldSendEmail, finalReason, finalReasons := e.parseFinalEvaluationResponse(response)
	job.FinalScore = finalScore
	job.ShouldSendEmail = shouldSendEmail
	job.FinalReason = finalReason   // For backward compatibility
	job.FinalReasons = finalReasons // Store all reasons

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

// BatchEvaluateJobs evaluates multiple jobs in a single LLM call for efficiency
func (e *Evaluator) BatchEvaluateJobs(jobs []*models.Job, batchSize int) ([]*models.Job, error) {
	if len(jobs) == 0 {
		return jobs, nil
	}

	var evaluatedJobs []*models.Job
	errorCount := 0

	// Process jobs in batches
	for i := 0; i < len(jobs); i += batchSize {
		end := i + batchSize
		if end > len(jobs) {
			end = len(jobs)
		}

		batch := jobs[i:end]
		e.logger.Progress(i+1, len(jobs), "Batch evaluating jobs %d-%d", i+1, end)

		// Apply rate limiting before API call
		if err := e.rateLimiter.Acquire(); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		batchPrompt := e.getBatchInitialPrompt(batch)
		response, err := e.openAIClient.ChatCompletion(batchPrompt, e.policy.Evaluation.MaxTokens.Batch)
		if err != nil {
			errorCount++
			e.logger.Error("Batch evaluation failed for jobs %d-%d: %v", i+1, end, err)
			// Fall back to individual evaluation for this batch
			individualResults := e.fallbackToIndividualEvaluation(batch)
			evaluatedJobs = append(evaluatedJobs, individualResults...)
			continue
		}

		// Parse batch response
		batchResults := e.parseBatchEvaluationResponse(response, batch)
		evaluatedJobs = append(evaluatedJobs, batchResults...)
	}

	if errorCount > 0 {
		e.logger.Warning("⚠️  %d batch evaluation errors encountered", errorCount)
	}

	return evaluatedJobs, nil
}

// getBatchInitialPrompt creates a prompt for evaluating multiple jobs at once
func (e *Evaluator) getBatchInitialPrompt(jobs []*models.Job) string {
	jobsText := ""
	for i, job := range jobs {
		jobsText += fmt.Sprintf(`Job %d: "%s" at %s (%s)
`, i+1, job.Position, job.Company, job.Location)
	}

	return renderTemplate(e.policy.Evaluation.BatchPromptTemplate, map[string]string{
		"JOBS": jobsText,
	})
}

// parseBatchEvaluationResponse parses responses for multiple jobs
func (e *Evaluator) parseBatchEvaluationResponse(response string, jobs []*models.Job) []*models.Job {
	cleanedResponse := e.cleanResponse(response)
	e.logger.Debug("Parsing batch evaluation response: %s", cleanedResponse)

	// Try to parse as JSON array
	type batchEvalResp struct {
		JobId     int      `json:"jobId"`
		Score     float64  `json:"score"`
		Recommend bool     `json:"recommend"`
		Reasons   []string `json:"reasons"`
	}

	var batchResults []batchEvalResp
	if err := json.Unmarshal([]byte(cleanedResponse), &batchResults); err == nil {
		// Successfully parsed batch response
		resultMap := make(map[int]*batchEvalResp)
		for i := range batchResults {
			resultMap[batchResults[i].JobId] = &batchResults[i]
		}

		var evaluatedJobs []*models.Job
		for i, job := range jobs {
			jobIndex := i + 1 // 1-based indexing in prompt
			if result, exists := resultMap[jobIndex]; exists {
				// Apply the result to the job
				job.Score = &result.Score
				if len(result.Reasons) > 0 {
					job.Reason = result.Reasons[0]
					job.Reasons = result.Reasons
				} else {
					job.Reason = "No reason provided."
					job.Reasons = []string{"No reason provided."}
				}
				e.logger.JobDetail("✅ Batch scored %.1f: %s at %s", result.Score, job.Position, job.Company)
			} else {
				// No result found for this job
				e.logger.Warning("No batch result found for job %d", jobIndex)
				job.Score = nil
				job.Reason = "Batch evaluation failed."
				job.Reasons = []string{"Batch evaluation failed."}
			}
			evaluatedJobs = append(evaluatedJobs, job)
		}

		e.logger.Debug("Successfully parsed batch response for %d jobs", len(jobs))
		return evaluatedJobs
	}

	// Fallback: treat as single job response for the first job
	e.logger.Debug("Batch parsing failed, falling back to individual parsing")
	return e.fallbackToIndividualEvaluation(jobs)
}

// fallbackToIndividualEvaluation evaluates jobs individually when batch fails
func (e *Evaluator) fallbackToIndividualEvaluation(jobs []*models.Job) []*models.Job {
	var evaluatedJobs []*models.Job
	for _, job := range jobs {
		// Apply a simple evaluation or mark as failed
		job.Score = nil
		job.Reason = "Batch evaluation failed, individual evaluation not performed."
		job.Reasons = []string{"Batch evaluation failed, individual evaluation not performed."}
		evaluatedJobs = append(evaluatedJobs, job)
	}
	return evaluatedJobs
}

func (e *Evaluator) getInitialPrompt(job *models.Job) string {
	return renderTemplate(e.policy.Evaluation.InitialPromptTemplate, map[string]string{
		"POSITION": job.Position,
		"COMPANY":  job.Company,
		"LOCATION": job.Location,
	})
}

func (e *Evaluator) getFinalPrompt(job *models.Job, jobDesc, cv string) string {
	cleanCV := cleanCVForPrompt(cv, e.policy.Evaluation.CVPromptTruncation)
	return renderTemplate(e.policy.Evaluation.FinalPromptTemplate, map[string]string{
		"CV":             cleanCV,
		"POSITION":       job.Position,
		"COMPANY":        job.Company,
		"LOCATION":       job.Location,
		"JOB_DESCRIPTION": jobDesc,
	})
}

// Helper function to clean CV text
func cleanCVForPrompt(cv string, cfg config.CVPromptCfg) string {
	// Remove excessive whitespace
	cv = strings.Join(strings.Fields(cv), " ")

	// Truncate if too long (keep most relevant parts)
	if len(cv) > cfg.MaxLength {
		head := cfg.HeadLength
		tail := cfg.TailLength
		if head > len(cv) {
			head = len(cv)
		}
		if tail > len(cv)-head {
			tail = len(cv) - head
		}
		cv = cv[:head] + "\n\n[CV truncated for length]...\n\n" + cv[len(cv)-tail:]
	}

	// Remove any markdown or formatting that might confuse the LLM
	cv = strings.ReplaceAll(cv, "**", "")
	cv = strings.ReplaceAll(cv, "*", "")
	cv = strings.ReplaceAll(cv, "##", "")
	cv = strings.ReplaceAll(cv, "#", "")

	return cv
}

func (e *Evaluator) cleanResponse(response string) string {
	// Remove markdown code blocks (```json ... ```) - enable multiline matching with (?s)
	codeBlockPattern := regexp.MustCompile(`(?s)\x60{3}json\s*\n?(.*?)\n?\x60{3}`)
	if match := codeBlockPattern.FindStringSubmatch(response); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	// Remove regular code blocks (``` ... ```) - enable multiline matching with (?s)
	codeBlockPattern2 := regexp.MustCompile(`(?s)\x60{3}\s*\n?(.*?)\n?\x60{3}`)
	if match := codeBlockPattern2.FindStringSubmatch(response); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	return strings.TrimSpace(response)
}

// Structured response types with validation
type EvaluationResponse struct {
	Score     float64  `json:"score"`
	Recommend bool     `json:"recommend"`
	Reasons   []string `json:"reasons"`
}

type FinalEvaluationResponse struct {
	FinalScore  float64  `json:"finalScore"`
	ShouldApply bool     `json:"shouldApply"`
	Reasons     []string `json:"reasons"`
}

type ParsedEvaluationResult struct {
	Score   *float64
	Reason  string
	Reasons []string
}

type ParsedFinalEvaluationResult struct {
	FinalScore      *float64
	ShouldSendEmail bool
	FinalReason     string
	FinalReasons    []string
}

// validateEvaluationResponse validates the structure and content of evaluation response
func (e *Evaluator) validateEvaluationResponse(resp *EvaluationResponse) error {
	if resp.Score < 0 || resp.Score > 10 {
		return fmt.Errorf("score must be between 0 and 10, got %.2f", resp.Score)
	}
	if len(resp.Reasons) == 0 {
		return fmt.Errorf("reasons array cannot be empty")
	}
	if len(resp.Reasons) > 5 {
		return fmt.Errorf("too many reasons (max 5), got %d", len(resp.Reasons))
	}
	for _, reason := range resp.Reasons {
		if len(strings.TrimSpace(reason)) < 5 {
			return fmt.Errorf("reason too short: '%s'", reason)
		}
	}
	return nil
}

// validateFinalEvaluationResponse validates the structure and content of final evaluation response
func (e *Evaluator) validateFinalEvaluationResponse(resp *FinalEvaluationResponse) error {
	if resp.FinalScore < 0 || resp.FinalScore > 10 {
		return fmt.Errorf("finalScore must be between 0 and 10, got %.2f", resp.FinalScore)
	}
	if len(resp.Reasons) == 0 {
		return fmt.Errorf("reasons array cannot be empty")
	}
	if len(resp.Reasons) > 5 {
		return fmt.Errorf("too many reasons (max 5), got %d", len(resp.Reasons))
	}
	for _, reason := range resp.Reasons {
		if len(strings.TrimSpace(reason)) < 5 {
			return fmt.Errorf("reason too short: '%s'", reason)
		}
	}
	return nil
}

// parseStructuredEvaluationResponse attempts to parse with validation and fallbacks
func (e *Evaluator) parseStructuredEvaluationResponse(response string) *ParsedEvaluationResult {
	// Attempt 1: Direct JSON parsing
	var evalResp EvaluationResponse
	if err := json.Unmarshal([]byte(response), &evalResp); err == nil {
		if validateErr := e.validateEvaluationResponse(&evalResp); validateErr == nil {
			e.logger.Debug("Successfully parsed and validated JSON evaluation response")
			reason := "No reason provided."
			if len(evalResp.Reasons) > 0 {
				reason = evalResp.Reasons[0]
			}
			return &ParsedEvaluationResult{
				Score:   &evalResp.Score,
				Reason:  reason,
				Reasons: evalResp.Reasons,
			}
		} else {
			e.logger.Debug("JSON parsed but validation failed: %v", validateErr)
		}
	}

	// Attempt 2: Try fixing common JSON issues
	fixedResponse := e.fixCommonJSONIssues(response)
	if fixedResponse != response {
		var evalResp EvaluationResponse
		if err := json.Unmarshal([]byte(fixedResponse), &evalResp); err == nil {
			if validateErr := e.validateEvaluationResponse(&evalResp); validateErr == nil {
				e.logger.Debug("Successfully parsed fixed JSON evaluation response")
				reason := "No reason provided."
				if len(evalResp.Reasons) > 0 {
					reason = evalResp.Reasons[0]
				}
				return &ParsedEvaluationResult{
					Score:   &evalResp.Score,
					Reason:  reason,
					Reasons: evalResp.Reasons,
				}
			}
		}
	}

	// Attempt 3: Extract JSON from markdown or other wrappers
	if extractedJSON := e.extractJSONFromText(response); extractedJSON != "" {
		var evalResp EvaluationResponse
		if err := json.Unmarshal([]byte(extractedJSON), &evalResp); err == nil {
			if validateErr := e.validateEvaluationResponse(&evalResp); validateErr == nil {
				e.logger.Debug("Successfully parsed extracted JSON evaluation response")
				reason := "No reason provided."
				if len(evalResp.Reasons) > 0 {
					reason = evalResp.Reasons[0]
				}
				return &ParsedEvaluationResult{
					Score:   &evalResp.Score,
					Reason:  reason,
					Reasons: evalResp.Reasons,
				}
			}
		}
	}

	e.logger.Debug("All structured parsing attempts failed, falling back to regex")
	return nil
}

// parseStructuredFinalEvaluationResponse attempts to parse final evaluation with validation
func (e *Evaluator) parseStructuredFinalEvaluationResponse(response string) *ParsedFinalEvaluationResult {
	// Attempt 1: Direct JSON parsing
	var finalResp FinalEvaluationResponse
	if err := json.Unmarshal([]byte(response), &finalResp); err == nil {
		if validateErr := e.validateFinalEvaluationResponse(&finalResp); validateErr == nil {
			e.logger.Debug("Successfully parsed and validated JSON final evaluation response")
			reason := "No reason provided."
			if len(finalResp.Reasons) > 0 {
				reason = finalResp.Reasons[0]
			}
			return &ParsedFinalEvaluationResult{
				FinalScore:      &finalResp.FinalScore,
				ShouldSendEmail: finalResp.ShouldApply,
				FinalReason:     reason,
				FinalReasons:    finalResp.Reasons,
			}
		} else {
			e.logger.Debug("JSON parsed but validation failed: %v", validateErr)
		}
	}

	// Apply the same fixing and extraction logic as evaluation response
	fixedResponse := e.fixCommonJSONIssues(response)
	if fixedResponse != response {
		var finalResp FinalEvaluationResponse
		if err := json.Unmarshal([]byte(fixedResponse), &finalResp); err == nil {
			if validateErr := e.validateFinalEvaluationResponse(&finalResp); validateErr == nil {
				e.logger.Debug("Successfully parsed fixed JSON final evaluation response")
				reason := "No reason provided."
				if len(finalResp.Reasons) > 0 {
					reason = finalResp.Reasons[0]
				}
				return &ParsedFinalEvaluationResult{
					FinalScore:      &finalResp.FinalScore,
					ShouldSendEmail: finalResp.ShouldApply,
					FinalReason:     reason,
					FinalReasons:    finalResp.Reasons,
				}
			}
		}
	}

	if extractedJSON := e.extractJSONFromText(response); extractedJSON != "" {
		var finalResp FinalEvaluationResponse
		if err := json.Unmarshal([]byte(extractedJSON), &finalResp); err == nil {
			if validateErr := e.validateFinalEvaluationResponse(&finalResp); validateErr == nil {
				e.logger.Debug("Successfully parsed extracted JSON final evaluation response")
				reason := "No reason provided."
				if len(finalResp.Reasons) > 0 {
					reason = finalResp.Reasons[0]
				}
				return &ParsedFinalEvaluationResult{
					FinalScore:      &finalResp.FinalScore,
					ShouldSendEmail: finalResp.ShouldApply,
					FinalReason:     reason,
					FinalReasons:    finalResp.Reasons,
				}
			}
		}
	}

	e.logger.Debug("All structured final parsing attempts failed, falling back to regex")
	return nil
}

// fixCommonJSONIssues attempts to fix common JSON formatting issues
func (e *Evaluator) fixCommonJSONIssues(response string) string {
	fixed := response

	// Fix trailing commas
	fixed = regexp.MustCompile(`,(\s*[}\]])`).ReplaceAllString(fixed, "$1")

	// Fix single quotes to double quotes
	fixed = regexp.MustCompile(`'([^']*)'`).ReplaceAllString(fixed, `"$1"`)

	// Fix unquoted keys
	fixed = regexp.MustCompile(`(\w+):`).ReplaceAllString(fixed, `"$1":`)

	// Fix true/false capitalization
	fixed = regexp.MustCompile(`(?i)\btrue\b`).ReplaceAllString(fixed, "true")
	fixed = regexp.MustCompile(`(?i)\bfalse\b`).ReplaceAllString(fixed, "false")

	return fixed
}

// extractJSONFromText tries to find JSON objects within the text
func (e *Evaluator) extractJSONFromText(text string) string {
	// Look for JSON objects starting with { and ending with }
	jsonPattern := regexp.MustCompile(`\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	matches := jsonPattern.FindAllString(text, -1)

	for _, match := range matches {
		// Try to validate that this looks like a proper response
		if strings.Contains(match, "score") || strings.Contains(match, "finalScore") {
			return match
		}
	}

	return ""
}

func (e *Evaluator) parseEvaluationResponse(response string) (*float64, string, []string) {
	// Clean the response first
	cleanedResponse := e.cleanResponse(response)
	e.logger.Debug("Parsing evaluation response: %s", cleanedResponse)

	// Try structured parsing with validation
	if result := e.parseStructuredEvaluationResponse(cleanedResponse); result != nil {
		return result.Score, result.Reason, result.Reasons
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

	// Try structured parsing with validation
	if result := e.parseStructuredFinalEvaluationResponse(cleanedResponse); result != nil {
		return result.FinalScore, result.ShouldSendEmail, result.FinalReason, result.FinalReasons
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

// ValidateFinalEvaluation runs a second LLM check to confirm the final evaluation is justified
func (e *Evaluator) ValidateFinalEvaluation(job *models.Job) (bool, string, error) {
	if job.FinalScore == nil {
		return false, "No final score to validate", nil
	}
	prompt := e.getValidationPrompt(job)
	response, err := e.openAIClient.ChatCompletion(prompt, e.policy.Evaluation.MaxTokens.Validation)
	if err != nil {
		return false, "LLM validation error", err
	}

	// Clean the response first to handle markdown code blocks
	cleanedResponse := e.cleanResponse(response)
	e.logger.Debug("Parsing validation response: %s", cleanedResponse)

	type ValidationResult struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	var result ValidationResult
	if err := json.Unmarshal([]byte(cleanedResponse), &result); err != nil {
		e.logger.Debug("Validation JSON parsing failed: %v", err)
		return true, "Failed to parse LLM validation", err
	}
	return result.Valid, result.Reason, nil
}

func (e *Evaluator) getValidationPrompt(job *models.Job) string {
	cvText, _ := e.cvReader.GetCV()
	finalScore := 0.0
	if job.FinalScore != nil {
		finalScore = *job.FinalScore
	}
	shouldApply := job.ShouldSendEmail
	finalReasons := job.FinalReasons
	if len(finalReasons) == 0 && job.FinalReason != "" {
		finalReasons = []string{job.FinalReason}
	}

	// Shortened CV for validation
	shortCV := cleanCVForPrompt(cvText, e.policy.Evaluation.CVPromptTruncation)
	if len(shortCV) > e.policy.Evaluation.CVPromptTruncation.ValidationMaxSize {
		shortCV = shortCV[:e.policy.Evaluation.CVPromptTruncation.ValidationMaxSize] + "..."
	}
	return renderTemplate(e.policy.Evaluation.ValidationPromptTemplate, map[string]string{
		"CV":             shortCV,
		"JOB_DESCRIPTION": job.JobDescription,
		"FINAL_SCORE":    fmt.Sprintf("%.1f", finalScore),
		"SHOULD_APPLY":   strconv.FormatBool(shouldApply),
		"FINAL_REASONS":  fmt.Sprintf("%q", finalReasons),
	})
}

func renderTemplate(template string, values map[string]string) string {
	rendered := template
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return rendered
}
