package main

import (
	"image"
	"image/color"
	"sync"
)

// Framebuffer Interface
type Framebuffer interface {
	Width() int
	Height() int
	SetPixel(x, y int, c color.RGBA)
	GetPixel(x, y int) color.RGBA
	DrawRect(x, y, w, h int, c color.RGBA)
	FillRect(x, y, w, h int, c color.RGBA)
	DrawLine(x1, y1, x2, y2 int, c color.RGBA)
	DrawDitherPattern(x, y, w, h int, c1, c2 color.RGBA)
	DrawText(x, y int, text string, fg, bg color.RGBA)
	DrawImage(x, y int, img image.Image)
	DrawImageScaled(x, y, w, h int, img image.Image)
	Flush()
	Close() error
}

// MemoryFramebuffer implements Framebuffer in memory for testing or offscreen rendering
type MemoryFramebuffer struct {
	width      int
	height     int
	backBuffer []uint32
}

func NewMemoryFramebuffer(width, height int) *MemoryFramebuffer {
	return &MemoryFramebuffer{
		width:      width,
		height:     height,
		backBuffer: make([]uint32, width*height),
	}
}

func (mfb *MemoryFramebuffer) Width() int  { return mfb.width }
func (mfb *MemoryFramebuffer) Height() int { return mfb.height }

func (mfb *MemoryFramebuffer) SetPixel(x, y int, c color.RGBA) {
	if x < 0 || x >= mfb.width || y < 0 || y >= mfb.height {
		return
	}
	mfb.backBuffer[y*mfb.width+x] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) | uint32(c.A)<<24
}

func (mfb *MemoryFramebuffer) GetPixel(x, y int) color.RGBA {
	if x < 0 || x >= mfb.width || y < 0 || y >= mfb.height {
		return color.RGBA{0, 0, 0, 0}
	}
	p := mfb.backBuffer[y*mfb.width+x]
	return color.RGBA{
		R: byte(p >> 16),
		G: byte(p >> 8),
		B: byte(p),
		A: byte(p >> 24),
	}
}

func (mfb *MemoryFramebuffer) DrawRect(x, y, w, h int, c color.RGBA) {
	for px := x; px < x+w; px++ {
		mfb.SetPixel(px, y, c)
		mfb.SetPixel(px, y+h-1, c)
	}
	for py := y; py < y+h; py++ {
		mfb.SetPixel(x, py, c)
		mfb.SetPixel(x+w-1, py, c)
	}
}

func (mfb *MemoryFramebuffer) FillRect(x, y, w, h int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		if py < 0 || py >= mfb.height {
			continue
		}
		for px := x; px < x+w; px++ {
			if px < 0 || px >= mfb.width {
				continue
			}
			mfb.backBuffer[py*mfb.width+px] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) | uint32(c.A)<<24
		}
	}
}

