package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/hyperbricks/hyperbricks/pkg/logging"
	"github.com/hyperbricks/hyperbricks/pkg/shared"
)

type Fields struct {
	Entry             string `mapstructure:"entry"`
	Outfile           string `mapstructure:"outfile"`
	Binary            string `mapstructure:"binary"`
	Enclose           string `mapstructure:"enclose"`
	Minify            bool   `mapstructure:"minify"`
	MinifyIdentifiers bool   `mapstructure:"minify_identifiers"`
	Mangle            bool   `mapstructure:"mangle"`
	Sourcemap         bool   `mapstructure:"sourcemap"`
	Debug             bool   `mapstructure:"debug"`
}

type Config struct {
	shared.Component `mapstructure:",squash"`
	PluginName       string `mapstructure:"plugin"`
	Fields           `mapstructure:"data"`
}

type EsbuildPlugin struct{}

var _ shared.PluginRenderer = (*EsbuildPlugin)(nil)

func (p *EsbuildPlugin) Render(instance interface{}, ctx context.Context) (string, []error) {
	var decodeErrs []shared.ComponentError
	var cfg Config

	if err := shared.DecodeWithBasicHooks(instance, &cfg); err != nil {
		decodeErrs = append(decodeErrs, shared.ComponentError{
			Hash:     shared.GenerateHash(),
			Path:     cfg.HyperBricksPath,
			Key:      cfg.HyperBricksKey,
			Rejected: true,
			Err:      fmt.Sprintf("decode error: %v", err),
		})
		errs := make([]error, len(decodeErrs))
		for i, e := range decodeErrs {
			errs[i] = e
		}
		return "", errs
	}

	log := logging.GetLogger()
	bin := cfg.Fields.Binary
	entry := cfg.Fields.Entry
	out := cfg.Fields.Outfile
	enclose := cfg.Fields.Enclose
	minify := cfg.Fields.Minify
	minifyIdentifiers := cfg.Fields.MinifyIdentifiers
	mangle := cfg.Fields.Mangle
	sourcemap := cfg.Fields.Sourcemap
	debug := cfg.Fields.Debug

	if entry == "" || out == "" {
		return "", []error{configErr(cfg, "both entry and outfile must be set")}
	}

	hbConfig := shared.GetHyperBricksConfiguration()
	resourcesDir, okRes := hbConfig.Directories["resources"]
	staticDir, okStat := hbConfig.Directories["static"]
	if !okRes || !okStat {
		return "", []error{configErr(cfg, "Could not find 'resources' or 'static' directories in hbConfig.Directories")}
	}

	ensureTrailingSlash := func(path string) string {
		path = filepath.ToSlash(filepath.Clean(path))
		if !strings.HasSuffix(path, "/") {
			return path + "/"
		}
		return path
	}
	resourcesDir = ensureTrailingSlash(resourcesDir)
	staticDir = ensureTrailingSlash(staticDir)

	entryPath := filepath.Join(resourcesDir, entry)
	outPath := filepath.Join(staticDir, out)

	if bin == "" {
		buildOpts := api.BuildOptions{
			EntryPoints:       []string{entryPath},
			Bundle:            true,
			Outfile:           outPath,
			Write:             true,
			Sourcemap:         api.SourceMapNone,
			MinifyWhitespace:  minify,
			MinifySyntax:      minify,
			MinifyIdentifiers: minifyIdentifiers,
		}
		if sourcemap {
			buildOpts.Sourcemap = api.SourceMapLinked
		}
		if mangle {
			buildOpts.MangleProps = ".*"
		}
		if debug {
			optsJson, err := json.MarshalIndent(buildOpts, "", "  ")
			if err != nil {
				log.Info("Esbuild API options: (failed to marshal BuildOptions: ", err, ")")
			} else {
				log.Info("Esbuild API options:\n" + string(optsJson))
			}
		}
		res := api.Build(buildOpts)
		if len(res.Errors) > 0 {
			var errs []error
			for _, e := range res.Errors {
				errs = append(errs, fmt.Errorf("esbuild API error: %s", e.Text))
			}
			return "", errs
		}
	} else {
		args := []string{"--bundle", "--outfile=" + outPath}
		if minify {
			args = append(args, "--minify")
		}
		if minifyIdentifiers {
			args = append(args, "--minify-identifiers")
		}
		if mangle {
			args = append(args, "--mangle-props=.*")
		}
		if sourcemap {
			args = append(args, "--sourcemap")
		}
		args = append(args, entryPath)

		if debug {
			log.Info("Running esbuild CLI:", bin, args)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		if err := cmd.Run(); err != nil {
			return "", []error{fmt.Errorf("esbuild CLI error: %v", err)}
		}
	}

	// Compute the static web path (relative to the "static/" dir)
	var webPath string
	if idx := strings.Index(staticDir, "static"); idx >= 0 {
		webPath = filepath.ToSlash(filepath.Join(staticDir[idx+len("static"):], out))
		webPath = "static" + webPath // Ensure prefix for web usage
		webPath = strings.ReplaceAll(webPath, "//", "/")
	} else {
		// fallback: just outfile
		webPath = "static/" + filepath.ToSlash(out)
	}

	var result string
	if enclose != "" {
		result = shared.EncloseContent(enclose, webPath)
	} else {
		result = webPath
	}
	return result, nil
}

func Plugin() (shared.PluginRenderer, error) {
	return &EsbuildPlugin{}, nil
}

func configErr(cfg Config, msg string) shared.ComponentError {
	return shared.ComponentError{
		Hash:     shared.GenerateHash(),
		Path:     cfg.HyperBricksPath,
		Key:      cfg.HyperBricksKey,
		Rejected: true,
		Err:      msg,
	}
}
