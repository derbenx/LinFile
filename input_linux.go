//go:build linux

package main

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"
	"golang.org/x/sys/unix"
)

const (
	KDSETMODE   = 0x4B3A
	KD_TEXT     = 0x00
	KD_GRAPHICS = 0x01
)

const (
	ABS_X        = 0x00
	ABS_Y        = 0x01
	ABS_MT_POS_X = 0x35
	ABS_MT_POS_Y = 0x36
)

var oldState *term.State

func HideConsoleCursor() {
	os.Stdout.WriteString("\033[?25l") // Hide cursor escape sequence

	// Make TTY stdin raw to capture keys directly if needed
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, _ = term.MakeRaw(int(os.Stdin.Fd()))
	}

	for _, ttyPath := range []string{"/dev/tty0", "/dev/tty", "/dev/tty1", "/dev/console"} {
		f, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
		if err == nil {
			unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(KDSETMODE), uintptr(KD_GRAPHICS))
			f.Close()
			break
		}
	}
}

func RestoreConsoleCursor() {
	os.Stdout.WriteString("\033[?25h") // Show cursor

	if oldState != nil {
		term.Restore(int(os.Stdin.Fd()), oldState)
	}

	for _, ttyPath := range []string{"/dev/tty0", "/dev/tty", "/dev/tty1", "/dev/console"} {
		f, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
		if err == nil {
			unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(KDSETMODE), uintptr(KD_TEXT))
			f.Close()
			break
		}
	}
}

func (is *InputSystem) Start() {
	// 1. Scan /dev/input for evdev and mice
	filepath.Walk("/dev/input", func(path string, info fs.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			base := filepath.Base(path)
			if base == "mice" || base == "mouse0" {
				go is.readMousePs2Loop(path)
			} else if filepath.HasPrefix(base, "event") {
				go is.readEvdevLoop(path)
			}
		}
		return nil
	})

	// 2. Fallback TTY Stdin reader for keyboard
	go is.readStdinLoop()
}

func (is *InputSystem) Stop() {
	close(is.stopCh)
}

func (is *InputSystem) readStdinLoop() {
	buf := make([]byte, 16)
	for {
		select {
		case <-is.stopCh:
			return
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			time.Sleep(20 * time.Millisecond)
			continue
		}

		is.mu.Lock()
		for i := 0; i < n; i++ {
			b := buf[i]

			// Check for Ctrl+C (3) or Ctrl+Q (17) or Esc (27)
			if b == 3 || b == 17 { // Ctrl+C / Ctrl+Q -> trigger exit/quit
				is.sendEvent(Event{Type: EventKeyDown, Key: KEY_Q, Ctrl: true, Super: true})
			} else if b == 27 { // ESC or arrow sequences
				if i+2 < n && buf[i+1] == '[' {
					switch buf[i+2] {
					case 'A':
						is.sendEvent(Event{Type: EventKeyDown, Key: KEY_UP})
					case 'B':
						is.sendEvent(Event{Type: EventKeyDown, Key: KEY_DOWN})
					case 'C':
						is.sendEvent(Event{Type: EventKeyDown, Key: KEY_RIGHT})
					case 'D':
						is.sendEvent(Event{Type: EventKeyDown, Key: KEY_LEFT})
					}
					i += 2
				} else {
					is.sendEvent(Event{Type: EventKeyDown, Key: KEY_ESC})
				}
			} else if b == 13 || b == 10 { // Enter
				is.sendEvent(Event{Type: EventKeyDown, Key: KEY_ENTER})
			} else if b == 127 || b == 8 { // Backspace
				is.sendEvent(Event{Type: EventKeyDown, Key: KEY_BACKSPACE})
			} else if b == 9 { // Tab
				is.sendEvent(Event{Type: EventKeyDown, Key: KEY_TAB})
			} else if b >= 32 && b <= 126 {
				ch := rune(b)
				key := runeToKey(ch)
				is.sendEvent(Event{Type: EventKeyDown, Key: key, Char: ch})
			}
		}
		is.mu.Unlock()
	}
}

