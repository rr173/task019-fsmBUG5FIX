package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProbeRejectMultipleJSON verifies that the API rejects request bodies
// containing multiple concatenated JSON objects (only one is allowed).
func TestProbeRejectMultipleJSON(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()

	// Two valid JSON objects concatenated — should be rejected
	body := `{"name":"t","states":["a","b"],"initial":"a","transitions":[{"from":"a","event":"go","to":"b"}],"state":"a","event":"go"}{"name":"t","states":["a"],"initial":"a","transitions":[],"state":"a","event":"x"}`
	resp, err := http.Post(srv.URL+"/apply", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("multiple JSON objects: status = %d, want 400", resp.StatusCode)
	}
}
