package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/muesli/termenv"
)

func detectDarkMode() bool {
	out := termenv.NewOutput(os.Stderr)
	return out.HasDarkBackground()
}

type checkThemeMsg struct {
	dark bool
	err  error
}

func queryTerminalBackground() (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return true, err
	}
	defer tty.Close()

	if err := tty.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return true, err
	}

	if _, err := tty.WriteString("\x1b]11;?\x07"); err != nil {
		return true, err
	}

	buf := make([]byte, 128)
	n, err := tty.Read(buf)
	if err != nil {
		return true, err
	}

	return parseBackgroundResponse(string(buf[:n]))
}

func parseBackgroundResponse(s string) (bool, error) {
	idx := strings.Index(s, "rgb:")
	if idx < 0 {
		return true, fmt.Errorf("no rgb in response")
	}

	rgb := s[idx+4:]
	parts := strings.SplitN(rgb, "/", 3)
	if len(parts) < 3 {
		return true, fmt.Errorf("invalid rgb")
	}

	parseHex := func(s string) (int, error) {
		s = strings.TrimRight(s, "\x07\x1b\\")
		s = strings.TrimSpace(s)
		var v int
		_, err := fmt.Sscanf(s, "%x", &v)
		return v, err
	}

	r, err := parseHex(parts[0])
	if err != nil {
		return true, err
	}
	g, err := parseHex(parts[1])
	if err != nil {
		return true, err
	}
	b, err := parseHex(parts[2])
	if err != nil {
		return true, err
	}

	if r > 255 || g > 255 || b > 255 {
		r >>= 8
		g >>= 8
		b >>= 8
	}

	brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return brightness < 128, nil
}