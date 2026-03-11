package cv

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"job-scorer/config"
	"job-scorer/utils"

	"github.com/ledongthuc/pdf"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

type CVReader struct {
	cvPath string
	cvText string
	mutex  sync.RWMutex
	logger *utils.Logger
	policy config.CVPolicy
}

func NewCVReader(cvPath string, policy config.CVPolicy) *CVReader {
	if key := strings.TrimSpace(os.Getenv("UNIPDF_LICENSE_KEY")); key != "" {
		_ = license.SetMeteredKey(key)
	}
	return &CVReader{
		cvPath: cvPath,
		logger: utils.NewLogger("CVReader"),
		policy: policy,
	}
}

func (c *CVReader) LoadCV() (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Return cached CV text if already loaded
	if c.cvText != "" {
		return c.cvText, nil
	}

	c.logger.Info("Loading CV from: %s", c.cvPath)

	// Check if file exists
	if _, err := os.Stat(c.cvPath); os.IsNotExist(err) {
		c.logger.Error("CV file not found: %v", err)
		return c.getFallbackCV(), nil
	}

	// Support plain-text CV formats directly (e.g. Markdown).
	if c.isTextCVFile() {
		raw, err := os.ReadFile(c.cvPath)
		if err != nil {
			c.logger.Error("Failed to read CV text file: %v", err)
			c.cvText = c.getFallbackCV()
			return c.cvText, nil
		}

		cleaned := c.cleanText(string(raw))
		if !c.isValidText(cleaned) {
			c.logger.Warning("CV text file content looks invalid, using fallback CV")
			c.cvText = c.getFallbackCV()
			return c.cvText, nil
		}

		c.cvText = cleaned
		if c.policy.LogParserUsed {
			c.logger.Info("CV loaded successfully using text parser (%d chars)", len(c.cvText))
		}
		c.logger.Debug("CV preview: %s", c.getPreview(c.cvText, 200))
		return c.cvText, nil
	}

	type extractionCandidate struct {
		source string
		text   string
	}

	// Try all available extractors and keep the best candidate.
	var candidates []extractionCandidate

	for _, parserName := range c.policy.ParserOrder {
		parser := strings.ToLower(strings.TrimSpace(parserName))
		var extractedText string
		var err error
		switch parser {
		case "unipdf":
			if !c.shouldUseUniPDF() {
				continue
			}
			extractedText, err = c.extractTextWithUniPDF()
		case "ledongthuc":
			extractedText, err = c.extractTextFromPDF()
		default:
			continue
		}
		if err != nil {
			c.logger.Warning("Failed to extract text with %s: %v", parser, err)
			continue
		}
		if cleaned := c.cleanText(extractedText); cleaned != "" {
			candidates = append(candidates, extractionCandidate{source: parser, text: cleaned})
		}
	}

	if len(candidates) == 0 {
		c.logger.Error("Could not extract text from CV PDF with any parser, using fallback CV")
		c.cvText = c.getFallbackCV()
		return c.cvText, nil
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		// Prefer valid extraction; otherwise prefer longer text.
		bestValid := c.isValidText(best.text)
		candidateValid := c.isValidText(candidate.text)
		if (candidateValid && !bestValid) || (candidateValid == bestValid && len(candidate.text) > len(best.text)) {
			best = candidate
		}
	}

	c.cvText = best.text
	if c.policy.LogParserUsed {
		c.logger.Info("CV loaded successfully using %s parser (%d chars)", best.source, len(c.cvText))
	}
	c.logger.Debug("CV preview: %s", c.getPreview(c.cvText, 200))

	return c.cvText, nil
}

func (c *CVReader) extractTextWithUniPDF() (string, error) {
	f, err := os.Open(c.cvPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReader(f)
	if err != nil {
		return "", err
	}
	n, err := pdfReader.GetNumPages()
	if err != nil {
		return "", err
	}
	c.logger.Info("PDF has %d pages (unipdf)", n)

	var textBuilder strings.Builder
	for i := 1; i <= n; i++ {
		page, err := pdfReader.GetPage(i)
		if err != nil {
			c.logger.Warning("Error getting page %d: %v", i, err)
			continue
		}
		ex, err := extractor.New(page)
		if err != nil {
			c.logger.Warning("Error creating extractor for page %d: %v", i, err)
			continue
		}
		text, err := ex.ExtractText()
		if err != nil {
			c.logger.Warning("Error extracting text from page %d: %v", i, err)
			continue
		}
		textBuilder.WriteString(text)
		textBuilder.WriteString("\n")
	}
	return textBuilder.String(), nil
}

func (c *CVReader) extractTextFromPDF() (string, error) {
	// Open PDF file
	file, reader, err := pdf.Open(c.cvPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var textBuilder strings.Builder

	// Extract text from all pages
	totalPages := reader.NumPage()
	c.logger.Info("PDF has %d pages (ledongthuc/pdf)", totalPages)

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Extract text content from the page
		text, err := page.GetPlainText(nil)
		if err != nil {
			c.logger.Warning("Error extracting text from page %d: %v", pageNum, err)
			continue
		}

		textBuilder.WriteString(text)
		textBuilder.WriteString("\n")
	}

	return textBuilder.String(), nil
}

func (c *CVReader) cleanText(text string) string {
	// Remove excessive whitespace
	text = strings.TrimSpace(text)

	// Replace multiple newlines with single newlines
	text = strings.ReplaceAll(text, "\n\n\n", "\n\n")

	// Keep printable unicode text and remove control characters.
	var cleaned strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\t' || (unicode.IsPrint(r) && !unicode.IsControl(r)) {
			cleaned.WriteRune(r)
		}
	}

	return cleaned.String()
}

func (c *CVReader) isValidText(text string) bool {
	if len(strings.TrimSpace(text)) < c.policy.MinValidTextLength {
		return false
	}

	// Heuristic: a parsed CV should contain enough letters and not be mostly symbols.
	total := 0
	letters := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) {
			letters++
		}
	}

	if total == 0 {
		return false
	}

	letterRatio := float64(letters) / float64(total)
	return letterRatio >= c.policy.MinLetterRatio
}

func (c *CVReader) GetCV() (string, error) {
	c.mutex.RLock()
	if c.cvText != "" {
		defer c.mutex.RUnlock()
		return c.cvText, nil
	}
	c.mutex.RUnlock()

	// If not loaded, load it
	return c.LoadCV()
}

func (c *CVReader) getFallbackCV() string {
	return strings.TrimSpace(c.policy.FallbackText)
}

func (c *CVReader) getPreview(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength] + "..."
}

func (c *CVReader) IsLoaded() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.cvText != ""
}

func (c *CVReader) shouldUseUniPDF() bool {
	if c.policy.AllowEnvOverrideUniPDF {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_UNIPDF"))) {
		case "1", "true", "yes":
			return true
		}
	}
	if strings.TrimSpace(os.Getenv("UNIPDF_LICENSE_KEY")) != "" {
		return true
	}
	return c.policy.EnableUniPDF
}

func (c *CVReader) isTextCVFile() bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(c.cvPath)))
	switch ext {
	case ".md", ".markdown", ".txt":
		return true
	default:
		return false
	}
}

func (c *CVReader) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cvText = ""
}
