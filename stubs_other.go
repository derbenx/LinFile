//go:build !linux

package main

import "fmt"

func OpenFramebuffer(device string) (Framebuffer, error) {
	return nil, fmt.Errorf("framebuffer is only supported on Linux")
}

func HideConsoleCursor()    {}
func RestoreConsoleCursor() {}

func (is *InputSystem) Start() {}
func (is *InputSystem) Stop()  {}
