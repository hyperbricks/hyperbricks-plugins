package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PluginManifest struct {
	Plugin                string   `json:"plugin"`
	Version               string   `json:"version"`
	Source                string   `json:"source"`
	CompatibleHyperbricks []string `json:"compatible_hyperbricks"`
	Description           string   `json:"description,omitempty"`
	// Add other fields as needed
}

func main() {
	root := "plugins"
	index := make(map[string]map[string]PluginManifest)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), "manifest.json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			return nil
		}

		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", path, err)
			return nil
		}

		if _, ok := index[manifest.Plugin]; !ok {
			index[manifest.Plugin] = make(map[string]PluginManifest)
		}
		index[manifest.Plugin][manifest.Version] = manifest

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Walk error: %v\n", err)
		os.Exit(1)
	}

	out, err := os.Create("plugins.index.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot write plugins.index.json: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(index); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot encode JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("plugins.index.json updated.")
}
