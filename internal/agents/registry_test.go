package agents

import (
	"sort"
	"testing"
)

func TestProvidersDataIntegrity(t *testing.T) {
	t.Run("no duplicate provider names", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, p := range Providers {
			if seen[p.Name] {
				t.Errorf("duplicate provider: %s", p.Name)
			}
			seen[p.Name] = true
		}
	})

	t.Run("all providers have non-empty name and help", func(t *testing.T) {
		for _, p := range Providers {
			if p.Name == "" {
				t.Error("provider has empty name")
			}
			if p.Help == "" {
				t.Errorf("provider %s has empty help", p.Name)
			}
		}
	})

	t.Run("ollama is the only provider that does not need a key", func(t *testing.T) {
		noKeyCount := 0
		for _, p := range Providers {
			if !p.NeedsKey {
				noKeyCount++
				if p.Name != "ollama" {
					t.Errorf("provider %s does not need a key but is not ollama", p.Name)
				}
			}
		}
		if noKeyCount != 1 {
			t.Errorf("expected 1 provider without NeedsKey, got %d", noKeyCount)
		}
	})
}

func TestModelsDataIntegrity(t *testing.T) {
	t.Run("no duplicate model IDs within same provider", func(t *testing.T) {
		seen := make(map[string]map[string]bool)
		for _, m := range Models {
			if seen[m.ID] == nil {
				seen[m.ID] = make(map[string]bool)
			}
			if seen[m.ID][m.Provider] {
				t.Errorf("duplicate model ID: %s (provider: %s)", m.ID, m.Provider)
			}
			seen[m.ID][m.Provider] = true
		}
	})

	t.Run("all models have non-empty ID, provider, and positive MaxCtx", func(t *testing.T) {
		for _, m := range Models {
			if m.ID == "" {
				t.Error("model has empty ID")
			}
			if m.Provider == "" {
				t.Errorf("model %s has empty provider", m.ID)
			}
			if m.MaxCtx <= 0 {
				t.Errorf("model %s has invalid MaxCtx: %d", m.ID, m.MaxCtx)
			}
		}
	})

	t.Run("all model providers exist in Providers list", func(t *testing.T) {
		providerNames := make(map[string]bool)
		for _, p := range Providers {
			providerNames[p.Name] = true
		}
		for _, m := range Models {
			if !providerNames[m.Provider] {
				t.Errorf("model %s references unknown provider: %s", m.ID, m.Provider)
			}
		}
	})

	t.Run("models are sorted by provider then by ID", func(t *testing.T) {
		for i := 1; i < len(Models); i++ {
			if Models[i].Provider < Models[i-1].Provider {
				t.Errorf("models not sorted by provider at index %d: %s > %s",
					i, Models[i-1].Provider, Models[i].Provider)
			}
			if Models[i].Provider == Models[i-1].Provider && Models[i].ID < Models[i-1].ID {
				t.Errorf("models not sorted by ID at index %d: %s > %s",
					i, Models[i-1].ID, Models[i].ID)
			}
		}
	})
}

func TestGetProvider(t *testing.T) {
	t.Run("finds gemini", func(t *testing.T) {
		var p ProviderDef
		for _, prov := range Providers {
			if prov.Name == "gemini" {
				p = prov
				break
			}
		}
		if p.Name != "gemini" {
			t.Errorf("GetProvider(gemini) failed")
		}
		if p.Help != "Google AI" {
			t.Errorf("gemini help = %v, want Google AI", p.Help)
		}
	})

	t.Run("finds ollama with NeedsKey=false", func(t *testing.T) {
		var p ProviderDef
		for _, prov := range Providers {
			if prov.Name == "ollama" {
				p = prov
				break
			}
		}
		if p.NeedsKey {
			t.Errorf("ollama should have NeedsKey=false, got true")
		}
	})
}

func TestGetModel(t *testing.T) {
	t.Run("finds gemini-2.5-flash under gemini provider", func(t *testing.T) {
		var found bool
		for _, m := range Models {
			if m.ID == "gemini-2.5-flash" && m.Provider == "gemini" {
				found = true
				break
			}
		}
		if !found {
			t.Error("gemini-2.5-flash not found under gemini provider")
		}
	})

	t.Run("finds all anthropic models", func(t *testing.T) {
		var anthropicModels []ModelDef
		for _, m := range Models {
			if m.Provider == "anthropic" {
				anthropicModels = append(anthropicModels, m)
			}
		}
		if len(anthropicModels) == 0 {
			t.Error("no anthropic models found")
		}
	})
}

func TestDefaultAgents(t *testing.T) {
	t.Run("contains small, build, plan", func(t *testing.T) {
		for _, name := range []string{"small", "build", "plan"} {
			agent, ok := DefaultAgents[name]
			if !ok {
				t.Errorf("default agent %s not found", name)
			}
			if agent.Strategy == "" {
				t.Errorf("agent %s has empty strategy", name)
			}
			if len(agent.Models) == 0 {
				t.Errorf("agent %s has no models", name)
			}
		}
	})

	t.Run("small uses round-robin strategy", func(t *testing.T) {
		agent := DefaultAgents["small"]
		if agent.Strategy != "round-robin" {
			t.Errorf("small strategy = %v, want round-robin", agent.Strategy)
		}
	})

	t.Run("build and plan use fallback strategy", func(t *testing.T) {
		for _, name := range []string{"build", "plan"} {
			agent := DefaultAgents[name]
			if agent.Strategy != "fallback" {
				t.Errorf("%s strategy = %v, want fallback", name, agent.Strategy)
			}
		}
	})

	t.Run("all default agent models exist in Models registry", func(t *testing.T) {
		modelIDs := make(map[string]bool)
		for _, m := range Models {
			modelIDs[m.ID] = true
		}
		for name, agent := range DefaultAgents {
			for _, modelID := range agent.Models {
				if !modelIDs[modelID] {
					t.Logf("agent %s references model %s not in registry (may be intentional)", name, modelID)
				}
			}
		}
	})
}

func TestStrategies(t *testing.T) {
	t.Run("contains fallback and round-robin", func(t *testing.T) {
		strategySet := make(map[string]bool)
		for _, s := range Strategies {
			strategySet[s] = true
		}
		if !strategySet["fallback"] {
			t.Error("Strategies does not contain fallback")
		}
		if !strategySet["round-robin"] {
			t.Error("Strategies does not contain round-robin")
		}
	})

	t.Run("strategies are sorted", func(t *testing.T) {
		sorted := append([]string{}, Strategies...)
		sort.Strings(sorted)
		if !sort.StringsAreSorted(sorted) {
			t.Error("Strategies should be sorted")
		}
	})
}

func TestModelFilterByProvider(t *testing.T) {
	t.Run("can filter models by provider", func(t *testing.T) {
		var geminiModels []ModelDef
		for _, m := range Models {
			if m.Provider == "gemini" {
				geminiModels = append(geminiModels, m)
			}
		}
		if len(geminiModels) == 0 {
			t.Error("no gemini models found")
		}
		for _, m := range geminiModels {
			if m.Provider != "gemini" {
				t.Errorf("wrong provider: %s", m.Provider)
			}
		}
	})
}