func (is *InputSystem) readMousePs2Loop(devicePath string) {
	file, err := os.Open(devicePath)
	if err != nil {
		return
	}
	defer file.Close()

	buf := make([]byte, 3)

	for {
		select {
		case <-is.stopCh:
			return
		default:
		}

		n, err := file.Read(buf)
		if err != nil || n < 3 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		b0 := buf[0]
		dx := int(int8(buf[1]))
		dy := int(int8(buf[2]))

		is.mu.Lock()
		is.mouseX += dx
		is.mouseY -= dy
		if is.mouseX < 0 { is.mouseX = 0 }
		if is.mouseX >= is.screenW { is.mouseX = is.screenW - 1 }
		if is.mouseY < 0 { is.mouseY = 0 }
		if is.mouseY >= is.screenH { is.mouseY = is.screenH - 1 }

		leftPressed := (b0 & 0x01) != 0
		rightPressed := (b0 & 0x02) != 0

		if leftPressed && !is.btnLeft {
			is.btnLeft = true
			nowMs := time.Now().UnixNano() / 1e6
			isDbl := (nowMs-is.lastClickMs < 350) && (abs(is.mouseX-is.lastClickX) < 5) && (abs(is.mouseY-is.lastClickY) < 5)
			is.lastClickMs = nowMs
			is.lastClickX = is.mouseX
			is.lastClickY = is.mouseY

			evtType := EventMouseDown
			if isDbl {
				evtType = EventMouseDoubleClick
			}

			is.sendEvent(Event{
				Type:        evtType,
				MouseX:      is.mouseX,
				MouseY:      is.mouseY,
				MouseButton: MouseLeft,
				Ctrl:        is.ctrl,
				Shift:       is.shift,
				Alt:         is.alt,
				Super:       is.super,
			})
		} else if !leftPressed && is.btnLeft {
			is.btnLeft = false
			is.sendEvent(Event{
				Type:        EventMouseUp,
				MouseX:      is.mouseX,
				MouseY:      is.mouseY,
				MouseButton: MouseLeft,
				Ctrl:        is.ctrl,
				Shift:       is.shift,
				Alt:         is.alt,
				Super:       is.super,
			})
		}

		if rightPressed && !is.btnRight {
			is.btnRight = true
			is.sendEvent(Event{
				Type:        EventMouseDown,
				MouseX:      is.mouseX,
				MouseY:      is.mouseY,
				MouseButton: MouseRight,
				Ctrl:        is.ctrl,
				Shift:       is.shift,
				Alt:         is.alt,
				Super:       is.super,
			})
		} else if !rightPressed && is.btnRight {
			is.btnRight = false
			is.sendEvent(Event{
				Type:        EventMouseUp,
				MouseX:      is.mouseX,
				MouseY:      is.mouseY,
				MouseButton: MouseRight,
				Ctrl:        is.ctrl,
				Shift:       is.shift,
				Alt:         is.alt,
				Super:       is.super,
			})
		}

		if dx != 0 || dy != 0 {
			is.sendEvent(Event{
				Type:   EventMouseMove,
				MouseX: is.mouseX,
				MouseY: is.mouseY,
				Ctrl:   is.ctrl,
				Shift:  is.shift,
				Alt:    is.alt,
				Super:  is.super,
			})
		}
		is.mu.Unlock()
	}
}

func (is *InputSystem) readEvdevLoop(devicePath string) {
	file, err := os.Open(devicePath)
	if err != nil {
		return
	}
	defer file.Close()

	buf := make([]byte, 24)

	for {
		select {
		case <-is.stopCh:
			return
		default:
		}

		n, err := file.Read(buf)
		if err != nil || n < 16 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var evType, evCode uint16
		var evValue int32

		if n >= 24 {
			evType = binary.LittleEndian.Uint16(buf[16:18])
			evCode = binary.LittleEndian.Uint16(buf[18:20])
			evValue = int32(binary.LittleEndian.Uint32(buf[20:24]))
		} else if n >= 16 {
			evType = binary.LittleEndian.Uint16(buf[8:10])
			evCode = binary.LittleEndian.Uint16(buf[10:12])
			evValue = int32(binary.LittleEndian.Uint32(buf[12:16]))
		}

		is.processEvent(evType, evCode, evValue)
	}
}

