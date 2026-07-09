//go:build desktop

#import <Foundation/Foundation.h>
#import <PDFKit/PDFKit.h>
#include <stdlib.h>

// extractPDFTextMacOS returns the full text of a PDF using PDFKit, or NULL if it
// can't be opened / has no text. The caller frees the returned string.
char* extractPDFTextMacOS(const char* path) {
    @autoreleasepool {
        NSString *p = [NSString stringWithUTF8String:path];
        if (p == nil) return NULL;
        NSURL *url = [NSURL fileURLWithPath:p];
        PDFDocument *doc = [[PDFDocument alloc] initWithURL:url];
        if (doc == nil) return NULL;
        NSString *text = [doc string];
        if (text == nil) return NULL;
        const char *utf8 = [text UTF8String];
        if (utf8 == NULL) return NULL;
        return strdup(utf8);
    }
}
