package ui

import "testing"

func TestBubblePickerSizeStandardTerminal(t *testing.T) {
	width, height := bubblePickerSize(90, 30, 3)
	if width != 86 {
		t.Fatalf("expected width 86, got %d", width)
	}
	if height != 9 {
		t.Fatalf("expected height 9, got %d", height)
	}
}

func TestBubblePickerSizeTinyTerminalStillFits(t *testing.T) {
	width, height := bubblePickerSize(20, 5, 25)
	if width > 20 {
		t.Fatalf("expected width to fit terminal, got %d", width)
	}
	if height > 5 {
		t.Fatalf("expected height to fit terminal, got %d", height)
	}
	if width <= 0 || height <= 0 {
		t.Fatalf("expected positive dimensions, got width=%d height=%d", width, height)
	}
}

func TestHuhSelectHeightBounds(t *testing.T) {
	if got := huhSelectHeight(0); got != 4 {
		t.Fatalf("expected minimum huh height 4, got %d", got)
	}
	if got := huhSelectHeight(3); got != 4 {
		t.Fatalf("expected huh height 4 for small lists, got %d", got)
	}
	if got := huhSelectHeight(20); got != 10 {
		t.Fatalf("expected max huh height 10, got %d", got)
	}
}