func (is *InputSystem) processEvent(evType, evCode uint16, evValue int32) {
	is.mu.Lock()
	defer is.mu.Unlock()

	switch evType {
	case EV_REL:
		switch evCode {
		case REL_X:
			is.mouseX += int(evValue)
			if is.mouseX < 0 { is.mouseX = 0 }
			if is.mouseX >= is.screenW { is.mouseX = is.screenW - 1 }
			is.sendEvent(Event{
				Type:   EventMouseMove,
				MouseX: is.mouseX,
				MouseY: is.mouseY,
				Ctrl:   is.ctrl,
				Shift:  is.shift,
				Alt:    is.alt,
				Super:  is.super,
			})
		case REL_Y:
			is.mouseY += int(evValue)
			if is.mouseY < 0 { is.mouseY = 0 }
			if is.mouseY >= is.screenH { is.mouseY = is.screenH - 1 }
			is.sendEvent(Event{
				Type:   EventMouseMove,
				MouseX: is.mouseX,
				MouseY: is.mouseY,
				Ctrl:   is.ctrl,
				Shift:  is.shift,
				Alt:    is.alt,
				Super:  is.super,
			})
		}

	case EV_ABS: // VirtualBox / Tablet / Touchpad Absolute Mouse Coordinates
		switch evCode {
		case ABS_X, ABS_MT_POS_X:
			if evValue > 0 {
				if evValue <= int32(is.screenW) {
					is.mouseX = int(evValue)
				} else {
					is.mouseX = int(evValue) * is.screenW / 32767
				}
			}
			is.sendEvent(Event{
				Type:   EventMouseMove,
				MouseX: is.mouseX,
				MouseY: is.mouseY,
				Ctrl:   is.ctrl,
				Shift:  is.shift,
				Alt:    is.alt,
				Super:  is.super,
			})
		case ABS_Y, ABS_MT_POS_Y:
			if evValue > 0 {
				if evValue <= int32(is.screenH) {
					is.mouseY = int(evValue)
				} else {
					is.mouseY = int(evValue) * is.screenH / 32767
				}
			}
			is.sendEvent(Event{
				Type:   EventMouseMove,
				MouseX: is.mouseX,
				MouseY: is.mouseY,
				Ctrl:   is.ctrl,
				Shift:  is.shift,
				Alt:    is.alt,
				Super:  is.super,
			})
		}

	case EV_KEY:
		code := evCode
		val := evValue

		if code == BTN_LEFT || code == BTN_TOUCH {
			if val == 1 { // Down
				is.btnLeft = true
				nowMs := time.Now().UnixNano() / 1e6
				isDbl := (nowMs-is.lastClickMs < 350) && (abs(is.mouseX-is.lastClickX) < 5) && (abs(is.mouseY-is.lastClickY) < 5)
				is.lastClickMs = nowMs
				is.lastClickX = is.mouseX
				is.lastClickY = is.mouseY

				evtType := EventMouseDown
				if isDbl {
					evtType = EventMouseDoubleClick
				}

				is.sendEvent(Event{
					Type:        evtType,
					MouseX:      is.mouseX,
					MouseY:      is.mouseY,
					MouseButton: MouseLeft,
					Ctrl:        is.ctrl,
					Shift:       is.shift,
					Alt:         is.alt,
					Super:       is.super,
				})
			} else if val == 0 { // Up
				is.btnLeft = false
				is.sendEvent(Event{
					Type:        EventMouseUp,
					MouseX:      is.mouseX,
					MouseY:      is.mouseY,
					MouseButton: MouseLeft,
					Ctrl:        is.ctrl,
					Shift:       is.shift,
					Alt:         is.alt,
					Super:       is.super,
				})
			}
			return
		}

		if code == BTN_RIGHT {
			if val == 1 {
				is.btnRight = true
				is.sendEvent(Event{
					Type:        EventMouseDown,
					MouseX:      is.mouseX,
					MouseY:      is.mouseY,
					MouseButton: MouseRight,
					Ctrl:        is.ctrl,
					Shift:       is.shift,
					Alt:         is.alt,
					Super:       is.super,
				})
			} else if val == 0 {
				is.btnRight = false
				is.sendEvent(Event{
					Type:        EventMouseUp,
					MouseX:      is.mouseX,
					MouseY:      is.mouseY,
					MouseButton: MouseRight,
					Ctrl:        is.ctrl,
					Shift:       is.shift,
					Alt:         is.alt,
					Super:       is.super,
				})
			}
			return
		}

		// Modifier updates
		switch code {
		case KEY_LEFTCTRL, KEY_RIGHTCTRL:
			is.ctrl = (val != 0)
		case KEY_LEFTSHIFT, KEY_RIGHTSHIFT:
			is.shift = (val != 0)
		case KEY_LEFTALT, KEY_RIGHTALT:
			is.alt = (val != 0)
		case KEY_LEFTMETA, KEY_RIGHTMETA:
			is.super = (val != 0)
		}

		if val == 1 || val == 2 { // Key Down / Repeat
			ch := keyToRune(code, is.shift)
			is.sendEvent(Event{
				Type:  EventKeyDown,
				Key:   code,
				Char:  ch,
				Ctrl:  is.ctrl,
				Shift: is.shift,
				Alt:   is.alt,
				Super: is.super,
			})
		} else if val == 0 { // Key Up
			is.sendEvent(Event{
				Type:  EventKeyUp,
				Key:   code,
				Ctrl:  is.ctrl,
				Shift: is.shift,
				Alt:   is.alt,
				Super: is.super,
			})
		}
	}
}

