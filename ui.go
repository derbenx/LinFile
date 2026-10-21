package main

import (
	"fmt"
	"image/color"
	"time"
)

// Classic GEOS / Windows 2.0 Color Palette
var (
	ColorDesktop    = color.RGBA{R: 0x00, G: 0x80, B: 0x80, A: 255} // Teal
	ColorWinBg      = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 255} // White
	ColorWinBorder  = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 255} // Black
	ColorTitleActive= color.RGBA{R: 0x80, G: 0x00, B: 0x80, A: 255} // Dark Magenta
	ColorTitleInact = color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 255} // Dark Gray
	ColorTitleText  = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 255} // White
	ColorText       = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 255} // Black
	ColorHighlight  = color.RGBA{R: 0x00, G: 0x00, B: 0x80, A: 255} // Dark Blue
	ColorHlText     = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 255} // White text on highlight
	ColorBarBg      = color.RGBA{R: 0xC0, G: 0xC0, B: 0xC0, A: 255} // Light Gray
	ColorShadow     = color.RGBA{R: 0x40, G: 0x40, B: 0x40, A: 255} // Dark Gray Shadow
	ColorButtonBg   = color.RGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 255} // Light Gray Button
)

type WindowKind int

const (
	KindFileManager WindowKind = iota
	KindTextEditor
	KindImageViewer
	KindHelp
)

type Window interface {
	ID() int
	Kind() WindowKind
	Title() string
	Bounds() (x, y, w, h int)
	SetBounds(x, y, w, h int)
	IsCollapsed() bool
	SetCollapsed(collapsed bool)
	Draw(fb Framebuffer, active bool)
	HandleEvent(e Event) bool
	OnClose()
}

type MenuItem struct {
	Label    string
	Action   func()
	Disabled bool
}

type Menu struct {
	X, Y, W, H int
	Items      []MenuItem
	HoverIndex int
}

type WindowManager struct {
	fb           Framebuffer
	windows      []Window
	activeWinID  int
	nextWinID    int
	contextMenu  *Menu
	altTabActive bool
	altTabIndex int

	// Drag state
	draggingWin Window
	dragOffX    int
	dragOffY    int
	resizingWin Window
	resizeSide  int // 1: bottom-right resize handle

	// Top menu / start bar
	startMenuOpen bool
	startMenu     *Menu
}

func NewWindowManager(fb Framebuffer) *WindowManager {
	wm := &WindowManager{
		fb:          fb,
		windows:     make([]Window, 0),
		nextWinID:   1,
		activeWinID: -1,
	}

	wm.startMenu = &Menu{
		X: 2, Y: 22, W: 180, H: 120,
		Items: []MenuItem{
			{Label: "New File Window (Super+E)", Action: func() { wm.OpenFileManagerWindow("/root") }},
			{Label: "Help (F1)", Action: func() { wm.OpenHelpWindow() }},
			{Label: "Quit LinFile (Super+Q)", Action: func() { panic("QUIT") }},
		},
	}

	return wm
}

func (wm *WindowManager) AddWindow(w Window) {
	wm.windows = append(wm.windows, w)
	wm.activeWinID = w.ID()
}

func (wm *WindowManager) RemoveWindow(id int) {
	for i, w := range wm.windows {
		if w.ID() == id {
			w.OnClose()
			wm.windows = append(wm.windows[:i], wm.windows[i+1:]...)
			if wm.activeWinID == id {
				if len(wm.windows) > 0 {
					wm.activeWinID = wm.windows[len(wm.windows)-1].ID()
				} else {
					wm.activeWinID = -1
				}
			}
			break
		}
	}
}

func (wm *WindowManager) ActiveWindow() Window {
	for _, w := range wm.windows {
		if w.ID() == wm.activeWinID {
			return w
		}
	}
	return nil
}

func (wm *WindowManager) ActivateWindow(id int) {
	wm.activeWinID = id
	for i, w := range wm.windows {
		if w.ID() == id {
			// Move to top of stack
			wm.windows = append(append(wm.windows[:i], wm.windows[i+1:]...), w)
			break
		}
	}
}

