package joboutcome

import "testing"

func TestProcessAndHoldPassesWithProcessedRaster(t *testing.T) {
	outcome := Evaluate(map[string]string{
		"status":               "done ripping",
		"state":                "processed",
		"has disk raster?":     "yes",
		"total pages ripped":   "1",
		"total pages rendered": "1",
		"last joblog event":    "LOG_FINISHED",
	}, Policy{RequireProcessedRaster: true})
	if !outcome.Pass {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestProcessAndHoldFailsUnsupportedXPS(t *testing.T) {
	outcome := Evaluate(map[string]string{
		"status":            "done printing",
		"state":             "process canceled",
		"error":             "XPS not supported.",
		"pdl error":         "XPS not supported.",
		"has disk raster?":  "no",
		"last joblog event": "LOG_ABORTED",
	}, Policy{RequireProcessedRaster: true})
	if outcome.Pass || outcome.Error != "XPS not supported." {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestProcessAndHoldFailsInvalidPaperSize(t *testing.T) {
	outcome := Evaluate(map[string]string{
		"status":               "done printing",
		"state":                "process error",
		"error":                "Invalid Paper Size",
		"pdl error":            "Invalid Paper Size",
		"total pages rendered": "0",
	}, Policy{RequireProcessedRaster: true})
	if outcome.Pass || outcome.Reason == "" {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestExpectedCancellationPasses(t *testing.T) {
	outcome := Evaluate(map[string]string{"state": "process canceled", "has been canceled?": "yes"}, Policy{ExpectCanceled: true})
	if !outcome.Pass {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestProcessRequiresRasterEvidence(t *testing.T) {
	outcome := Evaluate(map[string]string{"status": "done ripping", "state": "processed", "has disk raster?": "no", "total pages ripped": "0"}, Policy{RequireProcessedRaster: true})
	if outcome.Pass {
		t.Fatalf("outcome = %#v", outcome)
	}
}
