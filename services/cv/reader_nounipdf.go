//go:build !unipdf

package cv

import "fmt"

// Default build: the heavy unipdf dependency is not included. PDF text is
// extracted with the pure-Go ledongthuc parser instead; the "unipdf" entry in
// parserOrder is simply skipped. Build with -tags unipdf to enable it.

func setUniPDFLicenseFromEnv() {}

func (c *CVReader) extractTextWithUniPDF() (string, error) {
	return "", fmt.Errorf("unipdf parser is not included in this build (build with -tags unipdf)")
}
