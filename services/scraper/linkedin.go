package scraper

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"job-scorer/models"
	"job-scorer/utils"

	"github.com/PuerkitoBio/goquery"
)

type LinkedInScraper struct {
	client     *http.Client
	cache      *JobCache
	userAgents []string
	logger     *utils.Logger
}

type JobCache struct {
	cache map[string]CacheItem
	mutex sync.RWMutex
	ttl   time.Duration
}

type CacheItem struct {
	Data      interface{}
	Timestamp time.Time
}

type QueryOptions struct {
	Location         string
	Keyword          string
	DateSincePosted  string
	JobType          string
	RemoteFilter     string
	Salary           string
	ExperienceLevel  string
	SortBy           string
	Limit            int
	Page             int
}

func NewLinkedInScraper() *LinkedInScraper {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.1 Safari/605.1.15",
	}

	// Create a client that follows redirects
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Copy headers from original request
			for key, values := range via[0].Header {
				req.Header[key] = values
			}
			return nil
		},
		Jar: &cookieJar{cookies: make(map[string][]*http.Cookie)},
	}

	return &LinkedInScraper{
		client:     client,
		cache:      &JobCache{
			cache: make(map[string]CacheItem),
			ttl:   time.Hour,
		},
		userAgents: userAgents,
		logger:     utils.NewLogger("LinkedInScraper"),
	}
}

// Custom cookie jar to maintain cookies between requests
type cookieJar struct {
	cookies map[string][]*http.Cookie
	mu      sync.RWMutex
}

func (j *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies[u.Host] = cookies
}

func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.cookies[u.Host]
}

// getAPIHeaders returns headers optimized for API requests
func (s *LinkedInScraper) getAPIHeaders() http.Header {
	headers := http.Header{}
	headers.Set("User-Agent", s.getRandomUserAgent())
	headers.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Accept-Encoding", "gzip, deflate, br")
	headers.Set("Referer", "https://www.linkedin.com/jobs")
	headers.Set("X-Requested-With", "XMLHttpRequest")
	headers.Set("Connection", "keep-alive")
	headers.Set("Sec-Fetch-Dest", "empty")
	headers.Set("Sec-Fetch-Mode", "cors")
	headers.Set("Sec-Fetch-Site", "same-origin")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Pragma", "no-cache")
	// Add origin header
	headers.Set("Origin", "https://www.linkedin.com")
	return headers
}

