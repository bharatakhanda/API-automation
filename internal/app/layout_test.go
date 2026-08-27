package app

import "testing"

func TestEnterpriseLayoutDoesNotOverlapActionableControls(t *testing.T) {
	rects := enterpriseLayoutRects()
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Layer == "status" || rects[j].Layer == "status" {
				continue
			}
			if overlaps(rects[i], rects[j]) {
				t.Fatalf("%s overlaps %s: %#v %#v", rects[i].Name, rects[j].Name, rects[i], rects[j])
			}
		}
	}
}

func TestSettingsLayoutDoesNotOverlapItself(t *testing.T) {
	rects := settingsLayoutRects()
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if overlaps(rects[i], rects[j]) {
				t.Fatalf("%s overlaps %s: %#v %#v", rects[i].Name, rects[j].Name, rects[i], rects[j])
			}
		}
	}
}

func TestSettingsLayoutFitsInsideWindow(t *testing.T) {
	for _, r := range settingsLayoutRects() {
		if r.X < 0 || r.Y < 0 || r.X+r.W > windowWidth || r.Y+r.H > windowHeight {
			t.Fatalf("%s is outside %dx%d window: %#v", r.Name, windowWidth, windowHeight, r)
		}
	}
}

func TestEnterpriseLayoutFitsInsideWindow(t *testing.T) {
	for _, r := range enterpriseLayoutRects() {
		if r.X < 0 || r.Y < 0 || r.X+r.W > windowWidth || r.Y+r.H > windowHeight {
			t.Fatalf("%s is outside %dx%d window: %#v", r.Name, windowWidth, windowHeight, r)
		}
	}
}

func TestEnterpriseLayoutHasReadableSpacing(t *testing.T) {
	for _, r := range enterpriseLayoutRects() {
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("%s has invalid size: %#v", r.Name, r)
		}
	}
	assertVerticalGap(t, "folderPath", "selectionMode", 30)
	assertVerticalGap(t, "selectionMode", "method", 60)
	assertVerticalGap(t, "method", "results", 80)
	assertVerticalGap(t, "results", "log", 30)
}

func assertVerticalGap(t *testing.T, upper, lower string, minGap int) {
	t.Helper()
	rects := enterpriseLayoutRects()
	u, ok := findRect(rects, upper)
	if !ok {
		t.Fatalf("missing rect %s", upper)
	}
	l, ok := findRect(rects, lower)
	if !ok {
		t.Fatalf("missing rect %s", lower)
	}
	if gap := l.Y - (u.Y + u.H); gap < minGap {
		t.Fatalf("gap between %s and %s = %d, want >= %d", upper, lower, gap, minGap)
	}
}

func findRect(rects []uiRect, name string) (uiRect, bool) {
	for _, r := range rects {
		if r.Name == name {
			return r, true
		}
	}
	return uiRect{}, false
}

func overlaps(a, b uiRect) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}
