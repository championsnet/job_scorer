//go:build !(desktop && darwin)

package cv

// nativePDFText: no OS-native PDF extractor on this platform/build. The reader
// falls back to the pure-Go parser.
func nativePDFText(_ string) (string, bool) { return "", false }
