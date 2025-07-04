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
  system := `You are an expert AI Career Advisor specializing in entry-level job matching for EU candidates in Switzerland.

CRITICAL: Respond with ONLY valid JSON in this exact format:
{
  "score": int,         // 0 (worst) to 10 (best)
  "recommend": bool,    // true if candidate should apply
  "reasons": [string]   // list of 1-2 specific rationale bullets
}

EVALUATION CRITERIA (in order of importance):
   
1. FIELD FIT (CRITICAL):
   - GOOD FITS: Marketing, Business Development, Operations, Administrative, Customer Success, Generalist, Strategy, HR
   - BAD FITS: Software Engineering, Data Science, Finance, Legal, Medical
   - If technical role (engineer, developer, scientist) → Score = 0-3
   
2. SENIORITY LEVEL (HIGH):
   - PERFECT: Entry-level, junior, intern, graduate, trainee, assistant
   - ACCEPTABLE: "up to 2 years", "0-3 years experience", "manager"
   - BAD: Senior, Lead, Director, "5+ years", "experienced"

SCORING GUIDE:
- 9-10: Perfect match (right field, entry-level, good location)
- 7-8: Good match (minor issues)
- 5-6: Acceptable (some concerns)
- 3-4: Poor match (major issues)
- 0-2: Bad match (wrong field, senior role)`
  
  user := fmt.Sprintf(`CANDIDATE PROFILE:
- Citizenship: EU (Greek passport)
- Target Locations: Basel, Zurich, remote, commutable
- Skills: Marketing, Business Development, Operations, Administrative work, Generalist, Strategy, HR
- Experience Level: Entry-level/Junior (0-2 years)

JOB TO EVALUATE:
- Title: %s
- Company: %s
- Location: %s

ANALYZE THIS JOB POSTING CAREFULLY:
1. Determine if the role fits marketing/BD/ops/admin/generalist/strategy/HR fields
2. Assess if location is suitable for Basel/Zurich area or remote/commutable
3. Evaluate if it's appropriate for entry-level candidate

RESPOND WITH JSON ONLY.`, job.Position, job.Company, job.Location)

  return system + "\n\n" + user
}

func (e *Evaluator) getFinalPrompt(job *models.Job, jobDesc, cv string) string {
  system := `You are an expert AI Career Advisor performing a detailed CV vs Job match analysis.

CRITICAL: Respond with ONLY valid JSON in this exact format:
{
  "finalScore": int,    // 0 (worst) to 10 (best)
  "shouldApply": bool,  // true if candidate should apply (score > 5)
  "reasons": [string]   // list of 1–3 specific rationale bullets
}

CV MATCHING CRITERIA (in order of importance):

1. LANGUAGE REQUIREMENT (CRITICAL - Score 0 if German required):
   - Scan job description for German language fluency requirements (or any other non-english language requirements)
   - RED FLAGS: "German required", "Deutsch erforderlich", "DACH", "German native", "Fluent German", "{non-english language} required"
   - If German required → finalScore = 0, shouldApply = false
   
2. SKILL MATCH (CRITICAL):
   - Look for specific skills in CV that match job requirements
   - Marketing skills: digital marketing, content creation, social media
   - Business Development: sales, partnerships, client relations, growth
   - Operations: project management, process improvement, coordination
   - Administrative: office management, data entry, executive support, contract management, logistics
   - Strategy: business strategy, market research, competitive analysis, business development, business planning
   - Business Support: business support, business operations, business administration, business management, business development, business planning, communications
   
3. EXPERIENCE LEVEL (HIGH):
   - CV shows 0-2 years experience → Perfect for entry-level roles
   - If job requires 3+ years → Score -2
   - If job requires 5+ years → Score -5
   
4. LOCATION & PERMIT (HIGH):
   - EU citizen can work in Switzerland
   - Basel/Zurich area or remote work
   - If location is far → Score -2
   
5. CAREER GROWTH POTENTIAL (MEDIUM):
   - Learning opportunities, mentorship, career progression
   - If role is dead-end → Score -1
   
6. COMPANY REPUTATION (MEDIUM):
   - Established vs startup/unknown company
   - Industry reputation and stability

SCORING GUIDE:
- 9-10: Perfect CV match (skills align, right level, good location)
- 7-8: Good CV match (minor skill gaps)
- 5-6: Acceptable CV match (some skill gaps)
- 3-4: Poor CV match (major skill gaps)
- 0-2: Bad CV match (German/Non-english language required, wrong skills, senior role)

IMPORTANT: If job description contains German language requirements, immediately return finalScore=0 and shouldApply=false.`

  // Clean and truncate CV for better processing
  cleanCV := cleanCVForPrompt(cv)
  
  user := fmt.Sprintf(`CANDIDATE CV:
%s

JOB DETAILS:
- Title: %s
- Company: %s
- Location: %s
- Description: %s

PERFORM DETAILED CV ANALYSIS:
1. Check for German language and any other non-english language requirements in job description
2. Compare CV skills with job requirements
3. Assess experience level compatibility
4. Evaluate location and permit requirements
5. Consider career growth potential
6. Analyze company fit

RESPOND WITH JSON ONLY.`, cleanCV, job.Position, job.Company, job.Location, jobDesc)

  return system + "\n\n" + user
}

// Helper function to clean CV text
func cleanCVForPrompt(cv string) string {
	// Remove excessive whitespace
	cv = strings.Join(strings.Fields(cv), " ")
	
	// Truncate if too long (keep most relevant parts)
	if len(cv) > 2000 {
		// Keep first 1500 chars and last 500 chars
		cv = cv[:1500] + "\n\n[CV truncated for length]...\n\n" + cv[len(cv)-500:]
	}
	
	// Remove any markdown or formatting that might confuse the LLM
	cv = strings.ReplaceAll(cv, "**", "")
	cv = strings.ReplaceAll(cv, "*", "")
	cv = strings.ReplaceAll(cv, "##", "")
	cv = strings.ReplaceAll(cv, "#", "")
	
	return cv
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

// ValidateFinalEvaluation runs a second LLM check to confirm the final evaluation is justified
func (e *Evaluator) ValidateFinalEvaluation(job *models.Job) (bool, string, error) {
	if job.FinalScore == nil {
		return false, "No final score to validate", nil
	}
	prompt := e.getValidationPrompt(job)
	response, err := e.groqClient.ChatCompletion(prompt, 256)
	if err != nil {
		return false, "LLM validation error", err
	}
	type ValidationResult struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	var result ValidationResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return false, "Failed to parse LLM validation", err
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
	return fmt.Sprintf(`
You are a critical reviewer for an AI job-matching system. Your task is to validate the following job recommendation for accuracy and relevance.

CANDIDATE CV:
%s

JOB DESCRIPTION:
%s

PREVIOUS AI EVALUATION:
{
  "finalScore": %.1f,
  "shouldApply": %t,
  "reasons": %q
}

INSTRUCTIONS:
- Double-check if the finalScore and shouldApply are justified.
- If the job is not a good fit (wrong field, seniority, language, or skill mismatch), set "valid" to false.
- If the recommendation is correct, set "valid" to true.
- Respond with ONLY valid JSON: { "valid": true/false, "reason": "..." }
`, cvText, job.JobDescription, finalScore, shouldApply, finalReasons)
} 