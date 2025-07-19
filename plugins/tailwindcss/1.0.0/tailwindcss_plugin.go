package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hyperbricks/hyperbricks/pkg/logging"
	"github.com/hyperbricks/hyperbricks/pkg/shared"
	"go.uber.org/zap"
)

// Fields defines the plugin configuration
type Fields struct {
	InputCSS  string `mapstructure:"input_css"`  // Input CSS file (with @config if needed)
	OutputCSS string `mapstructure:"output_css"` // Output CSS file
	Config    string `mapstructure:"config"`     // (Optional) Tailwind config path (use @config in input CSS for v4+)
	Binary    string `mapstructure:"binary"`     // (Optional) Tailwind CLI binary path (default: "tailwindcss")
	Signal    bool   `mapstructure:"signal"`     // Run signal test before build
	Enclose   string `mapstructure:"enclose"`    // (Optional) Wrap output (not recommended for static file usage)
	Minify    bool   `mapstructure:"minify"`     // Pass --minify to CLI
	Debug     bool   `mapstructure:"debug"`      // Show verbose CLI/stdout/stderr logging
}

type TailwindConfig struct {
	shared.Component `mapstructure:",squash"`
	PluginName       string `mapstructure:"plugin"`
	Fields           `mapstructure:"data"`
}

type TailwindPlugin struct{}

var _ shared.PluginRenderer = (*TailwindPlugin)(nil)

// Runs a basic check if the Tailwind binary is available and functional
func signalTest(ctx context.Context, bin string) error {
	const input = "@import \"tailwindcss\";\n<div class=\"text-red-500\"></div>\n"
	cmd := exec.CommandContext(ctx, bin, "-i", "-", "--minify")
	cmd.Stdin = strings.NewReader(input)

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("signal test failed: %v — %s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), ".text-red-500") {
		return fmt.Errorf(".text-red-500 not found in output")
	}
	return nil
}

// Logs every line from a reader (stdout/stderr)
func dumpPipe(logger *zap.SugaredLogger, prefix string, r io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		logger.Info(fmt.Sprintf("%s: %s", prefix, scanner.Text()))
	}
}

// Main plugin logic
func (p *TailwindPlugin) Render(instance interface{}, ctx context.Context) (string, []error) {
	var errs []error
	var cfg TailwindConfig
	if err := shared.DecodeWithBasicHooks(instance, &cfg); err != nil {
		errs = append(errs, shared.ComponentError{
			Hash:     shared.GenerateHash(),
			Path:     cfg.HyperBricksPath,
			Key:      cfg.HyperBricksKey,
			Rejected: true,
			Err:      fmt.Sprintf("decode error: %v", err),
		})
		return "", errs
	}

	bin := cfg.Fields.Binary
	if bin == "" {
		bin = "tailwindcss"
	}
	logger := logging.GetLogger()

	// Optional signal test (sanity check for Tailwind CLI)
	if cfg.Fields.Signal {
		if cfg.Fields.Debug {
			logger.Info("→ Running tailwind signal test")
		}
		if err := signalTest(ctx, bin); err != nil {
			errs = append(errs, shared.ComponentError{
				Hash:     shared.GenerateHash(),
				Path:     cfg.HyperBricksPath,
				Key:      cfg.HyperBricksKey,
				Rejected: true,
				Err:      fmt.Sprintf("signal test error: %v", err),
			})
			return "", errs
		}
		if cfg.Fields.Debug {
			absPath, _ := filepath.Abs(cfg.Fields.InputCSS)
			wd, _ := os.Getwd()
			logger.Info("absPath:", absPath, "os.Getwd():", wd)
			logger.Info("✅ Tailwind signal test passed")
		}
	}

	if cfg.Fields.OutputCSS == "" {
		errs = append(errs, shared.ComponentError{
			Hash:     shared.GenerateHash(),
			Path:     cfg.HyperBricksPath,
			Key:      cfg.HyperBricksKey,
			Rejected: true,
			Err:      "output_css field must be provided",
		})
		return "", errs
	}

	// Compose CLI args for v4+: only -i/-o/(--minify)
	args := []string{"-i", cfg.Fields.InputCSS, "-o", cfg.Fields.OutputCSS}
	if cfg.Fields.Minify {
		args = append(args, "--minify")
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	if cfg.Fields.Debug {
		logger.Info("→ Running tailwind CLI: " + strings.Join(args, " "))
	}

	if cfg.Fields.Debug {
		stdoutPipe, _ := cmd.StdoutPipe()
		stderrPipe, _ := cmd.StderrPipe()

		if err := cmd.Start(); err != nil {
			errs = append(errs, shared.ComponentError{
				Hash:     shared.GenerateHash(),
				Path:     cfg.HyperBricksPath,
				Key:      cfg.HyperBricksKey,
				Rejected: true,
				Err:      fmt.Sprintf("failed to start tailwind: %v", err),
			})
			return "", errs
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go dumpPipe(logger, "STDOUT", stdoutPipe, &wg)
		go dumpPipe(logger, "STDERR", stderrPipe, &wg)
		wg.Wait()
		if err := cmd.Wait(); err != nil {
			errs = append(errs, shared.ComponentError{
				Hash:     shared.GenerateHash(),
				Path:     cfg.HyperBricksPath,
				Key:      cfg.HyperBricksKey,
				Rejected: true,
				Err:      fmt.Sprintf("tailwind failed: %v", err),
			})
			return "", errs
		}
	} else {
		if err := cmd.Run(); err != nil {
			errs = append(errs, shared.ComponentError{
				Hash:     shared.GenerateHash(),
				Path:     cfg.HyperBricksPath,
				Key:      cfg.HyperBricksKey,
				Rejected: true,
				Err:      fmt.Sprintf("tailwind failed: %v", err),
			})
			return "", errs
		}
	}

	result := ""
	if cfg.Fields.Enclose != "" {
		cssBytes, err := os.ReadFile(cfg.Fields.OutputCSS)
		if err != nil {
			errs = append(errs, shared.ComponentError{
				Hash:     shared.GenerateHash(),
				Path:     cfg.HyperBricksPath,
				Key:      cfg.HyperBricksKey,
				Rejected: true,
				Err:      fmt.Sprintf("failed to read output CSS: %v", err),
			})
			return "", errs
		}

		// Get static dir from HyperBricks config
		hbConfig := shared.GetHyperBricksConfiguration()
		staticDir := ""
		if tbstatic, ok := hbConfig.Directories["static"]; ok {
			staticDir = filepath.Clean(tbstatic)
			staticDir = filepath.ToSlash(staticDir) // normalize
		}

		// Make relPath absolute and normalized
		absOut, _ := filepath.Abs(cfg.Fields.OutputCSS)
		absOut = filepath.ToSlash(absOut)

		// Find "static" in the output path and extract everything after it
		relPath := ""
		if staticDir != "" {
			idx := strings.Index(absOut, staticDir)
			if idx >= 0 {
				relPath = absOut[idx+len(staticDir):]
				relPath = strings.TrimLeft(relPath, "/")
				relPath = "static/" + relPath // always prefix with static/
			}
		}

		result = cfg.Fields.Enclose
		result = strings.ReplaceAll(result, "|", relPath)
		result = strings.ReplaceAll(result, "{{css}}", string(cssBytes))

		logger.Info("Enclose | absOut:", absOut)
		logger.Info("Enclose | staticDir:", staticDir)
		logger.Info("Enclose | relPath (for link):", relPath)
	}
	return result, errs

}

func Plugin() (shared.PluginRenderer, error) {
	return &TailwindPlugin{}, nil
}
