package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// -----------------------------------------------------------------------------
// Text Editor Window ("Notepad")
// -----------------------------------------------------------------------------

type TextEditorWindow struct {
	id         int
	x, y, w, h int
	collapsed  bool
	path       string
	lines      []string
	cursorRow  int
	cursorCol  int
	scrollRow  int
	scrollCol  int
	modified   bool
	wm         *WindowManager
}

func NewTextEditorWindow(id int, path string, wm *WindowManager) *TextEditorWindow {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	te := &TextEditorWindow{
		id:        id,
		x:         60 + (id%8)*20,
		y:         70 + (id%8)*20,
		w:         560,
		h:         380,
		path:      absPath,
		lines:     []string{""},
		cursorRow: 0,
		cursorCol: 0,
		wm:        wm,
	}

	te.LoadFile()
	return te
}

func (te *TextEditorWindow) ID() int              { return te.id }
func (te *TextEditorWindow) Kind() WindowKind    { return KindTextEditor }
func (te *TextEditorWindow) Title() string       { return te.path }
func (te *TextEditorWindow) Bounds() (int, int, int, int) { return te.x, te.y, te.w, te.h }
func (te *TextEditorWindow) SetBounds(x, y, w, h int)    { te.x, te.y, te.w, te.h = x, y, w, h }
func (te *TextEditorWindow) IsCollapsed() bool   { return te.collapsed }
func (te *TextEditorWindow) SetCollapsed(c bool) { te.collapsed = c }
func (te *TextEditorWindow) OnClose()            {}

func (te *TextEditorWindow) LoadFile() {
	content, err := os.ReadFile(te.path)
	if err != nil {
		te.lines = []string{""}
		return
	}
	raw := strings.ReplaceAll(string(content), "\r\n", "\n")
	te.lines = strings.Split(raw, "\n")
	if len(te.lines) == 0 {
		te.lines = []string{""}
	}
	te.modified = false
}

func (te *TextEditorWindow) SaveFile() {
	data := strings.Join(te.lines, "\n")
	err := os.WriteFile(te.path, []byte(data), 0644)
	if err == nil {
		te.modified = false
	}
}

func (te *TextEditorWindow) HandleEvent(e Event) bool {
	if te.collapsed {
		return false
	}

	if e.Type == EventKeyDown {
		if e.Ctrl && e.Key == KEY_S {
			te.SaveFile()
			return true
		}

		switch e.Key {
		case KEY_UP:
			if te.cursorRow > 0 {
				te.cursorRow--
				if te.cursorCol > len(te.lines[te.cursorRow]) {
					te.cursorCol = len(te.lines[te.cursorRow])
				}
				if te.cursorRow < te.scrollRow {
					te.scrollRow = te.cursorRow
				}
			}
			return true
		case KEY_DOWN:
			if te.cursorRow < len(te.lines)-1 {
				te.cursorRow++
				if te.cursorCol > len(te.lines[te.cursorRow]) {
					te.cursorCol = len(te.lines[te.cursorRow])
				}
				maxVisible := (te.h - 65) / 18
				if te.cursorRow >= te.scrollRow+maxVisible {
					te.scrollRow = te.cursorRow - maxVisible + 1
				}
			}
			return true
		case KEY_LEFT:
			if te.cursorCol > 0 {
				te.cursorCol--
			} else if te.cursorRow > 0 {
				te.cursorRow--
				te.cursorCol = len(te.lines[te.cursorRow])
			}
			return true
		case KEY_RIGHT:
			if te.cursorCol < len(te.lines[te.cursorRow]) {
				te.cursorCol++
			} else if te.cursorRow < len(te.lines)-1 {
				te.cursorRow++
				te.cursorCol = 0
			}
			return true
		case KEY_ENTER:
			currLine := te.lines[te.cursorRow]
			left := currLine[:te.cursorCol]
			right := currLine[te.cursorCol:]
			te.lines[te.cursorRow] = left
			te.lines = append(te.lines[:te.cursorRow+1], append([]string{right}, te.lines[te.cursorRow+1:]...)...)
			te.cursorRow++
			te.cursorCol = 0
			te.modified = true
			return true
		case KEY_BACKSPACE:
			if te.cursorCol > 0 {
				currLine := te.lines[te.cursorRow]
				te.lines[te.cursorRow] = currLine[:te.cursorCol-1] + currLine[te.cursorCol:]
				te.cursorCol--
				te.modified = true
			} else if te.cursorRow > 0 {
				prevLen := len(te.lines[te.cursorRow-1])
				te.lines[te.cursorRow-1] += te.lines[te.cursorRow]
				te.lines = append(te.lines[:te.cursorRow], te.lines[te.cursorRow+1:]...)
				te.cursorRow--
				te.cursorCol = prevLen
				te.modified = true
			}
			return true
		}

		if e.Char >= 32 && e.Char <= 126 {
			currLine := te.lines[te.cursorRow]
			chStr := string(e.Char)
			te.lines[te.cursorRow] = currLine[:te.cursorCol] + chStr + currLine[te.cursorCol:]
			te.cursorCol++
			te.modified = true
			return true
		}
	}

	return false
}

