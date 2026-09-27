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
	if settings["apiKey"] != "nk-test123" {
		t.Errorf("apiKey = %v", settings["apiKey"])
	}
	models, ok := nenya["models"].(map[string]any)
	if !ok {
		t.Fatal("V2 provider must carry a `models` object")
	}
	if _, ok := models["build"]; !ok {
		t.Error("models must include the build model")
	}
}

func TestMergeOpenCodePreservesUnrelatedKeys(t *testing.T) {
	existing := []byte(`{
  "theme": "dark",
  "providers": {
    "other": {"package": "x"}
  }
}`)
	merged, removed, err := MergeOpenCodeProvider(existing, testEndpoint())
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if removed {
		t.Error("no legacy provider should have been removed")
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("merged output invalid JSON: %v", err)
	}
	if root["theme"] != "dark" {
		t.Error("unrelated top-level key dropped")
	}
	provider, ok := root["providers"].(map[string]any)
	if !ok {
		t.Fatal("merged output has no providers object")
	}
	if _, ok := provider["other"]; !ok {
		t.Error("unrelated provider dropped")
	}
	nenya, ok := provider["nenya"].(map[string]any)
	if !ok {
		t.Fatal("merged output has no nenya provider")
	}
	if nenya["package"] != "@opencode/ai/providers/openai-compatible" {
		t.Errorf("package = %v", nenya["package"])
	}
	settings, ok := nenya["settings"].(map[string]any)
	if !ok {
		t.Fatal("nenya provider has no settings")
	}
	if settings["apiKey"] != "nk-test123" {
		t.Errorf("apiKey = %v", settings["apiKey"])
	}
	if settings["baseURL"] != "http://localhost:9090/v1" {
		t.Errorf("baseURL = %v", settings["baseURL"])
	}
}

func TestMergeOpenCodeMigratesLegacyV1(t *testing.T) {
	existing := []byte(`{
  "provider": {
    "nenya": {"npm": "@ai-sdk/openai-compatible"},
    "other": {"npm": "x"}
  }
}`)
	merged, removed, err := MergeOpenCodeProvider(existing, testEndpoint())
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !removed {
		t.Error("expected the legacy provider.nenya to be reported as removed")
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("merged output invalid JSON: %v", err)
	}
	providers, ok := root["providers"].(map[string]any)
	if !ok {
		t.Fatal("merged output has no providers object")
	}
	if _, ok := providers["nenya"]; !ok {
		t.Error("V2 nenya provider not added")
	}
	legacy, ok := root["provider"].(map[string]any)
	if !ok {
		t.Fatal("legacy provider object was dropped entirely")
	}
	if _, ok := legacy["nenya"]; ok {
		t.Error("legacy V1 provider.nenya was not removed")
	}
	if _, ok := legacy["other"]; !ok {
		t.Error("unrelated legacy provider was dropped")
	}
}

func TestMergeOpenCodePreservesComments(t *testing.T) {
	existing := []byte("{\n  // user comment\n  \"theme\": \"dark\"\n}\n")
	merged, _, err := MergeOpenCodeProvider(existing, testEndpoint())
	if err != nil {
		t.Fatalf("merge with comments: %v", err)
	}
	for _, want := range []string{"user comment", `"providers"`, `"nenya"`, "@opencode/ai/providers/openai-compatible"} {
		if !strings.Contains(string(merged), want) {
			t.Errorf("merged output missing %q:\n%s", want, merged)
		}
	}
}

func TestMergeOpenCodeRejectsNonObject(t *testing.T) {
	if _, _, err := MergeOpenCodeProvider([]byte(`[1,2,3]`), testEndpoint()); err == nil {
		t.Fatal("expected error for array config")
	}
}

func TestMergeOpenCodeIdempotent(t *testing.T) {
	ep := testEndpoint()
	first, _, err := MergeOpenCodeProvider(nil, ep)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := MergeOpenCodeProvider(first, ep)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("merge is not idempotent:\n%s\n---\n%s", first, second)
	}
}
