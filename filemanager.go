package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileItem represents a file or directory entry
type FileItem struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// Global Clipboard
type Clipboard struct {
	SourcePath string
	IsCut      bool
}

var globalClipboard Clipboard

// ReadDirectory reads all entries from a folder path and returns sorted FileItems
func ReadDirectory(dirPath string) ([]FileItem, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var items []FileItem
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, FileItem{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort directories first, then files alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items, nil
}

func FormatFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024.0)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024.0*1024.0))
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1024.0*1024.0*1024.0))
}

func CopyFileOrDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else {
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

type FileManagerWindow struct {
	id          int
	x, y, w, h  int
	collapsed   bool
	path        string
	items       []FileItem
	selectedIdx map[int]bool
	focusedIdx  int
	scrollOff   int
	wm          *WindowManager

	// Path navigation history
	history     []string
	historyIdx  int

	// Input editing for path box
	editingPath bool
	pathBuffer  string
}

func NewFileManagerWindow(id int, path string, wm *WindowManager) *FileManagerWindow {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	fm := &FileManagerWindow{
		id:          id,
		x:           40 + (id%10)*20,
		y:           50 + (id%10)*20,
		w:           520,
		h:           360,
		path:        absPath,
		selectedIdx: make(map[int]bool),
		wm:          wm,
		history:     []string{absPath},
		historyIdx:  0,
	}

	fm.Refresh()
	return fm
}

func (fm *FileManagerWindow) ID() int              { return fm.id }
func (fm *FileManagerWindow) Kind() WindowKind    { return KindFileManager }
func (fm *FileManagerWindow) Title() string       { return fm.path }
func (fm *FileManagerWindow) Bounds() (int, int, int, int) { return fm.x, fm.y, fm.w, fm.h }
func (fm *FileManagerWindow) SetBounds(x, y, w, h int)    { fm.x, fm.y, fm.w, fm.h = x, y, w, h }
func (fm *FileManagerWindow) IsCollapsed() bool   { return fm.collapsed }
func (fm *FileManagerWindow) SetCollapsed(c bool) { fm.collapsed = c }
func (fm *FileManagerWindow) OnClose()            {}

func (fm *FileManagerWindow) Refresh() {
	items, err := ReadDirectory(fm.path)
	if err != nil {
		fm.items = nil
		return
	}
	fm.items = items
	fm.selectedIdx = make(map[int]bool)
	if fm.focusedIdx >= len(fm.items) {
		fm.focusedIdx = len(fm.items) - 1
	}
	if fm.focusedIdx < 0 {
		fm.focusedIdx = 0
	}
}

func (fm *FileManagerWindow) NavigateTo(newPath string) {
	absPath, err := filepath.Abs(newPath)
	if err != nil {
		absPath = newPath
	}

	if absPath == fm.path {
		return
	}

	// Truncate forward history if navigating to a new path
	fm.history = append(fm.history[:fm.historyIdx+1], absPath)
	fm.historyIdx++
	fm.path = absPath
	fm.focusedIdx = 0
	fm.scrollOff = 0
	fm.Refresh()
}

func (fm *FileManagerWindow) GoBackHistory() {
	if fm.historyIdx > 0 {
		fm.historyIdx--
		fm.path = fm.history[fm.historyIdx]
		fm.focusedIdx = 0
		fm.scrollOff = 0
		fm.Refresh()
	}
}

func (fm *FileManagerWindow) GoForwardHistory() {
	if fm.historyIdx < len(fm.history)-1 {
		fm.historyIdx++
		fm.path = fm.history[fm.historyIdx]
		fm.focusedIdx = 0
		fm.scrollOff = 0
		fm.Refresh()
	}
}

func (fm *FileManagerWindow) GoUpParent() {
	parent := filepath.Dir(fm.path)
	if parent != fm.path {
		fm.NavigateTo(parent)
	}
}

func (fm *FileManagerWindow) HandleEvent(e Event) bool {
	if fm.collapsed {
		return false
	}

	// Hotkeys for file window
	if e.Type == EventKeyDown {
		if e.Super {
			switch e.Key {
			case KEY_LEFT:
				fm.GoBackHistory()
				return true
			case KEY_RIGHT:
				fm.GoForwardHistory()
				return true
			}
		}

		if e.Ctrl {
			switch e.Key {
			case KEY_C:
				fm.CopySelected(false)
				return true
			case KEY_X:
				fm.CopySelected(true)
				return true
			case KEY_V:
				fm.PasteClipboard()
				return true
			}
		}

		switch e.Key {
		case KEY_BACKSPACE:
			if !fm.editingPath {
				fm.GoUpParent()
				return true
			}
		case KEY_UP:
			if fm.focusedIdx > 0 {
				fm.focusedIdx--
				if fm.focusedIdx < fm.scrollOff {
					fm.scrollOff = fm.focusedIdx
				}
			}
			return true
		case KEY_DOWN:
			if fm.focusedIdx < len(fm.items)-1 {
				fm.focusedIdx++
				maxVisible := (fm.h - 80) / 18
				if fm.focusedIdx >= fm.scrollOff+maxVisible {
					fm.scrollOff = fm.focusedIdx - maxVisible + 1
				}
			}
			return true
		case KEY_ENTER:
			if fm.focusedIdx >= 0 && fm.focusedIdx < len(fm.items) {
				item := fm.items[fm.focusedIdx]
				itemPath := filepath.Join(fm.path, item.Name)
				if item.IsDir {
					fm.NavigateTo(itemPath)
				} else {
					fm.OpenItem(itemPath)
				}
			}
			return true
		case KEY_DELETE:
			fm.DeleteSelected()
			return true
		}
	}

	// Mouse clicks in window
	if e.Type == EventMouseDown {
		mx, my := e.MouseX, e.MouseY

		// Right click opens Context Menu
		if e.MouseButton == MouseRight {
			if mx >= fm.x && mx < fm.x+fm.w && my >= fm.y+20 && my < fm.y+fm.h {
				// Determine if clicked on an item
				listY := fm.y + 55
				itemIdx := (my - listY) / 18 + fm.scrollOff
				if itemIdx >= 0 && itemIdx < len(fm.items) {
					fm.focusedIdx = itemIdx
					fm.selectedIdx = map[int]bool{itemIdx: true}
				}

				items := []MenuItem{
					{Label: "Open", Action: func() {
						if fm.focusedIdx >= 0 && fm.focusedIdx < len(fm.items) {
							it := fm.items[fm.focusedIdx]
							p := filepath.Join(fm.path, it.Name)
							if it.IsDir {
								fm.NavigateTo(p)
							} else {
								fm.OpenItem(p)
							}
						}
					}},
					{Label: "Cut (Ctrl+X)", Action: func() { fm.CopySelected(true) }},
					{Label: "Copy (Ctrl+C)", Action: func() { fm.CopySelected(false) }},
					{Label: "Paste (Ctrl+V)", Action: func() { fm.PasteClipboard() }},
					{Label: "Rename", Action: func() { fm.PromptRename() }},
					{Label: "Delete (Del)", Action: func() { fm.DeleteSelected() }},
					{Label: "New Folder", Action: func() { fm.PromptNewFolder() }},
					{Label: "New File", Action: func() { fm.PromptNewFile() }},
					{Label: "Refresh", Action: func() { fm.Refresh() }},
				}
				fm.wm.OpenContextMenu(mx, my, items)
				return true
			}
		}

		if e.MouseButton == MouseLeft {
			// Clicked Address Path Box (fm.y+22 to fm.y+42)
			if mx >= fm.x+5 && mx < fm.x+fm.w-5 && my >= fm.y+22 && my < fm.y+42 {
				// Up button check
				if mx >= fm.x+5 && mx <= fm.x+35 {
					fm.GoUpParent()
					return true
				}
			}

			// Item list click
			listY := fm.y + 55
			if my >= listY && my < fm.y+fm.h-20 {
				itemIdx := (my - listY) / 18 + fm.scrollOff
				if itemIdx >= 0 && itemIdx < len(fm.items) {
					fm.focusedIdx = itemIdx
					if e.Ctrl {
						fm.selectedIdx[itemIdx] = !fm.selectedIdx[itemIdx]
					} else {
						fm.selectedIdx = map[int]bool{itemIdx: true}
					}
				}
				return true
			}
		}
	}

	// Double click opens file / folder
	if e.Type == EventMouseDoubleClick && e.MouseButton == MouseLeft {
		listY := fm.y + 55
		itemIdx := (e.MouseY - listY) / 18 + fm.scrollOff
		if itemIdx >= 0 && itemIdx < len(fm.items) {
			item := fm.items[itemIdx]
			itemPath := filepath.Join(fm.path, item.Name)
			if item.IsDir {
				fm.NavigateTo(itemPath)
			} else {
				fm.OpenItem(itemPath)
			}
			return true
		}
	}

	return false
}

func (fm *FileManagerWindow) OpenItem(itemPath string) {
	ext := strings.ToLower(filepath.Ext(itemPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp":
		fm.wm.OpenImageViewerWindow(itemPath)
	default:
		// Default to Text Editor
		fm.wm.OpenTextEditorWindow(itemPath)
	}
}

func (fm *FileManagerWindow) CopySelected(isCut bool) {
	if fm.focusedIdx >= 0 && fm.focusedIdx < len(fm.items) {
		p := filepath.Join(fm.path, fm.items[fm.focusedIdx].Name)
		globalClipboard = Clipboard{
			SourcePath: p,
			IsCut:      isCut,
		}
	}
}

func (fm *FileManagerWindow) PasteClipboard() {
	if globalClipboard.SourcePath == "" {
		return
	}
	src := globalClipboard.SourcePath
	base := filepath.Base(src)
	dst := filepath.Join(fm.path, base)

	if src == dst {
		dst = filepath.Join(fm.path, "Copy_of_"+base)
	}

	CopyFileOrDir(src, dst)
	if globalClipboard.IsCut {
		os.RemoveAll(src)
		globalClipboard.SourcePath = ""
	}
	fm.Refresh()
}

func (fm *FileManagerWindow) DeleteSelected() {
	for idx := range fm.selectedIdx {
		if idx >= 0 && idx < len(fm.items) {
			p := filepath.Join(fm.path, fm.items[idx].Name)
			os.RemoveAll(p)
		}
	}
	fm.Refresh()
}

func (fm *FileManagerWindow) PromptRename() {
	if fm.focusedIdx >= 0 && fm.focusedIdx < len(fm.items) {
		oldName := fm.items[fm.focusedIdx].Name
		oldPath := filepath.Join(fm.path, oldName)
		newName := oldName + "_renamed"
		newPath := filepath.Join(fm.path, newName)
		os.Rename(oldPath, newPath)
		fm.Refresh()
	}
}

func (fm *FileManagerWindow) PromptNewFolder() {
	newFolderPath := filepath.Join(fm.path, "New_Folder")
	for i := 1; ; i++ {
		if _, err := os.Stat(newFolderPath); os.IsNotExist(err) {
			break
		}
		newFolderPath = filepath.Join(fm.path, fmt.Sprintf("New_Folder_%d", i))
	}
	os.Mkdir(newFolderPath, 0755)
	fm.Refresh()
}

func (fm *FileManagerWindow) PromptNewFile() {
	newFilePath := filepath.Join(fm.path, "New_File.txt")
	for i := 1; ; i++ {
		if _, err := os.Stat(newFilePath); os.IsNotExist(err) {
			break
		}
		newFilePath = filepath.Join(fm.path, fmt.Sprintf("New_File_%d.txt", i))
	}
	os.WriteFile(newFilePath, []byte(""), 0644)
	fm.Refresh()
}

func (fm *FileManagerWindow) Draw(fb Framebuffer, active bool) {
	// Drop Shadow
	fb.FillRect(fm.x+6, fm.y+6, fm.w, fm.h, ColorShadow)

	// Window Background & Frame
	fb.FillRect(fm.x, fm.y, fm.w, fm.h, ColorWinBg)
	fb.DrawRect(fm.x, fm.y, fm.w, fm.h, ColorWinBorder)

	// Title Bar
	titleBg := ColorTitleInact
	if active {
		titleBg = ColorTitleActive
	}
	fb.FillRect(fm.x+1, fm.y+1, fm.w-2, 20, titleBg)

	// Controls: Close [X], Collapse [-], Title Text
	fb.DrawText(fm.x+8, fm.y+3, "Folder: "+fm.path, ColorTitleText, titleBg)
	fb.FillRect(fm.x+fm.w-36, fm.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(fm.x+fm.w-36, fm.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(fm.x+fm.w-32, fm.y+2, "-", ColorText, ColorButtonBg)

	fb.FillRect(fm.x+fm.w-18, fm.y+3, 16, 15, ColorButtonBg)
	fb.DrawRect(fm.x+fm.w-18, fm.y+3, 16, 15, ColorWinBorder)
	fb.DrawText(fm.x+fm.w-14, fm.y+2, "X", ColorText, ColorButtonBg)

	if fm.collapsed {
		return
	}

	// Address Bar / Toolbar
	fb.FillRect(fm.x+2, fm.y+22, fm.w-4, 22, ColorBarBg)
	fb.DrawLine(fm.x+2, fm.y+44, fm.x+fm.w-2, fm.y+44, ColorWinBorder)

	// Up Button [ ^ ]
	fb.FillRect(fm.x+5, fm.y+24, 30, 18, ColorButtonBg)
	fb.DrawRect(fm.x+5, fm.y+24, 30, 18, ColorWinBorder)
	fb.DrawText(fm.x+12, fm.y+25, "^", ColorText, ColorButtonBg)

	// Path Box
	fb.FillRect(fm.x+40, fm.y+24, fm.w-48, 18, ColorWinBg)
	fb.DrawRect(fm.x+40, fm.y+24, fm.w-48, 18, ColorWinBorder)
	fb.DrawText(fm.x+44, fm.y+25, fm.path, ColorText, ColorWinBg)

	// File Table Header
	headerY := fm.y + 46
	fb.FillRect(fm.x+2, headerY, fm.w-4, 18, ColorBarBg)
	fb.DrawLine(fm.x+2, headerY+18, fm.x+fm.w-2, headerY+18, ColorWinBorder)

	fb.DrawText(fm.x+10, headerY+2, "Name", ColorText, ColorBarBg)
	fb.DrawText(fm.x+fm.w-180, headerY+2, "Size", ColorText, ColorBarBg)
	fb.DrawText(fm.x+fm.w-90, headerY+2, "Date", ColorText, ColorBarBg)

	// Items list view
	listY := headerY + 19
	maxVisible := (fm.h - 90) / 18

	for i := 0; i < maxVisible; i++ {
		idx := fm.scrollOff + i
		if idx >= len(fm.items) {
			break
		}

		item := fm.items[idx]
		itemY := listY + i*18

		bg := ColorWinBg
		fg := ColorText

		if fm.selectedIdx[idx] || idx == fm.focusedIdx {
			bg = ColorHighlight
			fg = ColorHlText
		}

		fb.FillRect(fm.x+3, itemY, fm.w-20, 18, bg)

		prefix := "[F] "
		if item.IsDir {
			prefix = "[D] "
		}

		nameStr := prefix + item.Name
		if len(nameStr) > 30 {
			nameStr = nameStr[:27] + "..."
		}

		fb.DrawText(fm.x+6, itemY+1, nameStr, fg, bg)

		sizeStr := "<DIR>"
		if !item.IsDir {
			sizeStr = FormatFileSize(item.Size)
		}
		fb.DrawText(fm.x+fm.w-180, itemY+1, sizeStr, fg, bg)

		dateStr := item.ModTime.Format("01/02/06")
		fb.DrawText(fm.x+fm.w-90, itemY+1, dateStr, fg, bg)
	}

	// Scrollbar on right edge
	sbX := fm.x + fm.w - 16
	sbY := listY
	sbH := fm.h - 90
	fb.FillRect(sbX, sbY, 14, sbH, ColorBarBg)
	fb.DrawRect(sbX, sbY, 14, sbH, ColorWinBorder)

	// Scroll thumb
	if len(fm.items) > maxVisible {
		thumbH := sbH * maxVisible / len(fm.items)
		if thumbH < 12 {
			thumbH = 12
		}
		thumbY := sbY + (sbH-thumbH)*fm.scrollOff/(len(fm.items)-maxVisible)
		fb.FillRect(sbX+1, thumbY, 12, thumbH, ColorButtonBg)
		fb.DrawRect(sbX+1, thumbY, 12, thumbH, ColorWinBorder)
	}

	// Status bar at bottom
	statusY := fm.y + fm.h - 20
	fb.FillRect(fm.x+2, statusY, fm.w-4, 18, ColorBarBg)
	fb.DrawLine(fm.x+2, statusY, fm.x+fm.w-2, statusY, ColorWinBorder)

	statusText := fmt.Sprintf("%d items", len(fm.items))
	if len(fm.selectedIdx) > 0 {
		statusText += fmt.Sprintf(" (%d selected)", len(fm.selectedIdx))
	}
	fb.DrawText(fm.x+6, statusY+2, statusText, ColorText, ColorBarBg)

	// Resize handle icon at bottom right corner
	fb.DrawLine(fm.x+fm.w-12, fm.y+fm.h-2, fm.x+fm.w-2, fm.y+fm.h-12, ColorWinBorder)
	fb.DrawLine(fm.x+fm.w-8, fm.y+fm.h-2, fm.x+fm.w-2, fm.y+fm.h-8, ColorWinBorder)
	fb.DrawLine(fm.x+fm.w-4, fm.y+fm.h-2, fm.x+fm.w-2, fm.y+fm.h-4, ColorWinBorder)
}
