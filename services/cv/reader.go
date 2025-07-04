package cv

import (
	"os"
	"strings"
	"sync"

	"job-scorer/utils"

	"github.com/gen2brain/go-fitz"
	"github.com/ledongthuc/pdf"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

func init() {
	// For personal use, unipdf works without a license key. If you have a key, set it here.
	_ = license.SetMeteredKey("")
}

type CVReader struct {
	cvPath string
	cvText string
	mutex  sync.RWMutex
	logger *utils.Logger
}

func NewCVReader(cvPath string) *CVReader {
	return &CVReader{
		cvPath: cvPath,
		logger: utils.NewLogger("CVReader"),
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

	// Try to extract text from PDF using unipdf first
	extractedText, err := c.extractTextWithUniPDF()
	if err != nil {
		c.logger.Warning("Failed to extract text with unipdf: %v, trying go-fitz", err)
		// Fallback to go-fitz
		extractedText, err = c.extractTextWithFitz()
		if err != nil {
			c.logger.Warning("Failed to extract text with go-fitz: %v, trying ledongthuc/pdf", err)
			// Fallback to original library
			extractedText, err = c.extractTextFromPDF()
			if err != nil {
				c.logger.Error("Error extracting text from PDF: %v", err)
				c.cvText = c.getFallbackCV()
				return c.cvText, nil
			}
		}
	}

	// Clean and validate extracted text
	cleanedText := c.cleanText(extractedText)
	if c.isValidText(cleanedText) {
		c.cvText = cleanedText
		c.logger.Info("CV loaded successfully")
		c.logger.Debug("CV preview: %s", c.getPreview(c.cvText, 200))
	} else {
		c.logger.Warning("Extracted text appears to be invalid or garbled, using fallback")
		c.cvText = c.getFallbackCV()
	}

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

func (c *CVReader) extractTextWithFitz() (string, error) {
	doc, err := fitz.New(c.cvPath)
	if err != nil {
		return "", err
	}
	defer doc.Close()

	var textBuilder strings.Builder
	totalPages := doc.NumPage()
	c.logger.Info("PDF has %d pages (go-fitz)", totalPages)

	for pageNum := 0; pageNum < totalPages; pageNum++ {
		// Extract text from the page
		text, err := doc.Text(pageNum)
		if err != nil {
			c.logger.Warning("Error extracting text from page %d: %v", pageNum, err)
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
	
	// Remove any non-printable characters except newlines and tabs
	var cleaned strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\t' || (r >= 32 && r <= 126) {
			cleaned.WriteRune(r)
		}
	}
	
	return cleaned.String()
}

func (c *CVReader) isValidText(text string) bool {
	if len(text) < 40 {
		return false
	}
	
	// Check if text contains expected keywords from a CV
	expectedKeywords := []string{"experience", "education", "skills", "work", "job", "company", "university", "degree", "marketing", "business", "development", "operations", "administration"}
	
	lowerText := strings.ToLower(text)
	keywordCount := 0
	
	for _, keyword := range expectedKeywords {
		if strings.Contains(lowerText, keyword) {
			keywordCount++
		}
	}
	
	// If we find at least 2 expected keywords, the text is likely valid
	return keywordCount >= 2
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
	return strings.TrimSpace(`
Vasiliki Ploumistou
Email: vas.ploumistou@gmail.com  | Phone: +41 77 210 03 06 | Location: Basel, Switzerland
LinkedIn: linkedin.com/in/vasiliki-ploumistou

⸻

CAREER OBJECTIVES:
To leverage my background in business development, marketing, and operations to contribute to high-impact projects within a forward-thinking organization.

PROFILE:
Dedicated and adaptable professional with 3+ years of experience in business development, marketing, and operations. A natural people-person who thrives on collaboration, takes initiative to solve challenges, and brings strong analytical and organizational skills to every role.

⸻

SKILLS:
• Business Development & Marketing: Client relations, market research, campaign analysis
• Digital Marketing & Communications: Social media strategy, influencer campaigns, content creation, SEO & SEM, performance tracking
• Project & Event Management: Project coordination, marketing logistics, event planning, partner/supplier liaison
• Strategic & Analytical Thinking: Market research, performance reporting, strategic planning
• Tools & Software: Microsoft Office (Excel, PowerPoint, Teams), Canva, CRM systems (Pipedrive), ERP systems (Odoo), Sprinklr, Dash Hudson

⸻

EXPERIENCE:
Business Development Assistant
SpiroChem AG, Basel, Switzerland | Jan 2025 – Present
• Support business development initiatives and client relationship management
• Process 20+ quotations and orders monthly via Odoo; coordinate logistics and supplier pricing
• Administer NDA/MSA/CDA contracts; maintain a 10 000+-record CRM
• Organize 6 international conferences per year, handling full logistics and marketing collateral
• Address weekly customer inquiries and contribute to strategic marketing plans

Social Media Marketing Intern
Rituals Cosmetics Enterprise, Amsterdam, Netherlands | Mar 2024 – Aug 2024
• Managed daily social media content and influencer campaigns using Sprinklr and Dash Hudson
• Analyzed influencer campaign performance; delivered detailed reports via Lefty
• Collaborated on cross-functional competitive analyses and brainstorming sessions
• Coordinated 5-person video production teams

Administrative Coordinator
Pushkin Education Center, Thessaloniki, Greece | Sep 2021 – Jan 2023
• Managed student schedules, communications, and issue resolution
• Supported managerial tasks and ensured clear information flow

Junior Marketing Specialist
Elcune, Thessaloniki, Greece | Feb 2020 – Jun 2020
• Conducted keyword research and implemented SEO strategies for Instagram
• Produced daily social media content; adapted quickly to shifting priorities

⸻

EDUCATION:
MSc Consultancy and Entrepreneurship
Rotterdam University of Applied Sciences | Sep 2023 – Oct 2024 (Pre-Master Business Administration)

BSc Social and Religious Studies
Aristotle University of Thessaloniki

⸻

PERSONAL:
Certifications
• Project Management Foundations, LinkedIn Learning
• SEO Foundations, LinkedIn Learning

Languages
• English (fluent)
• German (basic)
• Greek (native)
• Russian (basic)
    `)
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

func (c *CVReader) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cvText = ""
} 