func runeToKey(ch rune) uint16 {
	switch ch {
	case 'a', 'A': return KEY_A
	case 'b', 'B': return KEY_B
	case 'c', 'C': return KEY_C
	case 'd', 'D': return KEY_D
	case 'e', 'E': return KEY_E
	case 'f', 'F': return KEY_F
	case 'g', 'G': return KEY_G
	case 'h', 'H': return KEY_H
	case 'i', 'I': return KEY_I
	case 'j', 'J': return KEY_J
	case 'k', 'K': return KEY_K
	case 'l', 'L': return KEY_L
	case 'm', 'M': return KEY_M
	case 'n', 'N': return KEY_N
	case 'o', 'O': return KEY_O
	case 'p', 'P': return KEY_P
	case 'q', 'Q': return KEY_Q
	case 'r', 'R': return KEY_R
	case 's', 'S': return KEY_S
	case 't', 'T': return KEY_T
	case 'u', 'U': return KEY_U
	case 'v', 'V': return KEY_V
	case 'w', 'W': return KEY_W
	case 'x', 'X': return KEY_X
	case 'y', 'Y': return KEY_Y
	case 'z', 'Z': return KEY_Z
	}
	return 0
}

func keyToRune(code uint16, shift bool) rune {
	switch code {
	case KEY_A: if shift { return 'A' }; return 'a'
	case KEY_B: if shift { return 'B' }; return 'b'
	case KEY_C: if shift { return 'C' }; return 'c'
	case KEY_D: if shift { return 'D' }; return 'd'
	case KEY_E: if shift { return 'E' }; return 'e'
	case KEY_F: if shift { return 'F' }; return 'f'
	case KEY_G: if shift { return 'G' }; return 'g'
	case KEY_H: if shift { return 'H' }; return 'h'
	case KEY_I: if shift { return 'I' }; return 'i'
	case KEY_J: if shift { return 'J' }; return 'j'
	case KEY_K: if shift { return 'K' }; return 'k'
	case KEY_L: if shift { return 'L' }; return 'l'
	case KEY_M: if shift { return 'M' }; return 'm'
	case KEY_N: if shift { return 'N' }; return 'n'
	case KEY_O: if shift { return 'O' }; return 'o'
	case KEY_P: if shift { return 'P' }; return 'p'
	case KEY_Q: if shift { return 'Q' }; return 'q'
	case KEY_R: if shift { return 'R' }; return 'r'
	case KEY_S: if shift { return 'S' }; return 's'
	case KEY_T: if shift { return 'T' }; return 't'
	case KEY_U: if shift { return 'U' }; return 'u'
	case KEY_V: if shift { return 'V' }; return 'v'
	case KEY_W: if shift { return 'W' }; return 'w'
	case KEY_X: if shift { return 'X' }; return 'x'
	case KEY_Y: if shift { return 'Y' }; return 'y'
	case KEY_Z: if shift { return 'Z' }; return 'z'

	case KEY_1: if shift { return '!' }; return '1'
	case KEY_2: if shift { return '@' }; return '2'
	case KEY_3: if shift { return '#' }; return '3'
	case KEY_4: if shift { return '$' }; return '4'
	case KEY_5: if shift { return '%' }; return '5'
	case KEY_6: if shift { return '^' }; return '6'
	case KEY_7: if shift { return '&' }; return '7'
	case KEY_8: if shift { return '*' }; return '8'
	case KEY_9: if shift { return '(' }; return '9'
	case KEY_0: if shift { return ')' }; return '0'

	case KEY_SPACE: return ' '
	case KEY_MINUS: if shift { return '_' }; return '-'
	case KEY_EQUAL: if shift { return '+' }; return '='
	case KEY_LEFTBRACE: if shift { return '{' }; return '['
	case KEY_RIGHTBRACE: if shift { return '}' }; return ']'
	case KEY_SEMICOLON: if shift { return ':' }; return ';'
	case KEY_APOSTROPHE: if shift { return '"' }; return '\''
	case KEY_GRAVE: if shift { return '~' }; return '`'
	case KEY_BACKSLASH: if shift { return '|' }; return '\\'
	case KEY_COMMA: if shift { return '<' }; return ','
	case KEY_DOT: if shift { return '>' }; return '.'
	case KEY_SLASH: if shift { return '?' }; return '/'
	}
	return 0
}