func (te *TextEditorWindow) Draw(fb Framebuffer, active bool) {
	fb.FillRect(te.x+6, te.y+6, te.w, te.h, ColorShadow)
	fb.FillRect(te.x, te.y, te.w, te.h, ColorWinBg)
	fb.DrawRect(te.x, te.y, te.w, te.h, ColorWinBorder)

	titleBg := ColorTitleInact
	if active {
		titleBg = ColorTitleActive
	}
	fb.FillRect(te.x+1, te.y+1, te.w-2, 20, titleBg)

	titleStr := "Edit: " + te.path
	if te.modified {
		titleStr += " *"
	}
	fb.DrawText(te.x+8, te.y+3, titleStr, ColorTitleText, titleBg)

	fb.FillRect(te.x+te.w-36, te.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(te.x+te.w-36, te.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(te.x+te.w-32, te.y+2, "-", ColorText, ColorButtonBg)

	fb.FillRect(te.x+te.w-18, te.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(te.x+te.w-18, te.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(te.x+te.w-14, te.y+2, "X", ColorText, ColorButtonBg)

	if te.collapsed {
		return
	}

	// Toolbar Bar
	fb.FillRect(te.x+2, te.y+22, te.w-4, 20, ColorBarBg)
	fb.DrawLine(te.x+2, te.y+42, te.x+te.w-2, te.y+42, ColorWinBorder)
	fb.DrawText(te.x+10, te.y+24, "Save (Ctrl+S)", ColorText, ColorBarBg)

	// Editor Area
	editorY := te.y + 44
	maxVisible := (te.h - 65) / 18

	for i := 0; i < maxVisible; i++ {
		r := te.scrollRow + i
		if r >= len(te.lines) {
			break
		}

		lineY := editorY + i*18
		lineText := te.lines[r]
		fb.DrawText(te.x+10, lineY, lineText, ColorText, ColorWinBg)

		// Cursor
		if active && r == te.cursorRow {
			cx := te.x + 10 + te.cursorCol*8
			fb.FillRect(cx, lineY, 2, 16, ColorText)
		}
	}

	// Status Bar
	statusY := te.y + te.h - 20
	fb.FillRect(te.x+2, statusY, te.w-4, 18, ColorBarBg)
	fb.DrawLine(te.x+2, statusY, te.x+te.w-2, statusY, ColorWinBorder)

	statusStr := fmt.Sprintf("Ln %d, Col %d", te.cursorRow+1, te.cursorCol+1)
	fb.DrawText(te.x+6, statusY+2, statusStr, ColorText, ColorBarBg)
}

// -----------------------------------------------------------------------------
// Image Viewer Window
// -----------------------------------------------------------------------------

type ImageViewerWindow struct {
	id         int
	x, y, w, h int
	collapsed  bool
	path       string
	img        image.Image
	wm         *WindowManager
}

func NewImageViewerWindow(id int, path string, wm *WindowManager) *ImageViewerWindow {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	iv := &ImageViewerWindow{
		id:        id,
		x:         80 + (id%6)*20,
		y:         60 + (id%6)*20,
		w:         580,
		h:         420,
		path:      absPath,
		wm:        wm,
	}

	iv.LoadImage()
	return iv
}

func (iv *ImageViewerWindow) ID() int              { return iv.id }
func (iv *ImageViewerWindow) Kind() WindowKind    { return KindImageViewer }
func (iv *ImageViewerWindow) Title() string       { return iv.path }
func (iv *ImageViewerWindow) Bounds() (int, int, int, int) { return iv.x, iv.y, iv.w, iv.h }
func (iv *ImageViewerWindow) SetBounds(x, y, w, h int)    { iv.x, iv.y, iv.w, iv.h = x, y, w, h }
func (iv *ImageViewerWindow) IsCollapsed() bool   { return iv.collapsed }
func (iv *ImageViewerWindow) SetCollapsed(c bool) { iv.collapsed = c }
func (iv *ImageViewerWindow) OnClose()            {}

func (iv *ImageViewerWindow) LoadImage() {
	f, err := os.Open(iv.path)
	if err != nil {
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err == nil {
		iv.img = img
	}
}

func (iv *ImageViewerWindow) HandleEvent(e Event) bool {
	return false
}

func (iv *ImageViewerWindow) Draw(fb Framebuffer, active bool) {
	fb.FillRect(iv.x+6, iv.y+6, iv.w, iv.h, ColorShadow)
	fb.FillRect(iv.x, iv.y, iv.w, iv.h, ColorWinBg)
	fb.DrawRect(iv.x, iv.y, iv.w, iv.h, ColorWinBorder)

	titleBg := ColorTitleInact
	if active {
		titleBg = ColorTitleActive
	}
	fb.FillRect(iv.x+1, iv.y+1, iv.w-2, 20, titleBg)

	fb.DrawText(iv.x+8, iv.y+3, "Image: "+iv.path, ColorTitleText, titleBg)

	fb.FillRect(iv.x+iv.w-36, iv.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(iv.x+iv.w-36, iv.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(iv.x+iv.w-32, iv.y+2, "-", ColorText, ColorButtonBg)

	fb.FillRect(iv.x+iv.w-18, iv.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(iv.x+iv.w-18, iv.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(iv.x+iv.w-14, iv.y+2, "X", ColorText, ColorButtonBg)

	if iv.collapsed {
		return
	}

	viewY := iv.y + 22
	viewW := iv.w - 4
	viewH := iv.h - 44

	if iv.img != nil {
		fb.DrawImageScaled(iv.x+2, viewY, viewW, viewH, iv.img)
	} else {
		fb.DrawText(iv.x+20, viewY+20, "Failed to decode image", ColorText, ColorWinBg)
	}

	statusY := iv.y + iv.h - 20
	fb.FillRect(iv.x+2, statusY, iv.w-4, 18, ColorBarBg)
	fb.DrawLine(iv.x+2, statusY, iv.x+iv.w-2, statusY, ColorWinBorder)

	statusStr := "No Image Loaded"
	if iv.img != nil {
		b := iv.img.Bounds()
		statusStr = fmt.Sprintf("Resolution: %dx%d", b.Dx(), b.Dy())
	}
	fb.DrawText(iv.x+6, statusY+2, statusStr, ColorText, ColorBarBg)
}

// -----------------------------------------------------------------------------
// Help Window
// -----------------------------------------------------------------------------

type HelpWindow struct {
	id         int
	x, y, w, h int
	collapsed  bool
	wm         *WindowManager
}

func NewHelpWindow(id int, wm *WindowManager) *HelpWindow {
	return &HelpWindow{
		id:        id,
		x:         100,
		y:         80,
		w:         540,
		h:         360,
		wm:        wm,
	}
}

func (hw *HelpWindow) ID() int              { return hw.id }
func (hw *HelpWindow) Kind() WindowKind    { return KindHelp }
func (hw *HelpWindow) Title() string       { return "Help / Shortcuts" }
func (hw *HelpWindow) Bounds() (int, int, int, int) { return hw.x, hw.y, hw.w, hw.h }
func (hw *HelpWindow) SetBounds(x, y, w, h int)    { hw.x, hw.y, hw.w, hw.h = x, y, w, h }
func (hw *HelpWindow) IsCollapsed() bool   { return hw.collapsed }
func (hw *HelpWindow) SetCollapsed(c bool) { hw.collapsed = c }
func (hw *HelpWindow) OnClose()            {}

func (hw *HelpWindow) HandleEvent(e Event) bool { return false }

func (hw *HelpWindow) Draw(fb Framebuffer, active bool) {
	fb.FillRect(hw.x+6, hw.y+6, hw.w, hw.h, ColorShadow)
	fb.FillRect(hw.x, hw.y, hw.w, hw.h, ColorWinBg)
	fb.DrawRect(hw.x, hw.y, hw.w, hw.h, ColorWinBorder)

	titleBg := ColorTitleInact
	if active {
		titleBg = ColorTitleActive
	}
	fb.FillRect(hw.x+1, hw.y+1, hw.w-2, 20, titleBg)

	fb.DrawText(hw.x+8, hw.y+3, "LinFile GEOS File Manager - Help", ColorTitleText, titleBg)

	fb.FillRect(hw.x+hw.w-36, hw.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(hw.x+hw.w-36, hw.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(hw.x+hw.w-32, hw.y+2, "-", ColorText, ColorButtonBg)

	fb.FillRect(hw.x+hw.w-18, hw.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(hw.x+hw.w-18, hw.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(hw.x+hw.w-14, hw.y+2, "X", ColorText, ColorButtonBg)

	if hw.collapsed {
		return
	}

	shortcuts := []string{
		"Keyboard Shortcuts:",
		"  Super+E       - Open new file manager window",
		"  Super+K       - Close active window",
		"  Super+Q       - Quit LinFile",
		"  Super+Left/Right - Navigate folder history back/forward",
		"  Alt+Tab       - Cycle through open windows",
		"  Ctrl+X / C / V - Cut, Copy, Paste file/folder",
		"  Backspace     - Up to parent folder",
		"  Del           - Delete selected item",
		"",
		"Mouse Operations:",
		"  Right Click   - Context menu (Open, Edit, Copy, Delete, etc.)",
		"  Double Click  - Open folder or file in Editor / Viewer",
		"  Titlebar Drag - Move window (drag to edge for half/quarter snap)",
		"  [X] / [-]     - Close or collapse window",
	}

	for i, line := range shortcuts {
		fb.DrawText(hw.x+15, hw.y+30+i*18, line, ColorText, ColorWinBg)
	}
}
