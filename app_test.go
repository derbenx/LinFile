package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryFramebuffer(t *testing.T) {
	mfb := NewMemoryFramebuffer(800, 600)
	if mfb.Width() != 800 || mfb.Height() != 600 {
		t.Fatalf("expected 800x600, got %dx%d", mfb.Width(), mfb.Height())
	}

	testCol := color.RGBA{R: 255, G: 0, B: 128, A: 255}
	mfb.SetPixel(100, 100, testCol)
	got := mfb.GetPixel(100, 100)

	if got != testCol {
		t.Fatalf("expected pixel %v, got %v", testCol, got)
	}

	mfb.FillRect(0, 0, 50, 50, testCol)
	if mfb.GetPixel(25, 25) != testCol {
		t.Fatalf("FillRect failed")
	}
}

func TestDirectoryOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "linfile_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "sub_folder")
	os.Mkdir(subDir, 0755)

	testFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(testFile, []byte("Hello LinFile!"), 0644)

	items, err := ReadDirectory(tmpDir)
	if err != nil {
		t.Fatalf("ReadDirectory failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Directories sorted first
	if !items[0].IsDir || items[0].Name != "sub_folder" {
		t.Fatalf("expected sub_folder first, got %v", items[0].Name)
	}

	if items[1].IsDir || items[1].Name != "hello.txt" {
		t.Fatalf("expected hello.txt second, got %v", items[1].Name)
	}
}

func TestWindowManagerSnapping(t *testing.T) {
	mfb := NewMemoryFramebuffer(1024, 768)
	wm := NewWindowManager(mfb)

	fmWin := NewFileManagerWindow(1, ".", wm)
	wm.AddWindow(fmWin)

	wm.SnapWindow(fmWin, "left")
	x, y, w, h := fmWin.Bounds()
	if x != 0 || y != 22 || w != 512 || h != 746 {
		t.Fatalf("snap left incorrect: got (%d, %d, %d, %d)", x, y, w, h)
	}

	wm.SnapWindow(fmWin, "top-right")
	x, y, w, h = fmWin.Bounds()
	if x != 512 || y != 22 || w != 512 || h != 373 {
		t.Fatalf("snap top-right incorrect: got (%d, %d, %d, %d)", x, y, w, h)
	}
}

func TestTextEditorSave(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_edit_*.txt")
	if err != nil {
		t.Fatalf("temp file error: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	mfb := NewMemoryFramebuffer(800, 600)
	wm := NewWindowManager(mfb)

	te := NewTextEditorWindow(1, tmpFile.Name(), wm)
	te.lines = []string{"Line 1", "Line 2"}
	te.SaveFile()

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed reading file: %v", err)
	}

	if string(content) != "Line 1\nLine 2" {
		t.Fatalf("expected 'Line 1\\nLine 2', got '%s'", string(content))
	}
}
