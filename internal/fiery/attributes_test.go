package fiery

import "testing"

func TestExtractJobAttributesFromNestedAttributes(t *testing.T) {
	body := []byte(`{"data":{"item":{"id":"JOB-1","attributes":{"EFResolution":"360x720dpi","status":"done spooling","job release state":"production"}}}}`)
	attrs := extractJobAttributes(body)
	if attrs["EFResolution"] != "360x720dpi" {
		t.Fatalf("EFResolution = %q", attrs["EFResolution"])
	}
	if attrs["status"] != "done spooling" {
		t.Fatalf("status = %q", attrs["status"])
	}
	if attrs["job release state"] != "production" {
		t.Fatalf("job release state = %q", attrs["job release state"])
	}
}

func TestExtractJobAttributesFromNestedJobEventShape(t *testing.T) {
	body := []byte(`{"data":{"item":{"job":{"id":"JOB-1","attributes":{"EFResolution":{"value":"360x360dpi"},"status":"done ripping"}}}}}`)
	attrs := extractJobAttributes(body)
	if attrs["EFResolution"] != "360x360dpi" {
		t.Fatalf("EFResolution = %q", attrs["EFResolution"])
	}
	if attrs["status"] != "done ripping" {
		t.Fatalf("status = %q", attrs["status"])
	}
}

func TestExtractJobAttributesFromItemsShape(t *testing.T) {
	body := []byte(`{"data":{"items":[{"id":"JOB-1","attributes":{"EFColorMode":"CMYK"}}]}}`)
	attrs := extractJobAttributes(body)
	if attrs["EFColorMode"] != "CMYK" {
		t.Fatalf("EFColorMode = %q", attrs["EFColorMode"])
	}
}
