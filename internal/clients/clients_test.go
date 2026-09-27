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

func TestRenderOpenCodeUsesV2Shape(t *testing.T) {
	s, err := Render(OpenCode, testEndpoint())
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(s.Body), &root); err != nil {
		t.Fatalf("opencode snippet is not JSON: %v", err)
	}
	if _, ok := root["provider"]; ok {
		t.Error("snippet must not emit the V1 `provider` key")
	}
	providers, ok := root["providers"].(map[string]any)
	if !ok {
		t.Fatal("snippet must emit the V2 `providers` map")
	}
	nenya, ok := providers["nenya"].(map[string]any)
	if !ok {
		t.Fatal("snippet must define the nenya provider")
	}
	if nenya["package"] != "@opencode/ai/providers/openai-compatible" {
		t.Errorf("package = %v", nenya["package"])
	}
	if _, ok := nenya["npm"]; ok {
		t.Error("V2 provider must not use the V1 `npm` field")
	}
	if _, ok := nenya["options"]; ok {
		t.Error("V2 provider must not use the V1 `options` field")
	}
	settings, ok := nenya["settings"].(map[string]any)
	if !ok {
		t.Fatal("V2 provider must carry a `settings` object")
	}
	if settings["baseURL"] != "http://localhost:9090/v1" {
		t.Errorf("baseURL = %v", settings["baseURL"])
	}
}

func TestMergeOpenCodePreservesUnrelatedKeys(t *testing.T) {
	existing := []byte(`{
  "theme": "dark",
  "providers": {
    "other": {"package": "x"}
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
	provider := root["providers"].(map[string]any)
	if _, ok := provider["other"]; !ok {
		t.Error("unrelated provider dropped")
	}
	nenya := provider["nenya"].(map[string]any)
	if nenya["package"] != "@opencode/ai/providers/openai-compatible" {
		t.Errorf("package = %v", nenya["package"])
	}
	settings := nenya["settings"].(map[string]any)
	if settings["apiKey"] != "nk-test123" {
		t.Errorf("apiKey = %v", settings["apiKey"])
	}
	if settings["baseURL"] != "http://localhost:9090/v1" {
		t.Errorf("baseURL = %v", settings["baseURL"])
	}
}

func TestMergeOpenCodePreservesComments(t *testing.T) {
	existing := []byte("{\n  // user comment\n  \"theme\": \"dark\"\n}\n")
	merged, err := MergeOpenCodeProvider(existing, testEndpoint())
	if err != nil {
		t.Fatalf("merge with comments: %v", err)
	}
	if !strings.Contains(string(merged), "user comment") {
		t.Errorf("comment was not preserved:\n%s", merged)
	}
	if !strings.Contains(string(merged), "nenya") {
		t.Errorf("provider not added:\n%s", merged)
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