func (wm *WindowManager) NextWindow() {
	if len(wm.windows) < 2 {
		return
	}
	wm.altTabIndex = (wm.altTabIndex + 1) % len(wm.windows)
	wm.ActivateWindow(wm.windows[wm.altTabIndex].ID())
}

func (wm *WindowManager) PrevWindow() {
	if len(wm.windows) < 2 {
		return
	}
	wm.altTabIndex = (wm.altTabIndex - 1 + len(wm.windows)) % len(wm.windows)
	wm.ActivateWindow(wm.windows[wm.altTabIndex].ID())
}

func (wm *WindowManager) OpenContextMenu(x, y int, items []MenuItem) {
	h := len(items)*20 + 6
	w := 180
	if x+w > wm.fb.Width() {
		x = wm.fb.Width() - w - 2
	}
	if y+h > wm.fb.Height() {
		y = wm.fb.Height() - h - 2
	}
	wm.contextMenu = &Menu{
		X: x, Y: y, W: w, H: h,
		Items:      items,
		HoverIndex: -1,
	}
}

func (wm *WindowManager) CloseContextMenu() {
	wm.contextMenu = nil
}

func (wm *WindowManager) SnapWindow(w Window, snapType string) {
	sw, sh := wm.fb.Width(), wm.fb.Height()
	topMargin := 22
	usableH := sh - topMargin

	switch snapType {
	case "left":
		w.SetBounds(0, topMargin, sw/2, usableH)
	case "right":
		w.SetBounds(sw/2, topMargin, sw/2, usableH)
	case "top":
		w.SetBounds(0, topMargin, sw, usableH/2)
	case "bottom":
		w.SetBounds(0, topMargin+usableH/2, sw, usableH/2)
	case "top-left":
		w.SetBounds(0, topMargin, sw/2, usableH/2)
	case "top-right":
		w.SetBounds(sw/2, topMargin, sw/2, usableH/2)
	case "bottom-left":
		w.SetBounds(0, topMargin+usableH/2, sw/2, usableH/2)
	case "bottom-right":
		w.SetBounds(sw/2, topMargin+usableH/2, sw/2, usableH/2)
	}
}