// getHTMLHeaders returns headers optimized for HTML page requests
func (s *LinkedInScraper) getHTMLHeaders() http.Header {
	headers := http.Header{}
	headers.Set("User-Agent", s.getRandomUserAgent())
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	headers.Set("Accept-Language", "en-US,en;q=0.5")
	headers.Set("Accept-Encoding", "gzip, deflate, br")
	headers.Set("Connection", "keep-alive")
	headers.Set("Upgrade-Insecure-Requests", "1")
	return headers
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function to decompress gzip content
func (s *LinkedInScraper) decompressResponse(body []byte, contentEncoding string) ([]byte, error) {
	if strings.Contains(contentEncoding, "gzip") {
		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer gzipReader.Close()
		
		decompressed, err := io.ReadAll(gzipReader)
		if err != nil {
			return nil, fmt.Errorf("error decompressing gzip: %w", err)
		}
		s.logger.Info("Decompressed gzip content: %d -> %d bytes", len(body), len(decompressed))
		return decompressed, nil
	}
	return body, nil
}

func (s *LinkedInScraper) Query(options QueryOptions) ([]*models.Job, error) {
	var allJobs []*models.Job
	start := 0
	const batchSize = 10
	const maxConsecutiveErrors = 3
	consecutiveErrors := 0
	hasMore := true

	s.logger.Info("Starting paginated job fetching...")

	for hasMore {
		s.logger.Info("Fetching batch starting at position %d", start)
		
		jobs, err := s.fetchJobBatch(options, start)
		if err != nil {
			consecutiveErrors++
			s.logger.Error("Error fetching batch (attempt %d): %v", consecutiveErrors, err)
			
			if consecutiveErrors >= maxConsecutiveErrors {
				s.logger.Error("Max consecutive errors reached. Stopping pagination.")
				break
			}
			
			// Exponential backoff
			delay := time.Duration(math.Pow(2, float64(consecutiveErrors))) * time.Second
			s.logger.Info("Retrying after %v delay...", delay)
			time.Sleep(delay)
			continue
		}

		// Reset error counter on successful fetch
		consecutiveErrors = 0

		if len(jobs) == 0 {
			s.logger.Info("No more jobs found. Stopping pagination.")
			hasMore = false
			break
		}

		allJobs = append(allJobs, jobs...)
		s.logger.Info("Fetched %d jobs. Total: %d", len(jobs), len(allJobs))

		// Check if we should stop due to limit
		if options.Limit > 0 && len(allJobs) >= options.Limit {
			allJobs = allJobs[:options.Limit]
			s.logger.Info("Reached job limit of %d. Stopping pagination.", options.Limit)
			break
		}

		// Move to next batch
		start += batchSize

		// Add delay between requests (2-5 seconds)
		delay := time.Duration(1000+rand.Intn(2000)) * time.Millisecond
		s.logger.Info("Waiting %v before next batch...", delay)
		time.Sleep(delay)
	}

	s.logger.Info("Pagination completed. Total jobs fetched: %d", len(allJobs))
	return allJobs, nil
}

func (s *LinkedInScraper) fetchJobBatch(options QueryOptions, start int) ([]*models.Job, error) {
	// Build LinkedIn jobs API URL with start parameter
	baseURL := "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"
	params := url.Values{}

	if options.Keyword != "" {
		params.Add("keywords", strings.ReplaceAll(options.Keyword, " ", "+"))
	}
	if options.Location != "" {
		params.Add("geoId", options.Location)
	}
	if options.DateSincePosted != "" {
		params.Add("f_TPR", s.getDateSincePosted(options.DateSincePosted))
	}
	if options.JobType != "" {
		params.Add("f_JT", s.getJobType(options.JobType))
	}
	if options.RemoteFilter != "" {
		params.Add("f_WT", s.getRemoteFilter(options.RemoteFilter))
	}
	if options.ExperienceLevel != "" {
		params.Add("f_E", s.getExperienceLevel(options.ExperienceLevel))
	}
	
	// Add start parameter for pagination
	params.Add("start", strconv.Itoa(start))

	fullURL := baseURL + "?" + params.Encode()

	// Check cache first
	if cachedJobs := s.cache.Get(fullURL); cachedJobs != nil {
		if jobs, ok := cachedJobs.([]*models.Job); ok {
			s.logger.Info("Returning cached results for URL: %s", fullURL)
			return jobs, nil
		}
	}

	s.logger.Info("Fetching jobs from LinkedIn API: %s", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Use API headers
	req.Header = s.getAPIHeaders()

	maxRetries := 3
	var lastErr error
	var resp *http.Response

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Exponential backoff with jitter
			delay := time.Duration(math.Pow(2, float64(i))*float64(time.Second)) +
				time.Duration(rand.Int63n(int64(time.Second)))
			s.logger.Info("Retry %d after %v delay...", i+1, delay)
			time.Sleep(delay)
		}

		resp, err = s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("error making request (attempt %d): %w", i+1, err)
			continue
		}

		s.logger.Debug("Response Status: %d %s", resp.StatusCode, resp.Status)

		// Check response status
		if resp.StatusCode == http.StatusOK {
			break
		}

		// Read body for debugging on error
		if resp.Body != nil {
			bodyBytes, bodyErr := io.ReadAll(resp.Body)
			if bodyErr == nil {
				s.logger.Info("Error response body: %s", string(bodyBytes[:min(len(bodyBytes), 500)]))
			}
			resp.Body.Close()
		}

		// Handle specific status codes
		switch resp.StatusCode {
		case http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			// Should be handled by CheckRedirect
			continue
		case 999:
			lastErr = fmt.Errorf("LinkedIn bot detection (status 999). Try again later")
			// Longer delay for bot detection
			time.Sleep(10 * time.Second)
		case http.StatusTooManyRequests:
			lastErr = fmt.Errorf("rate limit exceeded (status 429)")
			// Even longer delay for rate limiting
			time.Sleep(15 * time.Second)
		default:
			lastErr = fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
	}

	if resp == nil || resp.StatusCode != http.StatusOK {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed to fetch jobs after %d retries", maxRetries)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Decompress if needed
	decompressedBody, err := s.decompressResponse(body, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, err
	}

	s.logger.Debug("Response body length: %d bytes", len(decompressedBody))

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(decompressedBody))
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %w", err)
	}

	jobs := s.parseJobs(doc)

	// Cache the results
	s.cache.Set(fullURL, jobs)

	s.logger.Info("Fetched %d jobs from batch", len(jobs))
	return jobs, nil
}

