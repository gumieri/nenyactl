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

// CatalogFromDescribe builds the picker catalog from nenya's describe provider
// catalog. entries are provider/model pairs (see CatalogModel); nenya owns the
// catalog, so no model is ever fabricated from a provider name.
func CatalogFromDescribe(entries []CatalogModel) Catalog {
	type key struct{ provider, model string }
	seen := make(map[key]bool)
	var catalog Catalog

	for _, e := range entries {
		if e.Provider == "" || e.Model == "" {
			continue
		}
		k := key{e.Provider, e.Model}
		if seen[k] {
			continue
		}
		seen[k] = true
		catalog.Models = append(catalog.Models, CatalogModel{Provider: e.Provider, Model: e.Model})
	}

	sort.Slice(catalog.Models, func(i, j int) bool {
		if catalog.Models[i].Provider != catalog.Models[j].Provider {
			return catalog.Models[i].Provider < catalog.Models[j].Provider
		}
		return catalog.Models[i].Model < catalog.Models[j].Model
	})
	return catalog
}
