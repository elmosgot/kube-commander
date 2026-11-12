package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard writes text to the system clipboard.
// Tries clip.exe (WSL/Windows), then wl-copy, then xclip, then pbcopy.
func copyToClipboard(text string) error {
	candidates := clipboardCommands()
	if len(candidates) == 0 {
		return fmt.Errorf("no clipboard tool (clip.exe, wl-copy, or xclip)")
	}
	var lastErr error
	for _, c := range candidates {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("%s: %w (%s)", c[0], err, strings.TrimSpace(stderr.String()))
			continue
		}
		return nil
	}
	return lastErr
}

func clipboardCommands() [][]string {
	var out [][]string
	if runtime.GOOS == "windows" {
		out = append(out, []string{"clip"})
	}
	if _, err := exec.LookPath("clip.exe"); err == nil {
		out = append(out, []string{"clip.exe"})
	}
	if _, err := exec.LookPath("wl-copy"); err == nil {
		out = append(out, []string{"wl-copy"})
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		out = append(out, []string{"xclip", "-selection", "clipboard"})
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("pbcopy"); err == nil {
			out = append(out, []string{"pbcopy"})
		}
	}
	return out
}
