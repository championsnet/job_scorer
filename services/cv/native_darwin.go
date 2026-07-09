//go:build desktop && darwin

package cv

/*
#cgo LDFLAGS: -framework Foundation -framework PDFKit
#include <stdlib.h>
char* extractPDFTextMacOS(const char* path);
*/
import "C"

import (
	"strings"
	"unsafe"
)

// nativePDFText extracts text from a PDF using macOS PDFKit, which handles far
// more real-world PDFs than the pure-Go parser.
func nativePDFText(path string) (string, bool) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	cstr := C.extractPDFTextMacOS(cpath)
	if cstr == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(cstr))
	text := C.GoString(cstr)
	return text, strings.TrimSpace(text) != ""
}