func (mfb *MemoryFramebuffer) DrawLine(x1, y1, x2, y2 int, c color.RGBA) {
	dx := x2 - x1
	if dx < 0 {
		dx = -dx
	}
	dy := y2 - y1
	if dy < 0 {
		dy = -dy
	}
	sx := -1
	if x1 < x2 {
		sx = 1
	}
	sy := -1
	if y1 < y2 {
		sy = 1
	}
	err := dx - dy

	for {
		mfb.SetPixel(x1, y1, c)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func (mfb *MemoryFramebuffer) DrawDitherPattern(x, y, w, h int, c1, c2 color.RGBA) {
	for py := y; py < y+h; py++ {
		if py < 0 || py >= mfb.height {
			continue
		}
		for px := x; px < x+w; px++ {
			if px < 0 || px >= mfb.width {
				continue
			}
			if (px+py)%2 == 0 {
				mfb.backBuffer[py*mfb.width+px] = uint32(c1.R)<<16 | uint32(c1.G)<<8 | uint32(c1.B) | uint32(c1.A)<<24
			} else {
				mfb.backBuffer[py*mfb.width+px] = uint32(c2.R)<<16 | uint32(c2.G)<<8 | uint32(c2.B) | uint32(c2.A)<<24
			}
		}
	}
}

func (mfb *MemoryFramebuffer) DrawText(x, y int, text string, fg, bg color.RGBA) {
	curX := x
	for _, ch := range text {
		if curX+8 > mfb.width {
			break
		}
		DrawChar8x16(mfb, curX, y, ch, fg, bg)
		curX += 8
	}
}

func (mfb *MemoryFramebuffer) DrawImage(x, y int, img image.Image) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	for py := 0; py < h; py++ {
		if y+py >= mfb.height {
			break
		}
		for px := 0; px < w; px++ {
			if x+px >= mfb.width {
				break
			}
			r, g, b, a := img.At(bounds.Min.X+px, bounds.Min.Y+py).RGBA()
			if a > 0 {
				mfb.SetPixel(x+px, y+py, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
			}
		}
	}
}

func (mfb *MemoryFramebuffer) DrawImageScaled(x, y, w, h int, img image.Image) {
	if w <= 0 || h <= 0 {
		return
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return
	}

	for py := 0; py < h; py++ {
		destY := y + py
		if destY < 0 || destY >= mfb.height {
			continue
		}
		srcY := bounds.Min.Y + (py * srcH / h)
		for px := 0; px < w; px++ {
			destX := x + px
			if destX < 0 || destX >= mfb.width {
				continue
			}
			srcX := bounds.Min.X + (px * srcW / w)
			r, g, b, a := img.At(srcX, srcY).RGBA()
			if a > 0 {
				mfb.SetPixel(destX, destY, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
			}
		}
	}
}

func (mfb *MemoryFramebuffer) Flush() {}
func (mfb *MemoryFramebuffer) Close() error { return nil }

// Event System Definitions
type EventType int

const (
	EventMouseMove EventType = iota
	EventMouseDown
	EventMouseUp
	EventMouseDoubleClick
	EventKeyDown
	EventKeyUp
	EventChar
)

type MouseButton int

const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseRight
	MouseMiddle
)

type Event struct {
	Type        EventType
	MouseX      int
	MouseY      int
	MouseButton MouseButton
	Key         uint16
	Char        rune
	Ctrl        bool
	Shift       bool
	Alt         bool
	Super       bool
}

// Common Keycodes
const (
	KEY_ESC        = 1
	KEY_1          = 2
	KEY_2          = 3
	KEY_3          = 4
	KEY_4          = 5
	KEY_5          = 6
	KEY_6          = 7
	KEY_7          = 8
	KEY_8          = 9
	KEY_9          = 10
	KEY_0          = 11
	KEY_MINUS      = 12
	KEY_EQUAL      = 13
	KEY_BACKSPACE  = 14
	KEY_TAB        = 15
	KEY_Q          = 16
	KEY_W          = 17
	KEY_E          = 18
	KEY_R          = 19
	KEY_T          = 20
	KEY_Y          = 21
	KEY_U          = 22
	KEY_I          = 23
	KEY_O          = 24
	KEY_P          = 25
	KEY_LEFTBRACE  = 26
	KEY_RIGHTBRACE = 27
	KEY_ENTER      = 28
	KEY_LEFTCTRL   = 29
	KEY_A          = 30
	KEY_S          = 31
	KEY_D          = 32
	KEY_F          = 33
	KEY_G          = 34
	KEY_H          = 35
	KEY_J          = 36
	KEY_K          = 37
	KEY_L          = 38
	KEY_SEMICOLON  = 39
	KEY_APOSTROPHE = 40
	KEY_GRAVE      = 41
	KEY_LEFTSHIFT  = 42
	KEY_BACKSLASH  = 43
	KEY_Z          = 44
	KEY_X          = 45
	KEY_C          = 46
	KEY_V          = 47
	KEY_B          = 48
	KEY_N          = 49
	KEY_M          = 50
	KEY_COMMA      = 51
	KEY_DOT        = 52
	KEY_SLASH      = 53
	KEY_RIGHTSHIFT = 54
	KEY_KPASTERISK = 55
	KEY_LEFTALT    = 56
	KEY_SPACE      = 57
	KEY_CAPSLOCK   = 58
	KEY_F1         = 59
	KEY_F2         = 60
	KEY_F3         = 61
	KEY_F4         = 62
	KEY_F5         = 63
	KEY_F6         = 64
	KEY_F7         = 65
	KEY_F8         = 66
	KEY_F9         = 67
	KEY_F10        = 68

	KEY_RIGHTCTRL = 97
	KEY_KPSLASH   = 98
	KEY_RIGHTALT  = 100
	KEY_HOME      = 102
	KEY_UP        = 103
	KEY_PAGEUP    = 104
	KEY_LEFT      = 105
	KEY_RIGHT     = 106
	KEY_END       = 107
	KEY_DOWN      = 108
	KEY_PAGEDOWN  = 109
	KEY_DELETE    = 111
	KEY_LEFTMETA  = 125
	KEY_RIGHTMETA = 126
)

type InputSystem struct {
	events      chan Event
	mouseX      int
	mouseY      int
	screenW     int
	screenH     int
	btnLeft     bool
	btnRight    bool
	ctrl        bool
	shift       bool
	alt         bool
	super       bool
	lastClickMs int64
	lastClickX  int
	lastClickY  int
	mu          sync.Mutex
	stopCh      chan struct{}
}

func NewInputSystem(screenW, screenH int) *InputSystem {
	is := &InputSystem{
		events:  make(chan Event, 256),
		mouseX:  screenW / 2,
		mouseY:  screenH / 2,
		screenW: screenW,
		screenH: screenH,
		stopCh:  make(chan struct{}),
	}
	return is
}

func (is *InputSystem) Events() <-chan Event {
	return is.events
}

func (is *InputSystem) MousePos() (int, int) {
	is.mu.Lock()
	defer is.mu.Unlock()
	return is.mouseX, is.mouseY
}

func (is *InputSystem) sendEvent(e Event) {
	select {
	case is.events <- e:
	default:
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
