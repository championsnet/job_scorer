//go:build desktop

#import <Cocoa/Cocoa.h>

// Build a standard menu bar. The Edit menu's Cut/Copy/Paste/Select All items use
// the responder-chain selectors, so their Cmd-key equivalents are delivered to
// the focused WKWebView text field — which is what makes the shortcuts work.
void installEditMenu(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        NSMenu *mainMenu = [app mainMenu];
        if (mainMenu == nil) {
            mainMenu = [[NSMenu alloc] init];
            [app setMainMenu:mainMenu];
        }

        // Application menu (gives us Cmd+Q as well).
        NSMenuItem *appItem = [[NSMenuItem alloc] init];
        [mainMenu addItem:appItem];
        NSMenu *appMenu = [[NSMenu alloc] init];
        [appItem setSubmenu:appMenu];
        [appMenu addItemWithTitle:@"Hide" action:@selector(hide:) keyEquivalent:@"h"];
        [appMenu addItemWithTitle:@"Quit" action:@selector(terminate:) keyEquivalent:@"q"];

        // Edit menu.
        NSMenuItem *editItem = [[NSMenuItem alloc] init];
        [mainMenu addItem:editItem];
        NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editItem setSubmenu:editMenu];
        [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
        NSMenuItem *redo = [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"z"];
        [redo setKeyEquivalentModifierMask:(NSEventModifierFlagCommand | NSEventModifierFlagShift)];
        [editMenu addItem:[NSMenuItem separatorItem]];
        [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    }
}
