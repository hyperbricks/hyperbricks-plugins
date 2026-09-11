package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperbricks/hyperbricks/pkg/shared"
)

type Fields struct {
	UploadDir    string   `mapstructure:"upload_dir"`
	InputName    string   `mapstructure:"input_name"`
	Label        string   `mapstructure:"label"`
	Button       string   `mapstructure:"button"`
	Action       string   `mapstructure:"action"`
	Accept       []string `mapstructure:"accept"`
	AllowedExts  []string `mapstructure:"allowed_exts"`
	AllowedTypes []string `mapstructure:"allowed_types"`
	MaxMB        int      `mapstructure:"max_mb"`
	Multiple     bool     `mapstructure:"multiple"`
	ShowSavedDir bool     `mapstructure:"show_saved_dir"`
}

type UploadPluginConfig struct {
	shared.Component `mapstructure:",squash"`
	PluginName       string `mapstructure:"plugin"`
	Fields           `mapstructure:"data"`
}

type UploadPlugin struct{}

var _ shared.PluginRenderer = (*UploadPlugin)(nil)

func (p *UploadPlugin) Render(instance interface{}, ctx context.Context) (any, []error) {
	var config UploadPluginConfig
	var errors []error

	if err := shared.DecodeWithBasicHooks(instance, &config); err != nil {
		errors = append(errors, shared.ComponentError{
			Hash:     shared.GenerateHash(),
			Path:     config.HyperBricksPath,
			Key:      config.HyperBricksKey,
			Rejected: true,
			Err:      fmt.Sprintf("Failed to decode upload plugin instance: %v", err),
		})
		return "<!-- upload plugin decode failed -->", errors
	}

	applyDefaults(&config.Fields)

	req, _ := ctx.Value(shared.Request).(*http.Request)
	writer, _ := ctx.Value(shared.ResponseWriter).(http.ResponseWriter)
	if writer != nil {
		writer.Header().Set("Cache-Control", "no-store")
	}
	if req == nil {
		return "<!-- upload plugin requires request context -->", errors
	}

	if req.Method == http.MethodPost {
		return p.handleUpload(req, writer, config, &errors), errors
	}

	return renderForm(req, config), errors
}

func (p *UploadPlugin) handleUpload(req *http.Request, writer http.ResponseWriter, config UploadPluginConfig, errors *[]error) string {
	contentType := strings.TrimSpace(req.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return renderStatus(req, config, "Upload failed", "Expected multipart/form-data request.", true)
	}

	maxBytes := int64(config.Fields.MaxMB) << 20
	if maxBytes > 0 && writer != nil {
		req.Body = http.MaxBytesReader(writer, req.Body, maxBytes)
	}

	if err := req.ParseMultipartForm(maxBytes); err != nil {
		return renderStatus(req, config, "Upload failed", fmt.Sprintf("Could not parse upload: %v", err), true)
	}
	defer func() {
		if req.MultipartForm != nil {
			_ = req.MultipartForm.RemoveAll()
		}
	}()

	files := req.MultipartForm.File[config.Fields.InputName]
	if len(files) == 0 {
		return renderStatus(req, config, "Upload failed", fmt.Sprintf("No file found in field %q.", config.Fields.InputName), true)
	}
	if !config.Fields.Multiple && len(files) > 1 {
		files = files[:1]
	}

	uploadDir := resolveUploadDir(config.Fields.UploadDir)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return renderStatus(req, config, "Upload failed", fmt.Sprintf("Could not create upload directory: %v", err), true)
	}

	saved := make([]string, 0, len(files))
	for _, header := range files {
		path, err := saveUploadFile(header, uploadDir, config.Fields)
		if err != nil {
			return renderStatus(req, config, "Upload failed", err.Error(), true)
		}
		saved = append(saved, path)
	}

	return renderSuccess(req, config, saved)
}

func applyDefaults(fields *Fields) {
	if strings.TrimSpace(fields.InputName) == "" {
		fields.InputName = "file"
	}
	if strings.TrimSpace(fields.Label) == "" {
		fields.Label = "Choose file"
	}
	if strings.TrimSpace(fields.Button) == "" {
		fields.Button = "Upload"
	}
	if fields.MaxMB <= 0 {
		fields.MaxMB = 20
	}
}