func (s *LinkedInScraper) parseJobs(doc *goquery.Document) []*models.Job {
	var jobs []*models.Job

	s.logger.Debug("Starting job parsing...")
	
	// Try multiple selectors
	selectors := []string{
		".job-search-card",
		".jobs-search__results-list li",
		"li[data-entity-urn]",
		".result-card",
		".job-result-card",
		"li",
	}

	for _, selector := range selectors {
		elements := doc.Find(selector)
		s.logger.Debug("Selector '%s' found %d elements", selector, elements.Length())
		
		if elements.Length() > 0 {
			elements.Each(func(i int, selection *goquery.Selection) {				
				job := s.parseJobCard(selection)
				if job != nil {
					jobs = append(jobs, job)
					s.logger.Debug("Successfully parsed job: %s at %s", job.Position, job.Company)
				}
			})
			
			if len(jobs) > 0 {
				break // Stop trying other selectors if we found jobs
			}
		}
	}

	s.logger.Debug("Total jobs parsed: %d", len(jobs))
	return jobs
}

func (s *LinkedInScraper) parseJobCard(selection *goquery.Selection) *models.Job {
	// Debug the selection
	html, _ := selection.Html()
	s.logger.Debug("Parsing job card HTML: %s", html[:min(len(html), 300)])

	// Try multiple selectors for each field
	titleSelectors := []string{
		"h3.base-search-card__title",
		".job-search-card__title",
		".base-search-card__title",
		"h3 a",
		"h3",
		".result-card__title",
		"a[data-tracking-control-name]",
	}

	var position string
	for _, selector := range titleSelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			position = strings.TrimSpace(element.Text())
			if position != "" {
				s.logger.Debug("Found position with selector '%s': %s", selector, position)
				break
			}
		}
	}

	if position == "" {
		s.logger.Debug("No position found, skipping job card")
		return nil
	}

	companySelectors := []string{
		"h4.base-search-card__subtitle",
		".job-search-card__subtitle",
		".base-search-card__subtitle",
		"h4",
		".result-card__subtitle",
		"a[data-tracking-control-name*='company']",
	}

	var company string
	for _, selector := range companySelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			company = strings.TrimSpace(element.Text())
			if company != "" {
				s.logger.Debug("Found company with selector '%s': %s", selector, company)
				break
			}
		}
	}

	locationSelectors := []string{
		".job-search-card__location",
		".job-result-card__location",
		".base-search-card__location",
		"[data-tracking-control-name*='location']",
	}

	var location string
	for _, selector := range locationSelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			location = strings.TrimSpace(element.Text())
			if location != "" {
				s.logger.Debug("Found location with selector '%s': %s", selector, location)
				break
			}
		}
	}

	// Extract job URL
	linkSelectors := []string{
		"a.base-card__full-link",
		".job-search-card__link",
		"h3 a",
		"a[href*='/jobs/view/']",
	}

	var jobURL string
	for _, selector := range linkSelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			if href, exists := element.Attr("href"); exists {
				jobURL = href
				if !strings.HasPrefix(jobURL, "http") {
					jobURL = "https://www.linkedin.com" + jobURL
				}
				s.logger.Debug("Found job URL with selector '%s': %s", selector, jobURL)
				break
			}
		}
	}

	// Extract company logo
	logoSelectors := []string{
		"img.job-search-card__logo",
		".company-logo",
		"img",
	}

	var companyLogo string
	for _, selector := range logoSelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			if src, exists := element.Attr("src"); exists {
				companyLogo = src
				s.logger.Debug("Found logo with selector '%s': %s", selector, companyLogo)
				break
			}
		}
	}

	// Extract date/time info
	timeSelectors := []string{
		".job-search-card__listdate",
		".job-result-card__listdate",
		"time",
	}

	var agoTime string
	for _, selector := range timeSelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			agoTime = strings.TrimSpace(element.Text())
			if agoTime != "" {
				s.logger.Debug("Found time with selector '%s': %s", selector, agoTime)
				break
			}
		}
	}

	// Extract salary if available
	salarySelectors := []string{
		".job-search-card__salary",
		".job-result-card__salary",
		"[data-tracking-control-name*='salary']",
	}

	salary := "Not specified"
	for _, selector := range salarySelectors {
		element := selection.Find(selector)
		if element.Length() > 0 {
			salaryText := strings.TrimSpace(element.Text())
			if salaryText != "" {
				salary = salaryText
				s.logger.Debug("Found salary with selector '%s': %s", selector, salary)
				break
			}
		}
	}

	// Create current date as posting date
	date := time.Now().Format("2006-01-02")

	s.logger.Debug("Creating job: position=%s, company=%s, location=%s", position, company, location)

	job, err := models.NewJob(position, company, location, date, salary, jobURL, companyLogo, agoTime)
	if err != nil {
		s.logger.Error("Error creating job: %v", err)
		return nil
	}

	return job
}

