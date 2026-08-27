package app

import "testing"

type textFitSpec struct {
	Name    string
	Text    string
	Width   int
	Padding int
}

func TestImportantUITextFitsWithoutLikelyTruncation(t *testing.T) {
	rects := rectMap(append(enterpriseLayoutRects(), settingsLayoutRects()...))
	specs := []textFitSpec{
		{Name: "settingsTitle", Text: "SETTINGS  SERVER CONNECTION", Width: rects["settingsTitle"].W, Padding: 8},
		{Name: "serverIPLabel", Text: "Server IP address", Width: rects["serverIPLabel"].W, Padding: 8},
		{Name: "secretLabel", Text: "Secret key", Width: rects["secretLabel"].W, Padding: 8},
		{Name: "passwordLabel", Text: "Admin password", Width: rects["passwordLabel"].W, Padding: 8},
		{Name: "settingsButton", Text: "Settings", Width: rects["settingsButton"].W, Padding: 28},
		{Name: "captureButton", Text: "Get server capabilities", Width: rects["captureButton"].W, Padding: 32},
		{Name: "runButton", Text: "Run automation", Width: rects["runButton"].W, Padding: 28},
		{Name: "cancelButton", Text: "Cancel run", Width: rects["cancelButton"].W, Padding: 28},
		{Name: "selectionMode", Text: "Random file", Width: rects["selectionMode"].W, Padding: 36},
		{Name: "runMode", Text: "Process and Hold", Width: rects["runMode"].W, Padding: 36},
		{Name: "strategy", Text: "All permutations", Width: rects["strategy"].W, Padding: 36},
		{Name: "maxCases", Text: "10000", Width: rects["maxCases"].W, Padding: 12},
	}
	for _, spec := range specs {
		if needed := estimatedSegoeUITextWidth(spec.Text) + spec.Padding; needed > spec.Width {
			t.Fatalf("%s likely truncates %q: estimated width %d + padding %d = %d, control width %d", spec.Name, spec.Text, estimatedSegoeUITextWidth(spec.Text), spec.Padding, needed, spec.Width)
		}
	}
}

func TestStatusAndHeadingsHaveEnoughWidth(t *testing.T) {
	rects := rectMap(enterpriseLayoutRects())
	specs := []textFitSpec{
		{Name: "status", Text: "Open Settings, enter server details, then click Get capabilities of the server.", Width: rects["status"].W, Padding: 8},
		{Name: "log", Text: "Activity log", Width: rects["log"].W, Padding: 8},
	}
	for _, spec := range specs {
		if needed := estimatedSegoeUITextWidth(spec.Text) + spec.Padding; needed > spec.Width {
			t.Fatalf("%s likely truncates %q: estimated required %d, width %d", spec.Name, spec.Text, needed, spec.Width)
		}
	}
}

func rectMap(rects []uiRect) map[string]uiRect {
	out := make(map[string]uiRect, len(rects))
	for _, rect := range rects {
		out[rect.Name] = rect
	}
	return out
}

// estimatedSegoeUITextWidth intentionally errs slightly high for default Windows
// dialog text at 96 DPI. It is a stable CI-friendly guard; final screenshot or
// manual review can still be used for high-DPI environment differences.
func estimatedSegoeUITextWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r == ' ':
			width += 4
		case r == 'i' || r == 'l' || r == 'I' || r == '.' || r == ',':
			width += 3
		case r == 'm' || r == 'w' || r == 'M' || r == 'W':
			width += 10
		case r >= 'A' && r <= 'Z':
			width += 8
		default:
			width += 7
		}
	}
	return width
}
