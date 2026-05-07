package config

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/tailscale/hujson"
)

type sourceInfo struct {
	filePath string
	value    *hujson.Value
}

func LoadEffectiveConfig(configFile, configD string) (*hujson.Value, map[string]sourceInfo, error) {
	sources := make(map[string]sourceInfo)

	entries, err := os.ReadDir(configD)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}

	var dropInFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".json" {
			dropInFiles = append(dropInFiles, filepath.Join(configD, entry.Name()))
		}
	}
	sort.Strings(dropInFiles)

	merged := &hujson.Value{Value: &hujson.Object{}}
	for _, file := range dropInFiles {
		v, err := jsonc.ReadFile(file)
		if err != nil {
			return nil, nil, err
		}
		obj, ok := jsonc.GetObject(v)
		if !ok {
			continue
		}
		for i := range obj.Members {
			key := jsonc.MemberName(&obj.Members[i])
			sources[key] = sourceInfo{
				filePath: file,
				value:    &obj.Members[i].Value,
			}
		}
		merged = mergeValues(merged, v)
	}

	configV, err := jsonc.ReadFile(configFile)
	if err != nil {
		return nil, nil, err
	}
	obj, ok := jsonc.GetObject(configV)
	if ok {
		for i := range obj.Members {
			key := jsonc.MemberName(&obj.Members[i])
			sources[key] = sourceInfo{
				filePath: configFile,
				value:    &obj.Members[i].Value,
			}
		}
	}
	merged = mergeValues(merged, configV)

	return merged, sources, nil
}

func mergeValues(base, overlay *hujson.Value) *hujson.Value {
	baseObj, baseOK := base.Value.(*hujson.Object)
	overlayObj, overlayOK := overlay.Value.(*hujson.Object)

	if !baseOK || !overlayOK {
		return overlay
	}

	for _, member := range overlayObj.Members {
		key := jsonc.MemberName(&member)
		exists := false
		for i := range baseObj.Members {
			if jsonc.MemberName(&baseObj.Members[i]) == key {
				baseObj.Members[i].Value = member.Value
				exists = true
				break
			}
		}
		if !exists {
			baseObj.Members = append(baseObj.Members, member)
		}
	}
	return base
}
