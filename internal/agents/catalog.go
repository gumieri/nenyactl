package agents

import "sort"

// Catalog is the model/provider catalog the agent editor offers. It is built
// from `nenya describe --json` (the authoritative catalog), never from a
// hardcoded list: the boundary rule makes nenya the source of truth for which
// models and providers exist and what they are called.
type Catalog struct {
	// Models is every selectable model, in (provider, model) order.
	Models []CatalogModel
}

// CatalogModel is one selectable model.
type CatalogModel struct {
	Provider string
	Model    string
}

// CatalogFromDescribe builds the picker catalog from nenya's describe
// providers: the provider catalog plus, when present, the configured provider
// names. Providers are de-duplicated and models keep their provider
// association. entries are (provider, model) pairs from
// `describe.providers.catalog[]`; configured is `describe.providers.configured`.
func CatalogFromDescribe(entries [][2]string, configured []string) Catalog {
	type key struct{ provider, model string }
	seen := make(map[key]bool)
	var catalog Catalog

	add := func(provider, model string) {
		if provider == "" || model == "" {
			return
		}
		k := key{provider, model}
		if seen[k] {
			return
		}
		seen[k] = true
		catalog.Models = append(catalog.Models, CatalogModel{Provider: provider, Model: model})
	}

	for _, e := range entries {
		add(e[0], e[1])
	}

	// A configured provider with no catalog entry still gets a row so the
	// picker is never empty when nenya reports configured providers but no
	// catalog (contract target).
	providerSeen := make(map[string]bool)
	for _, m := range catalog.Models {
		providerSeen[m.Provider] = true
	}
	for _, provider := range configured {
		if provider == "" || providerSeen[provider] {
			continue
		}
		providerSeen[provider] = true
		add(provider, provider)
	}

	sort.Slice(catalog.Models, func(i, j int) bool {
		if catalog.Models[i].Provider != catalog.Models[j].Provider {
			return catalog.Models[i].Provider < catalog.Models[j].Provider
		}
		return catalog.Models[i].Model < catalog.Models[j].Model
	})
	return catalog
}
