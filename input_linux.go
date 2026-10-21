//go:build linux

package main

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func (is *InputSystem) Start() {
	filepath.Walk("/dev/input", func(path string, info fs.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			go is.readLoop(path)
		}
		return nil
	})
}

func (is *InputSystem) Stop() {
	close(is.stopCh)
}

func (is *InputSystem) readLoop(devicePath string) {
	file, err := os.Open(devicePath)
	if err != nil {
		return
	}
	defer file.Close()

	// 24 bytes buffer for Linux input_event struct (timeval + uint16 type + uint16 code + int32 value)
	buf := make([]byte, 24)

	for {
		select {
		case <-is.stopCh:
			return
		default:
		}

		n, err := file.Read(buf)
		if err != nil || n < 16 {
			// Exit loop cleanly if read fails or EOF is reached
			return
		}

		var evType, evCode uint16
		var evValue int32

		if n >= 24 {
			evType = binary.LittleEndian.Uint16(buf[16:18])
			evCode = binary.LittleEndian.Uint16(buf[18:20])
			evValue = int32(binary.LittleEndian.Uint32(buf[20:24]))
		} else if n >= 16 { // 32-bit architecture timeval (8 bytes)
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
			if is.mouseX < 0 {
				is.mouseX = 0
			}
			if is.mouseX >= is.screenW {
				is.mouseX = is.screenW - 1
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
		case REL_Y:
			is.mouseY += int(evValue)
			if is.mouseY < 0 {
				is.mouseY = 0
			}
			if is.mouseY >= is.screenH {
				is.mouseY = is.screenH - 1
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

func keyToRune(code uint16, shift bool) rune {
	switch code {
	case KEY_A:
		if shift { return 'A' }; return 'a'
	case KEY_B:
		if shift { return 'B' }; return 'b'
	case KEY_C:
		if shift { return 'C' }; return 'c'
	case KEY_D:
		if shift { return 'D' }; return 'd'
	case KEY_E:
		if shift { return 'E' }; return 'e'
	case KEY_F:
		if shift { return 'F' }; return 'f'
	case KEY_G:
		if shift { return 'G' }; return 'g'
	case KEY_H:
		if shift { return 'H' }; return 'h'
	case KEY_I:
		if shift { return 'I' }; return 'i'
	case KEY_J:
		if shift { return 'J' }; return 'j'
	case KEY_K:
		if shift { return 'K' }; return 'k'
	case KEY_L:
		if shift { return 'L' }; return 'l'
	case KEY_M:
		if shift { return 'M' }; return 'm'
	case KEY_N:
		if shift { return 'N' }; return 'n'
	case KEY_O:
		if shift { return 'O' }; return 'o'
	case KEY_P:
		if shift { return 'P' }; return 'p'
	case KEY_Q:
		if shift { return 'Q' }; return 'q'
	case KEY_R:
		if shift { return 'R' }; return 'r'
	case KEY_S:
		if shift { return 'S' }; return 's'
	case KEY_T:
		if shift { return 'T' }; return 't'
	case KEY_U:
		if shift { return 'U' }; return 'u'
	case KEY_V:
		if shift { return 'V' }; return 'v'
	case KEY_W:
		if shift { return 'W' }; return 'w'
	case KEY_X:
		if shift { return 'X' }; return 'x'
	case KEY_Y:
		if shift { return 'Y' }; return 'y'
	case KEY_Z:
		if shift { return 'Z' }; return 'z'

	case KEY_1:
		if shift { return '!' }; return '1'
	case KEY_2:
		if shift { return '@' }; return '2'
	case KEY_3:
		if shift { return '#' }; return '3'
	case KEY_4:
		if shift { return '$' }; return '4'
	case KEY_5:
		if shift { return '%' }; return '5'
	case KEY_6:
		if shift { return '^' }; return '6'
	case KEY_7:
		if shift { return '&' }; return '7'
	case KEY_8:
		if shift { return '*' }; return '8'
	case KEY_9:
		if shift { return '(' }; return '9'
	case KEY_0:
		if shift { return ')' }; return '0'

	case KEY_SPACE:
		return ' '
	case KEY_MINUS:
		if shift { return '_' }; return '-'
	case KEY_EQUAL:
		if shift { return '+' }; return '='
	case KEY_LEFTBRACE:
		if shift { return '{' }; return '['
	case KEY_RIGHTBRACE:
		if shift { return '}' }; return ']'
	case KEY_SEMICOLON:
		if shift { return ':' }; return ';'
	case KEY_APOSTROPHE:
		if shift { return '"' }; return '\''
	case KEY_GRAVE:
		if shift { return '~' }; return '`'
	case KEY_BACKSLASH:
		if shift { return '|' }; return '\\'
	case KEY_COMMA:
		if shift { return '<' }; return ','
	case KEY_DOT:
		if shift { return '>' }; return '.'
	case KEY_SLASH:
		if shift { return '?' }; return '/'
	}
	return 0
}