func (wm *WindowManager) HandleEvent(e Event) {
	sw, sh := wm.fb.Width(), wm.fb.Height()

	// Alt+Tab handling
	if e.Alt && e.Key == KEY_TAB && e.Type == EventKeyDown {
		if !wm.altTabActive {
			wm.altTabActive = true
			wm.altTabIndex = 0
		}
		if e.Shift {
			wm.PrevWindow()
		} else {
			wm.NextWindow()
		}
		return
	}
	if !e.Alt && wm.altTabActive {
		wm.altTabActive = false
	}

	// Global Shortcuts
	if e.Type == EventKeyDown {
		// F1 opens Help Window
		if e.Key == KEY_F1 {
			wm.OpenHelpWindow()
			return
		}

		// Alt+F4 closes active window
		if e.Alt && e.Key == KEY_F4 {
			if active := wm.ActiveWindow(); active != nil {
				wm.RemoveWindow(active.ID())
			}
			return
		}

		// Ctrl+Alt+Q or Super+Q -> Prompt / Quit
		if (e.Ctrl && e.Alt && e.Key == KEY_Q) || (e.Super && e.Key == KEY_Q) {
			panic("QUIT")
		}

		// Super+E or Ctrl+Alt+E -> New file manager window
		if (e.Super && e.Key == KEY_E) || (e.Ctrl && e.Alt && e.Key == KEY_E) {
			wm.OpenFileManagerWindow(".")
			return
		}

		// Super+K or Ctrl+Alt+K -> Close window
		if (e.Super && e.Key == KEY_K) || (e.Ctrl && e.Alt && e.Key == KEY_K) {
			if active := wm.ActiveWindow(); active != nil {
				wm.RemoveWindow(active.ID())
			}
			return
		}

		// Ctrl+Alt+Arrows -> Snap active window to sides/halves
		if e.Ctrl && e.Alt {
			if active := wm.ActiveWindow(); active != nil {
				switch e.Key {
				case KEY_LEFT:
					wm.SnapWindow(active, "left")
					return
				case KEY_RIGHT:
					wm.SnapWindow(active, "right")
					return
				case KEY_UP:
					wm.SnapWindow(active, "top")
					return
				case KEY_DOWN:
					wm.SnapWindow(active, "bottom")
					return
				}
			}
		}

		// Ctrl+Shift+Arrows -> Nudge / Slide active window 20px
		if e.Ctrl && e.Shift {
			if active := wm.ActiveWindow(); active != nil {
				wx, wy, ww, wh := active.Bounds()
				switch e.Key {
				case KEY_LEFT:
					active.SetBounds(wx-20, wy, ww, wh)
					return
				case KEY_RIGHT:
					active.SetBounds(wx+20, wy, ww, wh)
					return
				case KEY_UP:
					active.SetBounds(wx, wy-20, ww, wh)
					return
				case KEY_DOWN:
					active.SetBounds(wx, wy+20, ww, wh)
					return
				}
			}
		}
	}

	// Context menu interaction
	if wm.contextMenu != nil {
		if e.Type == EventMouseMove {
			mx, my := e.MouseX, e.MouseY
			cm := wm.contextMenu
			if mx >= cm.X && mx < cm.X+cm.W && my >= cm.Y && my < cm.Y+cm.H {
				idx := (my - cm.Y - 3) / 20
				if idx >= 0 && idx < len(cm.Items) {
					cm.HoverIndex = idx
				} else {
					cm.HoverIndex = -1
				}
			} else {
				cm.HoverIndex = -1
			}
			return
		}
		if e.Type == EventMouseDown && e.MouseButton == MouseLeft {
			cm := wm.contextMenu
			mx, my := e.MouseX, e.MouseY
			if mx >= cm.X && mx < cm.X+cm.W && my >= cm.Y && my < cm.Y+cm.H {
				idx := (my - cm.Y - 3) / 20
				if idx >= 0 && idx < len(cm.Items) {
					item := cm.Items[idx]
					wm.CloseContextMenu()
					if item.Action != nil && !item.Disabled {
						item.Action()
					}
					return
				}
			}
			wm.CloseContextMenu()
			return
		}
		if e.Type == EventMouseDown && e.MouseButton == MouseRight {
			wm.CloseContextMenu()
		}
	}

	// Start bar interaction
	if e.Type == EventMouseDown && e.MouseButton == MouseLeft {
		if e.MouseY < 22 && e.MouseX >= 2 && e.MouseX <= 70 { // Start Button
			wm.startMenuOpen = !wm.startMenuOpen
			return
		}
	}

	if wm.startMenuOpen {
		if e.Type == EventMouseMove {
			sm := wm.startMenu
			if e.MouseX >= sm.X && e.MouseX < sm.X+sm.W && e.MouseY >= sm.Y && e.MouseY < sm.Y+sm.H {
				idx := (e.MouseY - sm.Y - 3) / 20
				if idx >= 0 && idx < len(sm.Items) {
					sm.HoverIndex = idx
				} else {
					sm.HoverIndex = -1
				}
			} else {
				sm.HoverIndex = -1
			}
			return
		}
		if e.Type == EventMouseDown && e.MouseButton == MouseLeft {
			sm := wm.startMenu
			if e.MouseX >= sm.X && e.MouseX < sm.X+sm.W && e.MouseY >= sm.Y && e.MouseY < sm.Y+sm.H {
				idx := (e.MouseY - sm.Y - 3) / 20
				if idx >= 0 && idx < len(sm.Items) {
					item := sm.Items[idx]
					wm.startMenuOpen = false
					if item.Action != nil {
						item.Action()
					}
					return
				}
			}
			wm.startMenuOpen = false
		}
	}

	// Window dragging & resizing
	if e.Type == EventMouseDown && e.MouseButton == MouseLeft {
		// Check top window down to bottom
		for i := len(wm.windows) - 1; i >= 0; i-- {
			w := wm.windows[i]
			wx, wy, ww, wh := w.Bounds()

			// Check window title bar drag or buttons
			if e.MouseX >= wx && e.MouseX < wx+ww && e.MouseY >= wy && e.MouseY < wy+20 {
				wm.ActivateWindow(w.ID())

				// Check Close button [X] at (wx+ww-18)
				if e.MouseX >= wx+ww-18 && e.MouseX < wx+ww-2 {
					wm.RemoveWindow(w.ID())
					return
				}
				// Check Collapse button [-] at (wx+ww-36)
				if e.MouseX >= wx+ww-36 && e.MouseX < wx+ww-20 {
					w.SetCollapsed(!w.IsCollapsed())
					return
				}

				// Title bar drag
				wm.draggingWin = w
				wm.dragOffX = e.MouseX - wx
				wm.dragOffY = e.MouseY - wy
				return
			}

			// Resize handle at bottom right corner (16x16)
			if !w.IsCollapsed() && e.MouseX >= wx+ww-16 && e.MouseX < wx+ww && e.MouseY >= wy+wh-16 && e.MouseY < wy+wh {
				wm.ActivateWindow(w.ID())
				wm.resizingWin = w
				wm.resizeSide = 1
				return
			}

			// Window body click activates window
			if e.MouseX >= wx && e.MouseX < wx+ww && e.MouseY >= wy && e.MouseY < wy+wh {
				wm.ActivateWindow(w.ID())
				w.HandleEvent(e)
				return
			}
		}
	}

	if e.Type == EventMouseMove {
		if wm.draggingWin != nil {
			nx := e.MouseX - wm.dragOffX
			ny := e.MouseY - wm.dragOffY
			_, _, ww, wh := wm.draggingWin.Bounds()

			// Screen snapping checks on release or Ctrl drag
			wm.draggingWin.SetBounds(nx, ny, ww, wh)
			return
		}

		if wm.resizingWin != nil {
			wx, wy, _, _ := wm.resizingWin.Bounds()
			nw := e.MouseX - wx
			nh := e.MouseY - wy
			if nw < 160 {
				nw = 160
			}
			if nh < 100 {
				nh = 100
			}
			wm.resizingWin.SetBounds(wx, wy, nw, nh)
			return
		}
	}

	if e.Type == EventMouseUp && e.MouseButton == MouseLeft {
		if wm.draggingWin != nil {
			mx, my := e.MouseX, e.MouseY
			// Snapping logic if dragged near edges or with Ctrl
			if e.Ctrl || mx <= 10 || mx >= sw-10 || my <= 25 || my >= sh-10 {
				if mx <= 20 && my <= 35 {
					wm.SnapWindow(wm.draggingWin, "top-left")
				} else if mx >= sw-20 && my <= 35 {
					wm.SnapWindow(wm.draggingWin, "top-right")
				} else if mx <= 20 && my >= sh-20 {
					wm.SnapWindow(wm.draggingWin, "bottom-left")
				} else if mx >= sw-20 && my >= sh-20 {
					wm.SnapWindow(wm.draggingWin, "bottom-right")
				} else if mx <= 10 {
					wm.SnapWindow(wm.draggingWin, "left")
				} else if mx >= sw-10 {
					wm.SnapWindow(wm.draggingWin, "right")
				} else if my <= 25 {
					wm.SnapWindow(wm.draggingWin, "top")
				} else if my >= sh-10 {
					wm.SnapWindow(wm.draggingWin, "bottom")
				}
			}
			wm.draggingWin = nil
		}
		wm.resizingWin = nil
	}

	// Dispatch event to active window
	if active := wm.ActiveWindow(); active != nil {
		active.HandleEvent(e)
	}
}

