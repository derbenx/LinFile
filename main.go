package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fbDevice := flag.String("fb", "/dev/fb0", "Framebuffer device path")
	flag.Parse()

	fmt.Println("Starting LinFile - GEOS-like GUI File Manager for Linux Framebuffer...")

	// Open Linux Framebuffer
	fb, err := OpenFramebuffer(*fbDevice)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening framebuffer %s: %v\n", *fbDevice, err)
		fmt.Fprintf(os.Stderr, "Note: LinFile requires framebuffer access (/dev/fb0) and evdev input access.\n")
		fmt.Fprintf(os.Stderr, "Please run with root/sudo privileges on a Linux TTY console.\n")
		os.Exit(1)
	}
	defer fb.Close()

	fmt.Printf("Framebuffer opened successfully: %dx%d\n", fb.Width(), fb.Height())

	// Hide console text cursor and switch TTY graphics mode if supported
	HideConsoleCursor()
	defer RestoreConsoleCursor()

	// Initialize Input System
	inputSystem := NewInputSystem(fb.Width(), fb.Height())
	inputSystem.Start()
	defer inputSystem.Stop()

	// Initialize Window Manager
	wm := NewWindowManager(fb)

	// Initial default window
	currDir, _ := os.Getwd()
	wm.OpenFileManagerWindow(currDir)

	// Catch OS interrupts
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Main event and render loop (60 FPS refresh rate)
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	defer func() {
		if r := recover(); r != nil {
			if r == "QUIT" {
				fmt.Println("LinFile exited normally.")
			} else {
				fmt.Printf("LinFile recovered from panic: %v\n", r)
			}
		}
	}()

	for {
		select {
		case <-sigCh:
			fmt.Println("Terminating LinFile...")
			return
		case event := <-inputSystem.Events():
			wm.HandleEvent(event)
		case <-ticker.C:
			// Render full GUI state
			wm.DrawAll()

			// Render Retro Mouse Cursor sprite (arrow pixel pointer)
			mx, my := inputSystem.MousePos()
			drawMouseCursor(fb, mx, my)

			// Flush backbuffer to framebuffer memory
			fb.Flush()
		}
	}
}

func drawMouseCursor(fb Framebuffer, x, y int) {
	// Classic GEOS white arrow with black border pixel cursor
	cursor := []string{
		"X         ",
		"XX        ",
		"X.X       ",
		"X..X      ",
		"X...X     ",
		"X....X    ",
		"X.....X   ",
		"X......X  ",
		"X...XXXX  ",
		"X..X      ",
		"X.X       ",
		"XX        ",
		"X         ",
	}

	for row, line := range cursor {
		for col, char := range line {
			px, py := x+col, y+row
			if char == 'X' {
				fb.SetPixel(px, py, ColorWinBorder)
			} else if char == '.' {
				fb.SetPixel(px, py, ColorWinBg)
			}
		}
	}
}