func (s *LinkedInScraper) FetchJobDescription(jobURL string) (string, error) {
	if jobURL == "" {
		return "", fmt.Errorf("job URL is empty")
	}

	// Check cache first
	if cachedDesc := s.cache.Get("desc_" + jobURL); cachedDesc != nil {
		if desc, ok := cachedDesc.(string); ok {
			return desc, nil
		}
	}

	s.logger.Info("Fetching job description from: %s", jobURL)

	// Add random delay
	time.Sleep(time.Duration(rand.Intn(2000)+500) * time.Millisecond)

	req, err := http.NewRequest("GET", jobURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Use HTML headers for job description
	req.Header = s.getHTMLHeaders()

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Decompress if needed
	decompressedBody, err := s.decompressResponse(body, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return "", err
	}

	s.logger.Info("Response body length: %d bytes", len(decompressedBody))
	s.logger.Info("Response body preview: %s", string(decompressedBody[:min(len(decompressedBody), 1000)]))

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(decompressedBody))
	if err != nil {
		return "", fmt.Errorf("error parsing HTML: %w", err)
	}

	// Try multiple selectors for job description
	selectors := []string{
		".show-more-less-html__markup",
		".jobs-description__content",
		".job-view-layout .jobs-description",
		".description__text",
	}

	var description string
	for _, selector := range selectors {
		desc := strings.TrimSpace(doc.Find(selector).Text())
		if desc != "" {
			description = desc
			break
		}
	}

	if description == "" {
		description = "Job description not available"
	}

	// Clean up the description
	description = s.cleanDescription(description)

	// Cache the result
	s.cache.Set("desc_"+jobURL, description)

	return description, nil
}

func (s *LinkedInScraper) cleanDescription(desc string) string {
	// Remove excessive whitespace
	re := regexp.MustCompile(`\s+`)
	desc = re.ReplaceAllString(desc, " ")
	
	// Trim and limit length
	desc = strings.TrimSpace(desc)
	if len(desc) > 5000 {
		desc = desc[:5000] + "..."
	}
	
	return desc
}

func (s *LinkedInScraper) getRandomUserAgent() string {
	return s.userAgents[rand.Intn(len(s.userAgents))]
}

func (s *LinkedInScraper) getDateSincePosted(dateSince string) string {
	dateRange := map[string]string{
		"past hour":     "r4000",
		"past 2 hours":  "r7600",
		"past month":    "r2592000",
		"past week":     "r604800",
		"past 24 hours": "r86400",
	}
	if val, ok := dateRange[strings.ToLower(dateSince)]; ok {
		return val
	}
	return ""
}

func (s *LinkedInScraper) getExperienceLevel(level string) string {
	experienceRange := map[string]string{
		"internship":   "1",
		"entry level":  "2",
		"associate":    "3",
		"senior":       "4",
		"director":     "5",
		"executive":    "6",
	}
	if val, ok := experienceRange[strings.ToLower(level)]; ok {
		return val
	}
	return ""
}

func (s *LinkedInScraper) getJobType(jobType string) string {
	jobTypeRange := map[string]string{
		"full time":  "F",
		"full-time":  "F",
		"part time":  "P",
		"part-time":  "P",
		"contract":   "C",
		"temporary":  "T",
		"volunteer":  "V",
		"internship": "I",
	}
	if val, ok := jobTypeRange[strings.ToLower(jobType)]; ok {
		return val
	}
	return ""
}

func (s *LinkedInScraper) getRemoteFilter(filter string) string {
	remoteFilterRange := map[string]string{
		"on-site": "1",
		"on site": "1",
		"remote":  "2",
		"hybrid":  "3",
	}
	if val, ok := remoteFilterRange[strings.ToLower(filter)]; ok {
		return val
	}
	return ""
}

// Cache methods
func (c *JobCache) Set(key string, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cache[key] = CacheItem{
		Data:      value,
		Timestamp: time.Now(),
	}
}

func (c *JobCache) Get(key string) interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	item, exists := c.cache[key]
	if !exists {
		return nil
	}
	
	if time.Since(item.Timestamp) > c.ttl {
		delete(c.cache, key)
		return nil
	}
	
	return item.Data
}

func (c *JobCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	now := time.Now()
	for key, item := range c.cache {
		if now.Sub(item.Timestamp) > c.ttl {
			delete(c.cache, key)
		}
	}
} 