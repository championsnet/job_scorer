//go:build unipdf

package cv

import (
	"os"
	"strings"

	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// unipdf is a large dependency and disabled by default. Build with -tags unipdf
// to include it (and set UNIPDF_LICENSE_KEY for full features).

func setUniPDFLicenseFromEnv() {
	if key := strings.TrimSpace(os.Getenv("UNIPDF_LICENSE_KEY")); key != "" {
		_ = license.SetMeteredKey(key)
	}
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
