package clients

import (
	"encoding/json"
	"strings"
	"testing"
)

func testEndpoint() Endpoint {
	return Endpoint{BaseURL: "http://localhost:9090", Token: "nk-test123"}
}

func TestParseName(t *testing.T) {
	for _, name := range Supported() {
		got, err := ParseName(string(name))
		if err != nil || got != name {
			t.Errorf("ParseName(%q) = %v, %v", name, got, err)
		}
	}
	if _, err := ParseName("bogus"); err == nil {
		t.Fatal("expected error for unknown client")
	}
}

func TestRenderUsesResolvedEndpoint(t *testing.T) {
	ep := testEndpoint()
	for _, name := range Supported() {
		s, err := Render(name, ep)
		if err != nil {
			t.Fatalf("Render(%s): %v", name, err)
		}
		if !strings.Contains(s.Body, ep.Token) {
			t.Errorf("%s snippet missing token", name)
		}
		if !strings.Contains(s.Body, "9090") {
			t.Errorf("%s snippet missing resolved port", name)
		}
		if strings.Contains(s.Body, "8080") {
			t.Errorf("%s snippet hardcodes 8080", name)
		}
	}
}

func TestRenderTrailingSlashNormalized(t *testing.T) {
	s, err := Render(OpenCode, Endpoint{BaseURL: "http://localhost:8080/", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.Body, "8080//v1") {
		t.Errorf("base URL not normalized: %s", s.Body)
	}
}

func TestMergeOpenCodePreservesUnrelatedKeys(t *testing.T) {
	existing := []byte(`{
  "theme": "dark",
  "provider": {
    "other": {"npm": "x"}
  }
}`)
	merged, err := MergeOpenCodeProvider(existing, testEndpoint())
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("merged output invalid JSON: %v", err)
	}
	if root["theme"] != "dark" {
		t.Error("unrelated top-level key dropped")
	}
	provider := root["provider"].(map[string]any)
	if _, ok := provider["other"]; !ok {
		t.Error("unrelated provider dropped")
	}
	nenya := provider["nenya"].(map[string]any)
	opts := nenya["options"].(map[string]any)
	if opts["apiKey"] != "nk-test123" {
		t.Errorf("apiKey = %v", opts["apiKey"])
	}
	if opts["baseURL"] != "http://localhost:9090/v1" {
		t.Errorf("baseURL = %v", opts["baseURL"])
	}
}

func TestMergeOpenCodeRejectsNonObject(t *testing.T) {
	if _, err := MergeOpenCodeProvider([]byte(`[1,2,3]`), testEndpoint()); err == nil {
		t.Fatal("expected error for array config")
	}
}

func TestMergeOpenCodeIdempotent(t *testing.T) {
	ep := testEndpoint()
	first, err := MergeOpenCodeProvider(nil, ep)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MergeOpenCodeProvider(first, ep)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("merge is not idempotent:\n%s\n---\n%s", first, second)
	}
}
