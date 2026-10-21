package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Framebuffer constants and Linux ioctl definitions
const (
	FBIOGET_VSCREENINFO = 0x4600
	FBIOGET_FSCREENINFO = 0x4602
)

type FbBitfield struct {
	Offset    uint32
	Length    uint32
	MsbRight  uint32
}

type FbVarScreeninfo struct {
	Xres           uint32
	Yres           uint32
	XresVirtual    uint32
	YresVirtual    uint32
	Xoffset        uint32
	Yoffset        uint32
	BitsPerPixel   uint32
	Grayscale      uint32
	Red            FbBitfield
	Green          FbBitfield
	Blue           FbBitfield
	Transp         FbBitfield
	Nonstd         uint32
	Activate       uint32
	Height         uint32
	Width          uint32
	AccelFlags     uint32
	Pixclock       uint32
	LeftMargin     uint32
	RightMargin    uint32
	UpperMargin    uint32
	LowerMargin    uint32
	HsyncLen       uint32
	VsyncLen       uint32
	Sync           uint32
	Vmode          uint32
	Rotate         uint32
	Colorspace     uint32
	Reserved       [4]uint32
}

type FbFixScreeninfo struct {
	Id          [16]byte
	SmemStart   uintptr
	SmemLen     uint32
	Type        uint32
	TypeAux     uint32
	Visual      uint32
	Xpanstep    uint16
	Ypanstep    uint16
	Ywrapstep   uint16
	LineLength  uint32
	MmioStart   uintptr
	MmioLen     uint32
	Accel       uint32
	Capabilities uint16
	Reserved    [2]uint16
}

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

type LinuxFB struct {
	file       *os.File
	fbMem      []byte
	backBuffer []uint32
	varInfo    FbVarScreeninfo
	fixInfo    FbFixScreeninfo
	width      int
	height     int
	stride     int
	bpp        int
	mu         sync.Mutex
}

func OpenFramebuffer(device string) (*LinuxFB, error) {
	if device == "" {
		device = os.Getenv("FRAMEBUFFER")
		if device == "" {
			device = "/dev/fb0"
		}
	}

	file, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open framebuffer device %s: %w", device, err)
	}

	var varInfo FbVarScreeninfo
	_, _, sysErr := unix.Syscall(unix.SYS_IOCTL, file.Fd(), uintptr(FBIOGET_VSCREENINFO), uintptr(unsafe.Pointer(&varInfo)))
	if sysErr != 0 {
		file.Close()
		return nil, fmt.Errorf("ioctl FBIOGET_VSCREENINFO failed: %v", sysErr)
	}

	var fixInfo FbFixScreeninfo
	_, _, sysErr = unix.Syscall(unix.SYS_IOCTL, file.Fd(), uintptr(FBIOGET_FSCREENINFO), uintptr(unsafe.Pointer(&fixInfo)))
	if sysErr != 0 {
		file.Close()
		return nil, fmt.Errorf("ioctl FBIOGET_FSCREENINFO failed: %v", sysErr)
	}

	fbMem, err := unix.Mmap(int(file.Fd()), 0, int(fixInfo.SmemLen), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap framebuffer memory failed: %w", err)
	}

	fb := &LinuxFB{
		file:       file,
		fbMem:      fbMem,
		varInfo:    varInfo,
		fixInfo:    fixInfo,
		width:      int(varInfo.Xres),
		height:     int(varInfo.Yres),
		stride:     int(fixInfo.LineLength),
		bpp:        int(varInfo.BitsPerPixel),
		backBuffer: make([]uint32, int(varInfo.Xres)*int(varInfo.Yres)),
	}

	return fb, nil
}

func (fb *LinuxFB) Width() int  { return fb.width }
func (fb *LinuxFB) Height() int { return fb.height }

func (fb *LinuxFB) SetPixel(x, y int, c color.RGBA) {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return
	}
	fb.backBuffer[y*fb.width+x] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) | uint32(c.A)<<24
}

func (fb *LinuxFB) GetPixel(x, y int) color.RGBA {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return color.RGBA{0, 0, 0, 0}
	}
	p := fb.backBuffer[y*fb.width+x]
	return color.RGBA{
		R: byte(p >> 16),
		G: byte(p >> 8),
		B: byte(p),
		A: byte(p >> 24),
	}
}

func (fb *LinuxFB) DrawRect(x, y, w, h int, c color.RGBA) {
	for px := x; px < x+w; px++ {
		fb.SetPixel(px, y, c)
		fb.SetPixel(px, y+h-1, c)
	}
	for py := y; py < y+h; py++ {
		fb.SetPixel(x, py, c)
		fb.SetPixel(x+w-1, py, c)
	}
}

