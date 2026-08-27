package preflight

import (
	"testing"

	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"
)

func TestRunProducesCertificationSnapshot(t *testing.T) {
	model := capabilities.Model{
		ServerName: "SERVER-85",
		Version:    "1.4",
		Queues:     []capabilities.Queue{{Name: "hold", Available: true}},
		Options: []capabilities.Option{
			{ID: "EFResolution", Label: "Resolution", Value: "360x720dpi", Values: []string{"360x720dpi"}},
			{ID: "EFBrightness", Label: "Brightness", Value: "00.00", Values: []string{"00.00"}},
			{ID: "EFColorMode", Label: "Color mode", Value: "CMYKPLUS", Values: []string{"CMYKPLUS"}},
			{ID: "EFMediaType", Label: "Media type", Value: "Board", Values: []string{"Board"}},
			{ID: "InputSlot", Label: "Input slot", Value: "Feeder1", Values: []string{"Feeder1"}},
			{ID: "EFOutputBin", Label: "Output bin", Value: "Stacker", Values: []string{"Stacker"}},
			{ID: "EFTextGfxQual", Label: "Quality", Value: "High", Values: []string{"High"}},
			{ID: "EFRotateDocument", Label: "Rotation", Value: "0", Values: []string{"0"}},
			{ID: "EFPrintSpeed", Label: "Print speed", Value: "Standard", Values: []string{"Standard"}},
		},
	}
	snapshot := Run(fiery.CapabilitySnapshot{Server: "https://server", APIVersion: "v5+v4"}, model)
	if snapshot.OverallStatus != "PASS" {
		t.Fatalf("status = %s failed=%v", snapshot.OverallStatus, snapshot.FailedProperties)
	}
	if !snapshot.Certification.Certified {
		t.Fatal("expected certified")
	}
}