func renderForm(req *http.Request, config UploadPluginConfig) string {
	action := strings.TrimSpace(config.Fields.Action)
	if action == "" && req != nil && req.URL != nil {
		action = req.URL.Path
	}
	if action == "" {
		action = "/"
	}

	multipleAttr := ""
	if config.Fields.Multiple {
		multipleAttr = " multiple"
	}

	acceptAttr := ""
	if accept := resolveAccept(config.Fields); accept != "" {
		acceptAttr = fmt.Sprintf(` accept="%s"`, html.EscapeString(accept))
	}

	return fmt.Sprintf(
		`<form class="upload-plugin" method="post" action="%s" enctype="multipart/form-data">
  <label class="upload-plugin__label">%s</label>
  <input class="upload-plugin__input" type="file" name="%s"%s%s>
  <button class="upload-plugin__button" type="submit">%s</button>
</form>`,
		html.EscapeString(action),
		html.EscapeString(config.Fields.Label),
		html.EscapeString(config.Fields.InputName),
		multipleAttr,
		acceptAttr,
		html.EscapeString(config.Fields.Button),
	)
}

func renderSuccess(req *http.Request, config UploadPluginConfig, saved []string) string {
	var b strings.Builder
	b.WriteString(renderForm(req, config))
	b.WriteString(`<div class="upload-plugin__status upload-plugin__status--success"><strong>Upload successful.</strong><ul>`)
	for _, path := range saved {
		display := path
		if !config.Fields.ShowSavedDir {
			display = filepath.Base(path)
		}
		b.WriteString(`<li>`)
		b.WriteString(html.EscapeString(display))
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></div>`)
	return b.String()
}

func renderStatus(req *http.Request, config UploadPluginConfig, title string, message string, isError bool) string {
	statusClass := "upload-plugin__status--success"
	if isError {
		statusClass = "upload-plugin__status--error"
	}
	return fmt.Sprintf(
		`%s<div class="upload-plugin__status %s"><strong>%s</strong><div>%s</div></div>`,
		renderForm(req, config),
		statusClass,
		html.EscapeString(title),
		html.EscapeString(message),
	)
}

func resolveAccept(fields Fields) string {
	if len(fields.Accept) > 0 {
		return strings.Join(fields.Accept, ",")
	}

	values := make([]string, 0, len(fields.AllowedExts)+len(fields.AllowedTypes))
	for _, ext := range fields.AllowedExts {
		ext = normalizeExt(ext)
		if ext != "" {
			values = append(values, ext)
		}
	}
	for _, allowedType := range fields.AllowedTypes {
		allowedType = strings.TrimSpace(allowedType)
		if allowedType != "" {
			values = append(values, allowedType)
		}
	}
	return strings.Join(values, ",")
}

func resolveUploadDir(raw string) string {
	hbConfig := shared.GetHyperBricksConfiguration()
	resourcesDir := strings.TrimSpace(hbConfig.Directories["resources"])
	moduleRoot := ""
	if resourcesDir != "" {
		moduleRoot = filepath.Dir(resourcesDir)
	}

	if strings.TrimSpace(raw) == "" {
		if resourcesDir != "" {
			return filepath.Join(resourcesDir, "uploads")
		}
		if moduleRoot != "" {
			return filepath.Join(moduleRoot, "uploads")
		}
		return filepath.Join("uploads")
	}

	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	if moduleRoot != "" {
		return filepath.Join(moduleRoot, filepath.Clean(raw))
	}
	return filepath.Clean(raw)
}

func saveUploadFile(header *multipart.FileHeader, uploadDir string, fields Fields) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file %q: %w", header.Filename, err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !extensionAllowed(ext, fields.AllowedExts) {
		return "", fmt.Errorf("file type %q is not allowed", ext)
	}

	sniff := make([]byte, 512)
	n, err := file.Read(sniff)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read uploaded file %q: %w", header.Filename, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to rewind uploaded file %q: %w", header.Filename, err)
	}

	contentType := http.DetectContentType(sniff[:n])
	if !contentTypeAllowed(contentType, fields.AllowedTypes) {
		return "", fmt.Errorf("content type %q is not allowed", contentType)
	}

	name := uniqueFilename(header.Filename)
	destPath := filepath.Join(uploadDir, name)

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to create %q: %w", destPath, err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		return "", fmt.Errorf("failed to write %q: %w", destPath, err)
	}

	return destPath, nil
}

func extensionAllowed(ext string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	ext = normalizeExt(ext)
	for _, candidate := range allowed {
		if normalizeExt(candidate) == ext {
			return true
		}
	}
	return false
}

func contentTypeAllowed(contentType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" && candidate == contentType {
			return true
		}
	}
	return false
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(strings.ToLower(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}

func uniqueFilename(original string) string {
	ext := strings.ToLower(filepath.Ext(original))
	base := strings.TrimSuffix(filepath.Base(original), ext)
	base = sanitizeFileStem(base)
	if base == "" {
		base = "upload"
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}

func sanitizeFileStem(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func Plugin() (shared.PluginRenderer, error) {
	return &UploadPlugin{}, nil
}