func (wm *WindowManager) DrawAll() {
	fb := wm.fb
	sw, sh := fb.Width(), fb.Height()

	// 1. Draw Desktop background (Teal with GEOS dither pattern)
	fb.DrawDitherPattern(0, 0, sw, sh, ColorDesktop, color.RGBA{0, 0x60, 0x60, 255})

	// 2. Draw Top Bar / Taskbar (GEOS Style)
	fb.FillRect(0, 0, sw, 22, ColorBarBg)
	fb.DrawLine(0, 21, sw, 21, ColorWinBorder)

	// Start / System Button
	startBg := ColorButtonBg
	if wm.startMenuOpen {
		startBg = ColorHighlight
	}
	fb.FillRect(2, 2, 68, 18, startBg)
	fb.DrawRect(2, 2, 68, 18, ColorWinBorder)
	if wm.startMenuOpen {
		fb.DrawText(8, 3, "LinFile", ColorHlText, startBg)
	} else {
		fb.DrawText(8, 3, "LinFile", ColorText, startBg)
	}

	// System Time display on right
	now := time.Now().Format("15:04:05")
	fb.DrawText(sw-75, 3, now, ColorText, ColorBarBg)

	// 3. Draw Windows stack
	for _, w := range wm.windows {
		active := (w.ID() == wm.activeWinID)
		w.Draw(fb, active)
	}

	// 4. Draw Start Menu if open
	if wm.startMenuOpen {
		wm.drawMenu(wm.startMenu)
	}

	// 5. Draw Context Menu if active
	if wm.contextMenu != nil {
		wm.drawMenu(wm.contextMenu)
	}

	// 6. Draw Alt+Tab Overlay if active
	if wm.altTabActive && len(wm.windows) > 0 {
		wm.drawAltTabOverlay()
	}
}