func (fb *LinuxFB) FillRect(x, y, w, h int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		if py < 0 || py >= fb.height {
			continue
		}
		for px := x; px < x+w; px++ {
			if px < 0 || px >= fb.width {
				continue
			}
			fb.backBuffer[py*fb.width+px] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) | uint32(c.A)<<24
		}
	}
}

func (fb *LinuxFB) DrawLine(x1, y1, x2, y2 int, c color.RGBA) {
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
		fb.SetPixel(x1, y1, c)
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

func (fb *LinuxFB) DrawDitherPattern(x, y, w, h int, c1, c2 color.RGBA) {
	for py := y; py < y+h; py++ {
		if py < 0 || py >= fb.height {
			continue
		}
		for px := x; px < x+w; px++ {
			if px < 0 || px >= fb.width {
				continue
			}
			if (px+py)%2 == 0 {
				fb.backBuffer[py*fb.width+px] = uint32(c1.R)<<16 | uint32(c1.G)<<8 | uint32(c1.B) | uint32(c1.A)<<24
			} else {
				fb.backBuffer[py*fb.width+px] = uint32(c2.R)<<16 | uint32(c2.G)<<8 | uint32(c2.B) | uint32(c2.A)<<24
			}
		}
	}
}

func (fb *LinuxFB) DrawText(x, y int, text string, fg, bg color.RGBA) {
	curX := x
	for _, ch := range text {
		if curX+8 > fb.width {
			break
		}
		DrawChar8x16(fb, curX, y, ch, fg, bg)
		curX += 8
	}
}

func (fb *LinuxFB) DrawImage(x, y int, img image.Image) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	for py := 0; py < h; py++ {
		if y+py >= fb.height {
			break
		}
		for px := 0; px < w; px++ {
			if x+px >= fb.width {
				break
			}
			r, g, b, a := img.At(bounds.Min.X+px, bounds.Min.Y+py).RGBA()
			if a > 0 {
				fb.SetPixel(x+px, y+py, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
			}
		}
	}
}

func (fb *LinuxFB) DrawImageScaled(x, y, w, h int, img image.Image) {
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
		if destY < 0 || destY >= fb.height {
			continue
		}
		srcY := bounds.Min.Y + (py * srcH / h)
		for px := 0; px < w; px++ {
			destX := x + px
			if destX < 0 || destX >= fb.width {
				continue
			}
			srcX := bounds.Min.X + (px * srcW / w)
			r, g, b, a := img.At(srcX, srcY).RGBA()
			if a > 0 {
				fb.SetPixel(destX, destY, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
			}
		}
	}
}

func (fb *LinuxFB) Flush() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	bpp := fb.bpp
	bytesPerPixel := bpp / 8
	rOff, gOff, bOff := fb.varInfo.Red.Offset, fb.varInfo.Green.Offset, fb.varInfo.Blue.Offset

	for y := 0; y < fb.height; y++ {
		rowStartFb := y * fb.stride
		rowStartBuf := y * fb.width

		for x := 0; x < fb.width; x++ {
			p := fb.backBuffer[rowStartBuf+x]
			r := uint8(p >> 16)
			g := uint8(p >> 8)
			b := uint8(p)

			offset := rowStartFb + x*bytesPerPixel
			if offset+bytesPerPixel > len(fb.fbMem) {
				continue
			}

			switch bytesPerPixel {
			case 4:
				val := (uint32(r) << rOff) | (uint32(g) << gOff) | (uint32(b) << bOff)
				fb.fbMem[offset] = byte(val)
				fb.fbMem[offset+1] = byte(val >> 8)
				fb.fbMem[offset+2] = byte(val >> 16)
				fb.fbMem[offset+3] = byte(val >> 24)
			case 3:
				fb.fbMem[offset] = b
				fb.fbMem[offset+1] = g
				fb.fbMem[offset+2] = r
			case 2: // 16-bit RGB 565
				val := uint16((uint16(r>>3) << 11) | (uint16(g>>2) << 5) | uint16(b>>3))
				fb.fbMem[offset] = byte(val)
				fb.fbMem[offset+1] = byte(val >> 8)
			}
		}
	}
}

func (fb *LinuxFB) Close() error {
	if fb.fbMem != nil {
		unix.Munmap(fb.fbMem)
	}
	if fb.file != nil {
		return fb.file.Close()
	}
	return nil
}

// MemoryFramebuffer implements Framebuffer in memory for virtualized testing or offscreen rendering
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