func (wm *WindowManager) drawMenu(m *Menu) {
	fb := wm.fb
	// Drop shadow
	fb.FillRect(m.X+4, m.Y+4, m.W, m.H, ColorShadow)
	// Menu container
	fb.FillRect(m.X, m.Y, m.W, m.H, ColorWinBg)
	fb.DrawRect(m.X, m.Y, m.W, m.H, ColorWinBorder)

	for i, item := range m.Items {
		itemY := m.Y + 3 + i*20
		bg := ColorWinBg
		fg := ColorText

		if item.Disabled {
			fg = ColorTitleInact
		} else if i == m.HoverIndex {
			bg = ColorHighlight
			fg = ColorHlText
		}

		fb.FillRect(m.X+2, itemY, m.W-4, 18, bg)
		fb.DrawText(m.X+6, itemY+1, item.Label, fg, bg)
	}
}

func (wm *WindowManager) drawAltTabOverlay() {
	fb := wm.fb
	sw, sh := fb.Width(), fb.Height()
	ow, oh := 360, 180
	ox, oy := (sw-ow)/2, (sh-oh)/2

	fb.FillRect(ox+6, oy+6, ow, oh, ColorShadow)
	fb.FillRect(ox, oy, ow, oh, ColorWinBg)
	fb.DrawRect(ox, oy, ow, oh, ColorWinBorder)

	fb.FillRect(ox+2, oy+2, ow-4, 20, ColorTitleActive)
	fb.DrawText(ox+10, oy+4, "Switch Window (Alt+Tab)", ColorTitleText, ColorTitleActive)

	for i, w := range wm.windows {
		wy := oy + 28 + i*22
		bg := ColorWinBg
		fg := ColorText
		if i == wm.altTabIndex {
			bg = ColorHighlight
			fg = ColorHlText
		}
		fb.FillRect(ox+10, wy, ow-20, 20, bg)
		kindName := "Folder"
		switch w.Kind() {
		case KindTextEditor:
			kindName = "Text Editor"
		case KindImageViewer:
			kindName = "Image Viewer"
		case KindHelp:
			kindName = "Help"
		}
		lbl := fmt.Sprintf("[%s] %s", kindName, w.Title())
		fb.DrawText(ox+15, wy+2, lbl, fg, bg)
	}
}

// Helper methods to spawn windows
func (wm *WindowManager) OpenFileManagerWindow(path string) {
	fmWin := NewFileManagerWindow(wm.nextWinID, path, wm)
	wm.nextWinID++
	wm.AddWindow(fmWin)
}

func (wm *WindowManager) OpenTextEditorWindow(path string) {
	teWin := NewTextEditorWindow(wm.nextWinID, path, wm)
	wm.nextWinID++
	wm.AddWindow(teWin)
}

func (wm *WindowManager) OpenImageViewerWindow(path string) {
	imgWin := NewImageViewerWindow(wm.nextWinID, path, wm)
	wm.nextWinID++
	wm.AddWindow(imgWin)
}

func (wm *WindowManager) OpenHelpWindow() {
	helpWin := NewHelpWindow(wm.nextWinID, wm)
	wm.nextWinID++
	wm.AddWindow(helpWin)
}
