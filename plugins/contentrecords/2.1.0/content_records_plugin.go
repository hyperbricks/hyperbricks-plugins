package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperbricks/hyperbricks/pkg/shared"
	_ "github.com/mattn/go-sqlite3"
)

// FieldDef defines a single editable field mapping.
type FieldDef struct {
	Type  string `mapstructure:"type"`
	Bind  string `mapstructure:"bind"`
	Path  string `mapstructure:"path"`
	Label string `mapstructure:"label"`
	Order int    `mapstructure:"order"`
}

// Fields defines the plugin field schema.
type Fields struct {
	Template    interface{}         `mapstructure:"template"`
	Type        interface{}         `mapstructure:"type"`   // legacy alias
	View        string              `mapstructure:"view"`   // list|single
	Action      string              `mapstructure:"action"` // render|edit
	Mode        string              `mapstructure:"mode"`   // legacy alias
	Store       string              `mapstructure:"store"`
	Schema      map[string]FieldDef `mapstructure:"schema"` // CMS form schema
	Fields      map[string]FieldDef `mapstructure:"fields"` // legacy alias
	Query       string              `mapstructure:"query"`  // SQL or query string
	SQL         string              `mapstructure:"sql"`    // legacy alias
	ID          interface{}         `mapstructure:"id"`
	IDs         []interface{}       `mapstructure:"ids"`
	Teaser      bool                `mapstructure:"teaser"`
	Inline      bool                `mapstructure:"inline"`
	InlineParam string              `mapstructure:"inline_param"`
	Preview     *bool               `mapstructure:"preview"`
	Seed        bool                `mapstructure:"seed"`
	Editable    bool                `mapstructure:"editable"`
	EditRoute   string              `mapstructure:"edit_route"`
	ListRoute   string              `mapstructure:"list_route"`
	Route       string              `mapstructure:"route"` // legacy alias
	RecordParam string              `mapstructure:"record_param"`
	UploadDir   string              `mapstructure:"upload_dir"`
	Upload      string              `mapstructure:"upload"`
}

// ContentRecordsConfig is the component config for this plugin.
type ContentRecordsConfig struct {
	shared.Component `mapstructure:",squash"`
	PluginName       string `mapstructure:"plugin"`
	Fields           `mapstructure:"data"`
}

// ContentRecordsPlugin implements the Hyperbricks plugin renderer.
type ContentRecordsPlugin struct{}

// Ensure ContentRecordsPlugin implements shared.PluginRenderer.
var _ shared.PluginRenderer = (*ContentRecordsPlugin)(nil)

// Render is called by the renderer.
func (p *ContentRecordsPlugin) Render(instance interface{}, ctx context.Context) (any, []error) {
	var config ContentRecordsConfig
	var errors []error

	if err := shared.DecodeWithBasicHooks(instance, &config); err != nil {
		errors = append(errors, shared.ComponentError{
			Hash:     shared.GenerateHash(),
			Path:     config.HyperBricksPath,
			Key:      config.HyperBricksKey,
			Rejected: true,
			Err:      fmt.Sprintf("Decode error: %v", err),
		})
		return "<!-- content_records_plugin decode failed -->", errors
	}

	view, action := resolveViewAction(config.Fields)
	switch {
	case view == "list" && action == "edit":
		return renderListEdit(config.Fields, ctx, &errors), errors
	case view == "single" && action == "edit":
		return renderSingleEdit(config.Fields, ctx, &errors), errors
	case view == "list" && action == "render":
		return renderListRender(config.Fields, ctx, &errors), errors
	case view == "single" && action == "render":
		return renderSingleRender(config.Fields, ctx, &errors), errors
	default:
		return "<!-- content_records_plugin unknown view/action -->", errors
	}
}

// Plugin is exposed for the main application.
func Plugin() (shared.PluginRenderer, error) {
	return &ContentRecordsPlugin{}, nil
}

type bindTarget struct {
	Path string
}

type record struct {
	ID     int64
	Fields map[string]string
}

type cmsField struct {
	Name  string
	Label string
	Type  string
	Bind  string
	Path  string
	Order int
}

type inlineOptions struct {
	Enabled   bool
	BindTypes map[string]string
}

var (
	dbMu     sync.Mutex
	dbByPath = map[string]*dbEntry{}
)

type dbEntry struct {
	db      *sql.DB
	once    sync.Once
	initErr error
}

func resolveViewAction(fields Fields) (string, string) {
	view := strings.ToLower(strings.TrimSpace(fields.View))
	action := strings.ToLower(strings.TrimSpace(fields.Action))

	mode := strings.ToLower(strings.TrimSpace(fields.Mode))
	if view == "" && action == "" && mode != "" {
		switch mode {
		case "render":
			view = "list"
			action = "render"
		case "cms":
			view = "list"
			action = "edit"
		case "edit":
			view = "single"
			action = "edit"
		}
	}

	if view == "" {
		view = "list"
	}
	if action == "" {
		action = "render"
	}

	return view, action
}

func resolveTemplateValue(fields Fields) interface{} {
	if fields.Template != nil {
		return fields.Template
	}
	if fields.Type != nil {
		return fields.Type
	}
	return nil
}

func applyTeaserFilter(template map[string]interface{}, fields Fields) map[string]interface{} {
	if !fields.Teaser {
		return template
	}
	filtered, ok := filterTemplateByFlag(template, "@teaser")
	if !ok {
		return template
	}
	return filtered
}

func resolveSchema(fields Fields) map[string]FieldDef {
	if len(fields.Schema) > 0 {
		return fields.Schema
	}
	if len(fields.Fields) > 0 {
		return fields.Fields
	}
	return nil
}

func resolveQuery(fields Fields) string {
	query := strings.TrimSpace(fields.Query)
	if query != "" {
		return query
	}
	return strings.TrimSpace(fields.SQL)
}

func resolveEditRoute(fields Fields) string {
	route := strings.TrimSpace(fields.EditRoute)
	if route != "" {
		return route
	}
	return strings.TrimSpace(fields.Route)
}

func resolveListRoute(fields Fields) string {
	route := strings.TrimSpace(fields.ListRoute)
	if route != "" {
		return route
	}
	return resolveEditRoute(fields)
}

func resolveRecordParam(fields Fields) string {
	param := strings.TrimSpace(fields.RecordParam)
	if param == "" {
		param = "id"
	}
	return param
}

func resolveInlineParam(fields Fields) string {
	param := strings.TrimSpace(fields.InlineParam)
	if param == "" {
		param = "edit"
	}
	return param
}

func resolveShowPreview(fields Fields) bool {
	if fields.Preview == nil {
		return true
	}
	return *fields.Preview
}

func redirectIfPossible(ctx context.Context, target string) bool {
	if ctx == nil || strings.TrimSpace(target) == "" {
		return false
	}
	writer, _ := ctx.Value(shared.ResponseWriter).(http.ResponseWriter)
	req, _ := ctx.Value(shared.Request).(*http.Request)
	if writer == nil || req == nil {
		return false
	}
	http.Redirect(writer, req, target, http.StatusSeeOther)
	return true
}

func inlineMode(fields Fields, ctx context.Context) bool {
	if !fields.Inline || ctx == nil {
		return false
	}
	val := GetInputFromContext(ctx, resolveInlineParam(fields))
	return parseBoolFlag(val)
}

func parseBoolFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func parseBoolish(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return parseBoolFlag(v)
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func isBoundaryPlugin(node map[string]interface{}) bool {
	nodeType, _ := node["@type"].(string)
	if !strings.EqualFold(strings.TrimSpace(nodeType), "<PLUGIN>") {
		return false
	}

	pluginName, _ := node["plugin"].(string)
	if strings.Contains(strings.ToLower(pluginName), "contentrecords") {
		return true
	}

	dataRaw, ok := node["data"]
	if !ok {
		return false
	}
	switch data := dataRaw.(type) {
	case map[string]interface{}:
		return parseBoolish(data["content_record_boundary"]) || parseBoolish(data["boundary"])
	case map[interface{}]interface{}:
		normalized := normalizeInterfaceMap(data)
		return parseBoolish(normalized["content_record_boundary"]) || parseBoolish(normalized["boundary"])
	default:
		return false
	}
}

func hasBoundaryPlugin(node interface{}) bool {
	switch typed := node.(type) {
	case map[string]interface{}:
		if isBoundaryPlugin(typed) {
			return true
		}
		for key, child := range typed {
			if strings.HasPrefix(key, "@") {
				continue
			}
			if hasBoundaryPlugin(child) {
				return true
			}
		}
	case map[interface{}]interface{}:
		normalized := normalizeInterfaceMap(typed)
		return hasBoundaryPlugin(normalized)
	case []interface{}:
		for _, child := range typed {
			if hasBoundaryPlugin(child) {
				return true
			}
		}
	}
	return false
}

func resolveSingleRecordID(fields Fields, ctx context.Context) int64 {
	if id := parseIDValue(fields.ID); id != 0 {
		return id
	}
	if len(fields.IDs) > 0 {
		if id := parseIDValue(fields.IDs[0]); id != 0 {
			return id
		}
	}
	return resolveRecordID(ctx, fields.RecordParam)
}

func resolveIDs(fields Fields) []int64 {
	ids := make([]int64, 0, len(fields.IDs)+1)
	if id := parseIDValue(fields.ID); id != 0 {
		ids = append(ids, id)
	}
	for _, raw := range fields.IDs {
		if id := parseIDValue(raw); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseIDValue(value interface{}) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		return parseRecordID(v)
	default:
		return 0
	}
}

func renderListEdit(fields Fields, ctx context.Context, errors *[]error) any {
	template, binds, db, contentType, ok := loadTemplateAndDB(fields, errors)
	if !ok {
		return "<!-- content_records_plugin list edit failed -->"
	}

	fieldDefs := collectCMSFields(fields, binds)
	applyCMSAction(ctx, db, contentType, fieldDefs, resolveUploadDir(fields), errors)

	records, err := fetchRecordsForList(db, fields, contentType)
	if err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: fetch records failed: %w", err))
		return "<!-- content_records_plugin fetch records failed -->"
	}

	imageBinds := collectImageBinds(fields, binds)
	listBinds := collectFlaggedBindKeys(template, "@list")
	values := buildCMSValues(template, binds, records, fields, fieldDefs, imageBinds, listBinds)
	cms := map[string]interface{}{
		"@type":  "<TEMPLATE>",
		"inline": cmsInlineTemplate(),
		"values": values,
	}

	return map[string]interface{}{
		"@type": "<TREE>",
		"10":    cms,
	}
}

func renderListRender(fields Fields, ctx context.Context, errors *[]error) any {
	template, binds, db, contentType, ok := loadTemplateAndDB(fields, errors)
	if !ok {
		return "<!-- content_records_plugin list render failed -->"
	}

	if inlineMode(fields, ctx) {
		if handled, response := handleInlineUpdate(ctx, db, contentType, binds, fields, template, errors); handled {
			return response
		}
	}

	records, err := fetchRecordsForList(db, fields, contentType)
	if err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: fetch records failed: %w", err))
		return "<!-- content_records_plugin fetch records failed -->"
	}

	imageBinds := collectImageBinds(fields, binds)
	template = applyTeaserFilter(template, fields)
	inlineOpts := buildInlineOptions(fields, binds, inlineMode(fields, ctx))
	return buildList(template, binds, records, imageBinds, fields.Editable, resolveEditRoute(fields), resolveRecordParam(fields), inlineOpts)
}

func renderSingleEdit(fields Fields, ctx context.Context, errors *[]error) any {
	template, binds, db, contentType, ok := loadTemplateAndDB(fields, errors)
	if !ok {
		return "<!-- content_records_plugin single edit failed -->"
	}

	fieldDefs := collectCMSFields(fields, binds)
	recordID := resolveSingleRecordID(fields, ctx)
	actionApplied := false
	actionSuccess := false

	if req, _ := ctx.Value(shared.Request).(*http.Request); req != nil && req.Method == http.MethodPost {
		parseRequestForm(req, errors)
		action := strings.ToLower(strings.TrimSpace(GetInputFromContext(ctx, "action")))
		if action != "" {
			actionApplied = true
		}
		values := readFieldValuesFromContext(ctx, fieldDefs)
		mergeUploads(ctx, fieldDefs, resolveUploadDir(fields), values, errors)
		formID := parseRecordID(GetInputFromContext(ctx, "record_id"))
		if formID != 0 {
			recordID = formID
		}

		switch action {
		case "update":
			if recordID == 0 {
				if newID, err := createRecord(db, contentType, values); err == nil {
					recordID = newID
					actionSuccess = true
				} else {
					*errors = append(*errors, fmt.Errorf("content_records_plugin: create failed: %w", err))
				}
			} else {
				if err := updateRecord(db, recordID, contentType, values); err != nil {
					*errors = append(*errors, fmt.Errorf("content_records_plugin: update failed: %w", err))
				} else {
					actionSuccess = true
				}
			}
		case "delete":
			if recordID != 0 {
				if err := deleteRecord(db, recordID, contentType); err != nil {
					*errors = append(*errors, fmt.Errorf("content_records_plugin: delete failed: %w", err))
				} else {
					actionSuccess = true
				}
				recordID = 0
			}
		case "create":
			if newID, err := createRecord(db, contentType, values); err == nil {
				recordID = newID
				actionSuccess = true
			} else {
				*errors = append(*errors, fmt.Errorf("content_records_plugin: create failed: %w", err))
			}
		}
	}

	if actionApplied && actionSuccess {
		if redirectIfPossible(ctx, resolveListRoute(fields)) {
			return ""
		}
	}

	if recordID == 0 {
		if newID, err := createRecordFromTemplate(db, contentType, template, binds); err == nil {
			recordID = newID
		} else {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: create failed: %w", err))
		}
	}

	rec, err := fetchRecordByID(db, recordID, contentType)
	if err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: fetch record failed: %w", err))
		return "<!-- content_records_plugin fetch record failed -->"
	}

	imageBinds := collectImageBinds(fields, binds)
	showPreview := resolveShowPreview(fields)
	var preview map[string]interface{}
	if showPreview {
		preview = buildList(template, binds, []record{rec}, imageBinds, false, "", "", nil)
	}
	values := buildEditValues(rec, fieldDefs, fields, preview)
	edit := map[string]interface{}{
		"@type":  "<TEMPLATE>",
		"inline": editInlineTemplate(),
		"values": values,
	}

	return map[string]interface{}{
		"@type": "<TREE>",
		"10":    edit,
	}
}

func renderSingleRender(fields Fields, ctx context.Context, errors *[]error) any {
	template, binds, db, contentType, ok := loadTemplateAndDB(fields, errors)
	if !ok {
		return "<!-- content_records_plugin single render failed -->"
	}

	if inlineMode(fields, ctx) {
		if handled, response := handleInlineUpdate(ctx, db, contentType, binds, fields, template, errors); handled {
			return response
		}
	}

	recordID := resolveSingleRecordID(fields, ctx)
	if recordID == 0 {
		query := resolveQuery(fields)
		if query != "" {
			if ids, err := fetchRecordIDs(db, query, contentType); err == nil && len(ids) > 0 {
				recordID = ids[0]
			}
		}
	}
	if recordID == 0 {
		return "<!-- content_records_plugin no record id -->"
	}

	rec, err := fetchRecordByID(db, recordID, contentType)
	if err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: fetch record failed: %w", err))
		return "<!-- content_records_plugin fetch record failed -->"
	}

	template = applyTeaserFilter(template, fields)
	instance, ok := deepCopy(template).(map[string]interface{})
	if !ok {
		return "<!-- content_records_plugin render failed -->"
	}
	stripPluginMetaKeys(instance)
	imageBinds := collectImageBinds(fields, binds)
	for bindKey, value := range rec.Fields {
		target, ok := binds[bindKey]
		if !ok {
			continue
		}
		if value == "" {
			if _, isImage := imageBinds[bindKey]; isImage {
				continue
			}
		}
		_ = setAtPath(instance, target.Path, value)
	}
	inlineOpts := buildInlineOptions(fields, binds, inlineMode(fields, ctx))
	applyInlineAttributes(instance, binds, rec, inlineOpts)
	if fields.Editable && strings.TrimSpace(resolveEditRoute(fields)) != "" {
		addEditLink(instance, resolveEditRoute(fields), resolveRecordParam(fields), rec.ID)
	}
	return instance
}

func loadTemplateAndDB(fields Fields, errors *[]error) (map[string]interface{}, map[string]bindTarget, *sql.DB, string, bool) {
	templateValue := resolveTemplateValue(fields)
	template, ok := normalizeToStringMap(templateValue)
	if !ok {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: data.template must be a map"))
		return nil, nil, nil, "", false
	}

	binds := map[string]bindTarget{}
	collectBinds(template, "", binds)

	db, err := getDB(fields.Store)
	if err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: db open failed: %w", err))
		return nil, nil, nil, "", false
	}

	contentType := resolveTypeName(templateValue)
	if fields.Seed {
		if err := ensureSeed(db, template, binds, contentType); err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: seed failed: %w", err))
		}
	}

	return template, binds, db, contentType, true
}

func buildList(template map[string]interface{}, binds map[string]bindTarget, records []record, imageBinds map[string]struct{}, editable bool, route string, recordParam string, inline *inlineOptions) map[string]interface{} {
	list := map[string]interface{}{
		"@type": "<TREE>",
	}

	for i, rec := range records {
		instance, ok := deepCopy(template).(map[string]interface{})
		if !ok {
			continue
		}
		stripPluginMetaKeys(instance)
		for bindKey, value := range rec.Fields {
			target, ok := binds[bindKey]
			if !ok {
				continue
			}
			if value == "" {
				if _, isImage := imageBinds[bindKey]; isImage {
					continue
				}
			}
			_ = setAtPath(instance, target.Path, value)
		}

		if editable && strings.TrimSpace(route) != "" {
			addEditLink(instance, route, recordParam, rec.ID)
		}
		applyInlineAttributes(instance, binds, rec, inline)

		key := strconv.Itoa((i + 1) * 10)
		list[key] = instance
	}

	return list
}

func stripPluginMetaKeys(node interface{}) {
	switch typed := node.(type) {
	case map[string]interface{}:
		if isBoundaryPlugin(typed) {
			return
		}
		for key, child := range typed {
			if key == "@bind" || key == "@name" || key == "@list" || key == "@teaser" {
				delete(typed, key)
				continue
			}
			stripPluginMetaKeys(child)
		}
	case []interface{}:
		for _, child := range typed {
			stripPluginMetaKeys(child)
		}
	}
}

func buildInlineOptions(fields Fields, binds map[string]bindTarget, active bool) *inlineOptions {
	if !active {
		return nil
	}
	return &inlineOptions{
		Enabled:   true,
		BindTypes: buildBindTypeMap(fields, binds),
	}
}

func buildBindTypeMap(fields Fields, binds map[string]bindTarget) map[string]string {
	out := map[string]string{}
	schema := resolveSchema(fields)
	for name, def := range schema {
		bind := strings.TrimSpace(def.Bind)
		if bind == "" && strings.TrimSpace(def.Path) != "" {
			if resolved, ok := findBindByPath(binds, strings.TrimSpace(def.Path)); ok {
				bind = resolved
			}
		}
		if bind == "" {
			bind = name
		}
		if _, ok := binds[bind]; !ok {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(def.Type))
		if t == "" {
			t = "text"
		}
		out[bind] = t
	}
	return out
}

func applyInlineAttributes(instance map[string]interface{}, binds map[string]bindTarget, rec record, inline *inlineOptions) {
	if inline == nil || !inline.Enabled {
		return
	}
	for bindKey, target := range binds {
		nodePath, _ := splitPath(target.Path)
		node := findInlineNode(instance, nodePath)
		if node == nil {
			continue
		}
		if nodeType, ok := node["@type"].(string); ok && nodeType == "<TREE>" {
			continue
		}
		value := rec.Fields[bindKey]
		bindType := "text"
		if inline.BindTypes != nil {
			if t := strings.TrimSpace(inline.BindTypes[bindKey]); t != "" {
				bindType = strings.ToLower(t)
			}
		}
		applyInlineWrapper(node, bindKey, rec.ID, bindType, value)
	}
}

func applyInlineWrapper(node map[string]interface{}, bindKey string, recordID int64, bindType string, value string) {
	inlineWrapper := fmt.Sprintf(
		`<span class="cr-inline" data-cr-bind="%s" data-cr-id="%d" data-cr-type="%s" data-cr-value="%s">|</span>`,
		html.EscapeString(bindKey),
		recordID,
		html.EscapeString(bindType),
		html.EscapeString(value),
	)
	blockWrapper := fmt.Sprintf(
		`<div class="cr-inline cr-inline--block" data-cr-bind="%s" data-cr-id="%d" data-cr-type="%s" data-cr-value="%s">|</div>`,
		html.EscapeString(bindKey),
		recordID,
		html.EscapeString(bindType),
		html.EscapeString(value),
	)
	if existing, ok := node["enclose"].(string); ok && strings.TrimSpace(existing) != "" {
		if strings.Contains(existing, "data-cr-bind=") || strings.Contains(existing, "cr-inline") {
			return
		}
		node["enclose"] = strings.Replace(existing, "|", inlineWrapper, 1)
		return
	}
	node["enclose"] = blockWrapper
}

func splitPath(path string) (string, string) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1]
}

func getNodeAtPath(root map[string]interface{}, path string) map[string]interface{} {
	if path == "" {
		return root
	}
	node, ok := getAtPath(root, path)
	if !ok {
		return nil
	}
	typed, ok := node.(map[string]interface{})
	if !ok {
		return nil
	}
	return typed
}

func findInlineNode(root map[string]interface{}, path string) map[string]interface{} {
	current := strings.TrimSpace(path)
	for {
		node := getNodeAtPath(root, current)
		if node != nil {
			if nodeType, ok := node["@type"].(string); ok && strings.TrimSpace(nodeType) != "" {
				return node
			}
		}
		if current == "" {
			break
		}
		if idx := strings.LastIndex(current, "."); idx >= 0 {
			current = current[:idx]
		} else {
			current = ""
		}
	}
	return getNodeAtPath(root, path)
}

func collectFlaggedBindKeys(template map[string]interface{}, flagKey string) map[string]struct{} {
	out := map[string]struct{}{}
	collectFlaggedBindKeysInto(template, flagKey, out)
	return out
}

func collectFlaggedBindKeysInto(node interface{}, flagKey string, out map[string]struct{}) {
	switch typed := node.(type) {
	case map[string]interface{}:
		if isBoundaryPlugin(typed) {
			return
		}
		if hasFlag(typed, flagKey) {
			if bindValue, ok := typed["@bind"].(map[string]interface{}); ok {
				if field, ok := bindValue["field"].(string); ok {
					field = strings.TrimSpace(field)
					if field != "" {
						out[field] = struct{}{}
					}
				}
			}
		}
		for key, child := range typed {
			if strings.HasPrefix(key, "@") {
				continue
			}
			collectFlaggedBindKeysInto(child, flagKey, out)
		}
	case map[interface{}]interface{}:
		normalized := normalizeInterfaceMap(typed)
		collectFlaggedBindKeysInto(normalized, flagKey, out)
	case []interface{}:
		for _, child := range typed {
			collectFlaggedBindKeysInto(child, flagKey, out)
		}
	}
}

func hasFlag(node map[string]interface{}, flagKey string) bool {
	raw, ok := node[flagKey]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		val := strings.ToLower(strings.TrimSpace(v))
		return val != "" && val != "false" && val != "0"
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func filterTemplateByFlag(template map[string]interface{}, flagKey string) (map[string]interface{}, bool) {
	filtered, ok := filterNodeByFlag(template, flagKey)
	if !ok {
		return nil, false
	}
	result, ok := filtered.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return result, true
}

func filterNodeByFlag(node interface{}, flagKey string) (interface{}, bool) {
	switch typed := node.(type) {
	case map[string]interface{}:
		if hasFlag(typed, flagKey) {
			return deepCopy(typed), true
		}
		if isBoundaryPlugin(typed) {
			return nil, false
		}
		out := map[string]interface{}{}
		kept := false
		for key, child := range typed {
			if strings.HasPrefix(key, "@") {
				out[key] = child
				continue
			}
			filtered, ok := filterNodeByFlag(child, flagKey)
			if ok {
				out[key] = filtered
				kept = true
			}
		}
		if kept {
			for key, child := range typed {
				if strings.HasPrefix(key, "@") {
					continue
				}
				if _, exists := out[key]; exists {
					continue
				}
				switch child.(type) {
				case map[string]interface{}, map[interface{}]interface{}, []interface{}:
					continue
				default:
					out[key] = child
				}
			}
			return out, true
		}
		return nil, false
	case map[interface{}]interface{}:
		normalized := normalizeInterfaceMap(typed)
		return filterNodeByFlag(normalized, flagKey)
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, child := range typed {
			filtered, ok := filterNodeByFlag(child, flagKey)
			if ok {
				out = append(out, filtered)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func resolveListFieldIDs(fieldDefs []cmsField, listBinds map[string]struct{}) []interface{} {
	ids := make([]interface{}, 0, len(fieldDefs))
	if len(listBinds) == 0 {
		for _, field := range fieldDefs {
			bindKey := field.Bind
			if bindKey == "" {
				bindKey = field.Name
			}
			ids = append(ids, bindKey)
		}
		return ids
	}
	remaining := map[string]struct{}{}
	for key := range listBinds {
		remaining[key] = struct{}{}
	}
	for _, field := range fieldDefs {
		bindKey := field.Bind
		if bindKey == "" {
			bindKey = field.Name
		}
		if _, ok := listBinds[bindKey]; ok {
			ids = append(ids, bindKey)
			delete(remaining, bindKey)
		}
	}
	if len(remaining) > 0 {
		extra := make([]string, 0, len(remaining))
		for key := range remaining {
			extra = append(extra, key)
		}
		sort.Strings(extra)
		for _, key := range extra {
			ids = append(ids, key)
		}
	}
	return ids
}

func buildCMSValues(template map[string]interface{}, binds map[string]bindTarget, records []record, fields Fields, fieldDefs []cmsField, imageBinds map[string]struct{}, listBinds map[string]struct{}) map[string]interface{} {
	view, action := resolveViewAction(fields)
	recordIDs := make([]interface{}, 0, len(records))
	recordsMap := make(map[string]interface{}, len(records))
	for _, rec := range records {
		idStr := strconv.FormatInt(rec.ID, 10)
		recordIDs = append(recordIDs, idStr)

		fieldMap := make(map[string]interface{}, len(rec.Fields))
		for k, v := range rec.Fields {
			fieldMap[k] = v
		}

		recordsMap[idStr] = map[string]interface{}{
			"id":     idStr,
			"fields": fieldMap,
		}
	}

	fieldIDsList := make([]interface{}, 0, len(fieldDefs))
	fieldsMap := make(map[string]interface{}, len(fieldDefs))
	for _, field := range fieldDefs {
		bindKey := field.Bind
		if bindKey == "" {
			bindKey = field.Name
		}
		fieldIDsList = append(fieldIDsList, bindKey)
		fieldsMap[bindKey] = map[string]interface{}{
			"name":  field.Name,
			"label": field.Label,
			"type":  field.Type,
			"bind":  bindKey,
			"path":  field.Path,
		}
	}

	listFieldIDs := resolveListFieldIDs(fieldDefs, listBinds)
	for _, fieldID := range listFieldIDs {
		idStr, ok := fieldID.(string)
		if !ok {
			continue
		}
		if _, ok := fieldsMap[idStr]; ok {
			continue
		}
		fieldsMap[idStr] = map[string]interface{}{
			"name":  idStr,
			"label": idStr,
			"type":  "text",
			"bind":  idStr,
			"path":  "",
		}
	}

	showPreview := resolveShowPreview(fields)
	var preview map[string]interface{}
	if showPreview {
		preview = buildList(template, binds, records, imageBinds, false, "", "", nil)
	}

	return map[string]interface{}{
		"type":           resolveTypeName(template),
		"view":           view,
		"action":         action,
		"store":          fields.Store,
		"query":          resolveQuery(fields),
		"edit_route":     resolveEditRoute(fields),
		"record_param":   resolveRecordParam(fields),
		"records":        recordsMap,
		"record_ids":     recordIDs,
		"fields":         fieldsMap,
		"field_ids":      fieldIDsList,
		"list_field_ids": listFieldIDs,
		"show_preview":   showPreview,
		"preview":        preview,
	}
}

func buildEditValues(rec record, fieldDefs []cmsField, fields Fields, preview map[string]interface{}) map[string]interface{} {
	view, action := resolveViewAction(fields)
	fieldIDsList := make([]interface{}, 0, len(fieldDefs))
	fieldsMap := make(map[string]interface{}, len(fieldDefs))
	for _, field := range fieldDefs {
		bindKey := field.Bind
		if bindKey == "" {
			bindKey = field.Name
		}
		fieldIDsList = append(fieldIDsList, bindKey)
		fieldsMap[bindKey] = map[string]interface{}{
			"name":  field.Name,
			"label": field.Label,
			"type":  field.Type,
			"bind":  bindKey,
			"path":  field.Path,
		}
	}

	recordID := strconv.FormatInt(rec.ID, 10)
	showPreview := resolveShowPreview(fields)
	return map[string]interface{}{
		"type":         resolveTypeName(resolveTemplateValue(fields)),
		"view":         view,
		"action":       action,
		"store":        fields.Store,
		"record_id":    recordID,
		"show_preview": showPreview,
		"record": map[string]interface{}{
			"id":     recordID,
			"fields": mapStringToInterface(rec.Fields),
		},
		"fields":    fieldsMap,
		"field_ids": fieldIDsList,
		"preview":   preview,
	}
}

func mapStringToInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cmsInlineTemplate() string {
	return `
<div class="content-records-dashboard" data-theme="dark">
  <div class="shell">
    <header class="hero">
      <div class="hero-bar">
        <div class="badge">CMS</div>
      </div>
      <div class="hero-brand">
        <div class="hero-text">
          <h1>Content Records</h1>
          <p>Type: {{ .type }} · Store: {{ .store }}</p>
        </div>
        <div class="hero-actions">
          {{ if .edit_route }}
            <form method="get" action="{{ .edit_route }}">
              <button class="secondary" type="submit">New</button>
            </form>
          {{ else }}
            <span class="subhead">Set edit_route to enable New</span>
          {{ end }}
        </div>
      </div>
    </header>

    <div class="grid">
      <section class="panel">
        <div class="panel-header">
          <h2>Records</h2>
          {{ if .query }}
            <div class="mono">Query: {{ .query }}</div>
          {{ end }}
        </div>
        <div class="table-wrap">
          <table class="content-records-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Type</th>
                <th>Fields</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {{ range $i, $id := .record_ids }}
                {{ $rec := index $.records $id }}
                <tr>
                  <td data-label="ID">{{ $id }}</td>
                  <td data-label="Type">{{ $.type }}</td>
                  <td data-label="Fields">
                    <div class="content-records-fields">
                      {{ range $j, $fieldID := $.list_field_ids }}
                        {{ $def := index $.fields $fieldID }}
                        <div class="content-records-field-row">
                          <span class="content-records-field-label">{{ $def.label }}</span>
                          <span class="content-records-field-value">{{ index $rec.fields $fieldID }}</span>
                        </div>
                      {{ end }}
                    </div>
                  </td>
                  <td data-label="Actions">
                    <div class="row-actions">
                      {{ if $.edit_route }}
                        <form method="get" action="{{ $.edit_route }}">
                          <input type="hidden" name="{{ $.record_param }}" value="{{ $id }}">
                          <button class="secondary" type="submit">Edit</button>
                        </form>
                      {{ end }}
                      <form method="post">
                        <input type="hidden" name="action" value="delete">
                        <input type="hidden" name="record_id" value="{{ $id }}">
                        <button class="danger" type="submit">Delete</button>
                      </form>
                    </div>
                  </td>
                </tr>
              {{ else }}
                <tr>
                  <td class="content-records-empty" colspan="4">No records found.</td>
                </tr>
              {{ end }}
            </tbody>
          </table>
        </div>
      </section>

      {{ if .show_preview }}
      <section class="panel content-records-preview">
        <h2>Preview</h2>
        <div class="content-records-preview__body">
          {{ .preview }}
        </div>
      </section>
      {{ end }}
    </div>
  </div>
</div>
`
}

func editInlineTemplate() string {
	return `
<div class="content-records-dashboard" data-theme="dark">
  <div class="shell">
    <header class="hero">
      <div class="hero-bar">
        <div class="badge">CMS</div>
      </div>
      <div class="hero-brand">
        <div class="hero-text">
          <h1>Edit Record</h1>
          <p>Record: {{ .record_id }} · Type: {{ .type }}</p>
        </div>
      </div>
    </header>

    <div class="grid">
      <section class="panel">
        <div class="panel-header">
          <h2>Fields</h2>
        </div>
        <form method="post" enctype="multipart/form-data">
          <input type="hidden" name="record_id" value="{{ .record_id }}">
          {{ range $j, $fieldID := .field_ids }}
            {{ $def := index $.fields $fieldID }}
            <div class="field" data-field="{{ $fieldID }}">
              <label>{{ $def.label }}</label>
              {{ if eq $def.type "image" }}
                <div class="note-row">
                  <span class="mono">Current: {{ index $.record.fields $fieldID }}</span>
                </div>
                <input type="hidden" name="{{ $fieldID }}" value="{{ index $.record.fields $fieldID }}">
                <input type="file" name="{{ $fieldID }}">
              {{ else if eq $def.type "markdown" }}
                <textarea name="{{ $fieldID }}">{{ index $.record.fields $fieldID }}</textarea>
              {{ else }}
                <input type="text" name="{{ $fieldID }}" value="{{ index $.record.fields $fieldID }}">
              {{ end }}
            </div>
          {{ end }}

          <div class="content-records-actions">
            <div class="actions-row">
              <button type="submit" name="action" value="update">Save</button>
              <button class="secondary" type="submit" name="action" value="create">New</button>
              <button class="ghost" type="button" onclick="window.history.back()">Cancel</button>
            </div>
            <div class="actions-row">
              <button class="danger" type="submit" name="action" value="delete">Delete</button>
            </div>
          </div>
        </form>
      </section>

      {{ if .show_preview }}
      <section class="panel content-records-preview">
        <h2>Preview</h2>
        <div class="content-records-preview__body">
          {{ .preview }}
        </div>
      </section>
      {{ end }}
    </div>
  </div>
</div>
`
}

func collectCMSFields(fields Fields, binds map[string]bindTarget) []cmsField {
	var out []cmsField
	schema := resolveSchema(fields)
	if len(schema) > 0 {
		hasOrder := false
		for name, def := range schema {
			bind := strings.TrimSpace(def.Bind)
			if bind == "" && strings.TrimSpace(def.Path) != "" {
				if resolved, ok := findBindByPath(binds, strings.TrimSpace(def.Path)); ok {
					bind = resolved
				}
			}
			if bind == "" {
				bind = name
			}
			if _, ok := binds[bind]; !ok {
				continue
			}
			label := strings.TrimSpace(def.Label)
			if label == "" {
				label = bind
			}
			if def.Order != 0 {
				hasOrder = true
			}
			out = append(out, cmsField{
				Name:  name,
				Label: label,
				Type:  strings.TrimSpace(def.Type),
				Bind:  bind,
				Path:  def.Path,
				Order: def.Order,
			})
		}
		if hasOrder {
			sort.SliceStable(out, func(i, j int) bool {
				oi := out[i].Order
				oj := out[j].Order
				if oi == 0 {
					oi = 1 << 30
				}
				if oj == 0 {
					oj = 1 << 30
				}
				if oi == oj {
					return out[i].Name < out[j].Name
				}
				return oi < oj
			})
		} else {
			sort.SliceStable(out, func(i, j int) bool {
				return out[i].Name < out[j].Name
			})
		}
		return out
	}

	keys := make([]string, 0, len(binds))
	for key := range binds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, cmsField{
			Name:  key,
			Label: key,
			Type:  "text",
			Bind:  key,
			Path:  binds[key].Path,
		})
	}
	return out
}

func collectImageBinds(fields Fields, binds map[string]bindTarget) map[string]struct{} {
	imageBinds := map[string]struct{}{}
	schema := resolveSchema(fields)
	if len(schema) == 0 {
		return imageBinds
	}
	for name, def := range schema {
		if strings.ToLower(strings.TrimSpace(def.Type)) != "image" {
			continue
		}
		bind := strings.TrimSpace(def.Bind)
		if bind == "" && strings.TrimSpace(def.Path) != "" {
			if resolved, ok := findBindByPath(binds, strings.TrimSpace(def.Path)); ok {
				bind = resolved
			}
		}
		if bind == "" {
			bind = name
		}
		if _, ok := binds[bind]; ok {
			imageBinds[bind] = struct{}{}
		}
	}
	return imageBinds
}

func findBindByPath(binds map[string]bindTarget, path string) (string, bool) {
	for key, target := range binds {
		if target.Path == path {
			return key, true
		}
	}
	return "", false
}

func resolveRecordID(ctx context.Context, param string) int64 {
	key := strings.TrimSpace(param)
	if key == "" {
		key = "id"
	}
	return parseRecordID(GetInputFromContext(ctx, key))
}

func parseRecordID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func applyCMSAction(ctx context.Context, db *sql.DB, contentType string, fieldDefs []cmsField, uploadDir string, errors *[]error) {
	req, _ := ctx.Value(shared.Request).(*http.Request)
	if req == nil || req.Method != http.MethodPost {
		return
	}

	parseRequestForm(req, errors)
	action := strings.ToLower(strings.TrimSpace(GetInputFromContext(ctx, "action")))
	if action == "" {
		return
	}

	values := readFieldValuesFromContext(ctx, fieldDefs)
	mergeUploads(ctx, fieldDefs, uploadDir, values, errors)

	switch action {
	case "create":
		if _, err := createRecord(db, contentType, values); err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: create failed: %w", err))
		}
	case "update":
		idStr := GetInputFromContext(ctx, "record_id")
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: invalid record_id"))
			return
		}
		if err := updateRecord(db, id, contentType, values); err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: update failed: %w", err))
		}
	case "delete":
		idStr := GetInputFromContext(ctx, "record_id")
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: invalid record_id"))
			return
		}
		if err := deleteRecord(db, id, contentType); err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: delete failed: %w", err))
		}
	}
}

func readFieldValuesFromContext(ctx context.Context, fieldDefs []cmsField) map[string]string {
	values := make(map[string]string, len(fieldDefs))
	for _, def := range fieldDefs {
		key := def.Bind
		if key == "" {
			key = def.Name
		}
		values[key] = GetInputFromContext(ctx, key)
	}
	return values
}

func resolveUploadDir(fields Fields) string {
	if strings.TrimSpace(fields.UploadDir) != "" {
		return strings.TrimSpace(fields.UploadDir)
	}
	if strings.TrimSpace(fields.Upload) != "" {
		return strings.TrimSpace(fields.Upload)
	}
	return ""
}

func mergeUploads(ctx context.Context, fieldDefs []cmsField, uploadDir string, values map[string]string, errors *[]error) {
	uploadDir = strings.TrimSpace(uploadDir)
	if uploadDir == "" {
		return
	}

	req, _ := ctx.Value(shared.Request).(*http.Request)
	if req == nil {
		return
	}

	contentType := req.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return
	}

	if err := req.ParseMultipartForm(20 << 20); err != nil {
		*errors = append(*errors, fmt.Errorf("content_records_plugin: upload parse failed: %w", err))
		return
	}

	for _, def := range fieldDefs {
		if strings.ToLower(strings.TrimSpace(def.Type)) != "image" {
			continue
		}
		key := def.Bind
		if key == "" {
			key = def.Name
		}
		path, err := saveUploadFile(req, key, uploadDir)
		if err != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: upload failed: %w", err))
			continue
		}
		if path != "" {
			values[key] = path
		}
	}
}

type inlineUpdatePayload struct {
	Inline   string
	RecordID string
	Bind     string
	Value    string
}

func handleInlineUpdate(ctx context.Context, db *sql.DB, contentType string, binds map[string]bindTarget, fields Fields, template map[string]interface{}, errors *[]error) (bool, any) {
	req, _ := ctx.Value(shared.Request).(*http.Request)
	if req == nil || req.Method != http.MethodPost {
		return false, nil
	}

	payload, err := parseInlinePayload(req, ctx, fields, errors)
	if err != nil {
		if errors != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: inline parse failed: %w", err))
		}
		return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid inline payload",
		})
	}

	if !parseBoolFlag(payload.Inline) {
		return false, nil
	}

	bindKey := strings.TrimSpace(payload.Bind)
	if bindKey == "" {
		return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
			"error": "bind is required",
		})
	}
	if _, ok := binds[bindKey]; !ok {
		// Allow nested ContentRecords plugins to handle binds outside this template.
		if hasBoundaryPlugin(template) {
			return false, nil
		}
		return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
			"error": "unknown bind",
		})
	}

	recordID := parseRecordID(strings.TrimSpace(payload.RecordID))
	if recordID == 0 {
		recordID = resolveRecordID(ctx, resolveRecordParam(fields))
	}
	if recordID == 0 {
		return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
			"error": "record_id is required",
		})
	}

	value := payload.Value
	contentTypeHeader := req.Header.Get("Content-Type")
	if strings.HasPrefix(contentTypeHeader, "multipart/form-data") {
		uploadDir := resolveUploadDir(fields)
		if uploadDir == "" && hasFileUpload(req, "file", bindKey) {
			return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
				"error": "upload_dir is required for file uploads",
			})
		}
		if uploadDir != "" {
			path, err := saveUploadFile(req, "file", uploadDir)
			if err != nil {
				if errors != nil {
					*errors = append(*errors, fmt.Errorf("content_records_plugin: upload failed: %w", err))
				}
				return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
					"error": "upload failed",
				})
			}
			if path == "" {
				path, err = saveUploadFile(req, bindKey, uploadDir)
				if err != nil {
					if errors != nil {
						*errors = append(*errors, fmt.Errorf("content_records_plugin: upload failed: %w", err))
					}
					return true, writeInlineJSON(ctx, http.StatusBadRequest, map[string]interface{}{
						"error": "upload failed",
					})
				}
			}
			if path != "" {
				value = path
			}
		}
	}

	if err := updateRecordField(db, recordID, contentType, bindKey, value); err != nil {
		if errors != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: inline update failed: %w", err))
		}
		return true, writeInlineJSON(ctx, http.StatusInternalServerError, map[string]interface{}{
			"error": "update failed",
		})
	}

	return true, writeInlineJSON(ctx, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"record_id": strconv.FormatInt(recordID, 10),
		"bind":      bindKey,
		"value":     value,
	})
}

func parseInlinePayload(req *http.Request, ctx context.Context, fields Fields, errors *[]error) (inlineUpdatePayload, error) {
	contentType := req.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		payload, err := decodeInlineJSON(req)
		if err != nil {
			return inlineUpdatePayload{}, err
		}
		return inlineUpdatePayload{
			Inline:   firstNonEmpty(getStringFromAny(payload["cr_inline"]), getStringFromAny(payload["inline"])),
			RecordID: firstNonEmpty(getStringFromAny(payload["record_id"]), getStringFromAny(payload["id"])),
			Bind:     firstNonEmpty(getStringFromAny(payload["bind"]), getStringFromAny(payload["field"])),
			Value:    getStringFromAny(payload["value"]),
		}, nil
	}

	parseRequestForm(req, errors)
	return inlineUpdatePayload{
		Inline:   firstNonEmpty(GetInputFromContext(ctx, "cr_inline"), GetInputFromContext(ctx, "inline")),
		RecordID: firstNonEmpty(GetInputFromContext(ctx, "record_id"), GetInputFromContext(ctx, resolveRecordParam(fields))),
		Bind:     firstNonEmpty(GetInputFromContext(ctx, "bind"), GetInputFromContext(ctx, "field")),
		Value:    GetInputFromContext(ctx, "value"),
	}, nil
}

// decodeInlineJSON reads the JSON payload while restoring req.Body so nested
// ContentRecords plugins can parse the same request.
func decodeInlineJSON(req *http.Request) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if req == nil || req.Body == nil {
		return payload, nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return payload, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		if err == io.EOF {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	return payload, nil
}

func getStringFromAny(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func hasFileUpload(req *http.Request, fieldNames ...string) bool {
	if req == nil {
		return false
	}
	if req.MultipartForm == nil {
		return false
	}
	for _, name := range fieldNames {
		if files := req.MultipartForm.File[name]; len(files) > 0 {
			return true
		}
	}
	return false
}

func writeInlineJSON(ctx context.Context, status int, payload map[string]interface{}) string {
	writer, _ := ctx.Value(shared.ResponseWriter).(http.ResponseWriter)
	if writer != nil {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.WriteHeader(status)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"error":"inline response failed"}`
	}
	return string(data)
}

func saveUploadFile(req *http.Request, fieldName, uploadDir string) (string, error) {
	file, header, err := req.FormFile(fieldName)
	if err != nil {
		if err == http.ErrMissingFile {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", err
	}

	ext := filepath.Ext(header.Filename)
	base := strings.TrimSuffix(filepath.Base(header.Filename), ext)
	base = sanitizeFilename(base)
	if base == "" {
		base = "upload"
	}
	filename := fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
	destPath := filepath.Join(uploadDir, filename)

	dest, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		return "", err
	}

	return destPath, nil
}

func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func createRecord(db *sql.DB, contentType string, values map[string]string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.Exec(`INSERT INTO records(type) VALUES(?)`, contentType)
	if err != nil {
		return 0, err
	}
	recordID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := upsertFields(tx, recordID, values); err != nil {
		return 0, err
	}

	return recordID, tx.Commit()
}

func createRecordFromTemplate(db *sql.DB, contentType string, template map[string]interface{}, binds map[string]bindTarget) (int64, error) {
	values := defaultValuesFromTemplate(template, binds)
	return createRecord(db, contentType, values)
}

func updateRecord(db *sql.DB, recordID int64, contentType string, values map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if contentType != "" {
		if _, err := tx.Exec(`UPDATE records SET updated_at = CURRENT_TIMESTAMP WHERE id = ? AND type = ?`, recordID, contentType); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE records SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, recordID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM record_fields WHERE record_id = ?`, recordID); err != nil {
		return err
	}

	if err := upsertFields(tx, recordID, values); err != nil {
		return err
	}

	return tx.Commit()
}

func updateRecordField(db *sql.DB, recordID int64, contentType string, bindKey string, value string) error {
	if recordID == 0 {
		return fmt.Errorf("record id is required")
	}
	if strings.TrimSpace(bindKey) == "" {
		return fmt.Errorf("bind key is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var res sql.Result
	if contentType != "" {
		res, err = tx.Exec(`UPDATE records SET updated_at = CURRENT_TIMESTAMP WHERE id = ? AND type = ?`, recordID, contentType)
	} else {
		res, err = tx.Exec(`UPDATE records SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, recordID)
	}
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("record not found")
	}

	res, err = tx.Exec(`UPDATE record_fields SET value = ? WHERE record_id = ? AND bind_key = ?`, value, recordID, bindKey)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		if _, err := tx.Exec(`INSERT INTO record_fields(record_id, bind_key, value) VALUES(?, ?, ?)`, recordID, bindKey, value); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func deleteRecord(db *sql.DB, recordID int64, contentType string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`DELETE FROM record_fields WHERE record_id = ?`, recordID); err != nil {
		return err
	}
	if contentType != "" {
		if _, err := tx.Exec(`DELETE FROM records WHERE id = ? AND type = ?`, recordID, contentType); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`DELETE FROM records WHERE id = ?`, recordID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertFields(tx *sql.Tx, recordID int64, values map[string]string) error {
	for key, value := range values {
		if _, err := tx.Exec(
			`INSERT INTO record_fields(record_id, bind_key, value) VALUES(?, ?, ?)`,
			recordID, key, value,
		); err != nil {
			return err
		}
	}
	return nil
}

// Fetch value for key from context (form, query, body)
func GetInputFromContext(ctx context.Context, key string) string {
	if form, ok := ctx.Value(shared.FormData).(url.Values); ok && form != nil {
		if v, exists := form[key]; exists && len(v) > 0 {
			return v[0]
		}
	}
	if req, ok := ctx.Value(shared.Request).(*http.Request); ok && req != nil {
		if v := req.Form.Get(key); v != "" {
			return v
		}
		if v := req.PostForm.Get(key); v != "" {
			return v
		}
		vals := req.URL.Query()
		if v, exists := vals[key]; exists && len(v) > 0 {
			return v[0]
		}
	}
	if body, ok := ctx.Value(shared.RequestBody).(io.Reader); ok && body != nil {
		var m map[string]interface{}
		decoder := json.NewDecoder(body)
		if err := decoder.Decode(&m); err == nil {
			if v, found := m[key]; found {
				if str, ok := v.(string); ok {
					return str
				}
			}
		}
	}
	return ""
}

func parseRequestForm(req *http.Request, errors *[]error) {
	if req == nil {
		return
	}
	contentType := req.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if req.MultipartForm != nil {
			return
		}
		if err := req.ParseMultipartForm(20 << 20); err != nil {
			if errors != nil {
				*errors = append(*errors, fmt.Errorf("content_records_plugin: parse multipart form failed: %w", err))
			}
		}
		return
	}
	if err := req.ParseForm(); err != nil {
		if errors != nil {
			*errors = append(*errors, fmt.Errorf("content_records_plugin: parse form failed: %w", err))
		}
	}
}

func resolveTypeName(value interface{}) string {
	if value == nil {
		return ""
	}
	if m, ok := value.(map[string]interface{}); ok {
		if t, ok := m["@name"].(string); ok {
			return t
		}
	}
	return ""
}

func getDB(store string) (*sql.DB, error) {
	store = strings.TrimSpace(store)
	if store == "" {
		return nil, fmt.Errorf("content_records_plugin: data.store is required")
	}

	if store != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
			return nil, err
		}
	}

	dbMu.Lock()
	entry, ok := dbByPath[store]
	if !ok {
		db, err := sql.Open("sqlite3", store)
		if err != nil {
			dbMu.Unlock()
			return nil, err
		}
		entry = &dbEntry{db: db}
		dbByPath[store] = entry
	}
	dbMu.Unlock()

	if err := entry.db.Ping(); err != nil {
		return nil, err
	}

	entry.once.Do(func() {
		entry.initErr = initSchema(entry.db)
	})

	if entry.initErr != nil {
		return entry.db, entry.initErr
	}

	return entry.db, nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS record_fields (
			record_id INTEGER,
			bind_key TEXT,
			value TEXT,
			PRIMARY KEY(record_id, bind_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_records_type ON records(type)`,
		`CREATE INDEX IF NOT EXISTS idx_record_fields_record_id ON record_fields(record_id)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func ensureSeed(db *sql.DB, template map[string]interface{}, binds map[string]bindTarget, contentType string) error {
	count, err := countRecords(db, contentType)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = createRecordFromTemplate(db, contentType, template, binds)
	return err
}

func countRecords(db *sql.DB, contentType string) (int, error) {
	if strings.TrimSpace(contentType) == "" {
		var total int
		err := db.QueryRow(`SELECT COUNT(*) FROM records`).Scan(&total)
		return total, err
	}
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM records WHERE type = ?`, contentType).Scan(&total)
	return total, err
}

func fetchRecordsForList(db *sql.DB, fields Fields, contentType string) ([]record, error) {
	ids := resolveIDs(fields)
	if len(ids) > 0 {
		return fetchRecordsByIDs(db, ids, contentType)
	}
	query := resolveQuery(fields)
	return fetchRecords(db, query, contentType)
}

func fetchRecordsByIDs(db *sql.DB, ids []int64, contentType string) ([]record, error) {
	records := make([]record, 0, len(ids))
	for _, id := range ids {
		rec, err := fetchRecordByID(db, id, contentType)
		if err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

func fetchRecords(db *sql.DB, sqlQuery string, contentType string) ([]record, error) {
	ids, err := fetchRecordIDs(db, sqlQuery, contentType)
	if err != nil {
		return nil, err
	}

	records := make([]record, 0, len(ids))
	for _, id := range ids {
		fields, err := fetchRecordFields(db, id)
		if err != nil {
			return nil, err
		}
		records = append(records, record{
			ID:     id,
			Fields: fields,
		})
	}

	return records, nil
}

func fetchRecordIDs(db *sql.DB, sqlQuery string, contentType string) ([]int64, error) {
	if strings.TrimSpace(sqlQuery) == "" {
		if strings.TrimSpace(contentType) == "" {
			rows, err := db.Query(`SELECT id FROM records ORDER BY id`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			return scanIDs(rows)
		}

		rows, err := db.Query(`SELECT id FROM records WHERE type = ? ORDER BY id`, contentType)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanIDs(rows)
	}

	rows, err := db.Query(sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]int64, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, nil
	}

	var ids []int64
	for rows.Next() {
		values := make([]interface{}, len(cols))
		holder := make([]interface{}, len(cols))
		for i := range values {
			holder[i] = &values[i]
		}
		if err := rows.Scan(holder...); err != nil {
			return nil, err
		}
		id, ok := toInt64(values[0])
		if !ok {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func toInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case []byte:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func defaultValuesFromTemplate(template map[string]interface{}, binds map[string]bindTarget) map[string]string {
	values := make(map[string]string, len(binds))
	for bindKey, target := range binds {
		val := ""
		if v, ok := getAtPath(template, target.Path); ok {
			val = fmt.Sprintf("%v", v)
		}
		values[bindKey] = val
	}
	return values
}

func fetchRecordFields(db *sql.DB, recordID int64) (map[string]string, error) {
	rows, err := db.Query(`SELECT bind_key, value FROM record_fields WHERE record_id = ?`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := map[string]string{}
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if _, exists := fields[key]; exists {
			continue
		}
		fields[key] = value
	}
	return fields, rows.Err()
}

func fetchRecordByID(db *sql.DB, recordID int64, contentType string) (record, error) {
	if recordID == 0 {
		return record{}, fmt.Errorf("record id is required")
	}
	var id int64
	var err error
	if contentType != "" {
		err = db.QueryRow(`SELECT id FROM records WHERE id = ? AND type = ?`, recordID, contentType).Scan(&id)
	} else {
		err = db.QueryRow(`SELECT id FROM records WHERE id = ?`, recordID).Scan(&id)
	}
	if err != nil {
		return record{}, err
	}
	fields, err := fetchRecordFields(db, id)
	if err != nil {
		return record{}, err
	}
	return record{ID: id, Fields: fields}, nil
}

func collectBinds(node map[string]interface{}, path string, binds map[string]bindTarget) {
	if bindValue, ok := node["@bind"]; ok {
		bindMap, ok := bindValue.(map[string]interface{})
		if ok {
			field, _ := bindMap["field"].(string)
			bindPath, _ := bindMap["path"].(string)
			field = strings.TrimSpace(field)
			bindPath = strings.TrimSpace(bindPath)
			if field != "" && bindPath != "" {
				if _, exists := binds[field]; !exists {
					fullPath := bindPath
					if path != "" {
						fullPath = path + "." + bindPath
					}
					binds[field] = bindTarget{Path: fullPath}
				}
			}
		}
	}

	if isBoundaryPlugin(node) {
		return
	}

	for key, value := range node {
		if strings.HasPrefix(key, "@") {
			continue
		}
		child, ok := value.(map[string]interface{})
		if ok {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			collectBinds(child, childPath, binds)
		} else if childMap, ok := value.(map[interface{}]interface{}); ok {
			normalized := normalizeInterfaceMap(childMap)
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			collectBinds(normalized, childPath, binds)
		}
	}
}

func normalizeToStringMap(value interface{}) (map[string]interface{}, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		return v, true
	case map[interface{}]interface{}:
		return normalizeInterfaceMap(v), true
	default:
		return nil, false
	}
}

func normalizeInterfaceMap(value map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(value))
	for k, v := range value {
		key, ok := k.(string)
		if !ok {
			key = fmt.Sprintf("%v", k)
		}
		switch child := v.(type) {
		case map[string]interface{}:
			result[key] = child
		case map[interface{}]interface{}:
			result[key] = normalizeInterfaceMap(child)
		case []interface{}:
			result[key] = normalizeSlice(child)
		default:
			result[key] = v
		}
	}
	return result
}

func normalizeSlice(values []interface{}) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		switch child := v.(type) {
		case map[string]interface{}:
			out[i] = child
		case map[interface{}]interface{}:
			out[i] = normalizeInterfaceMap(child)
		case []interface{}:
			out[i] = normalizeSlice(child)
		default:
			out[i] = v
		}
	}
	return out
}

func deepCopy(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		copied := make(map[string]interface{}, len(v))
		for key, child := range v {
			copied[key] = deepCopy(child)
		}
		return copied
	case map[interface{}]interface{}:
		copied := make(map[interface{}]interface{}, len(v))
		for key, child := range v {
			copied[key] = deepCopy(child)
		}
		return copied
	case []interface{}:
		copied := make([]interface{}, len(v))
		for i, child := range v {
			copied[i] = deepCopy(child)
		}
		return copied
	default:
		return v
	}
}

func setAtPath(root map[string]interface{}, path string, value interface{}) bool {
	parts := strings.Split(path, ".")
	var current interface{} = root
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		isLast := i == len(parts)-1
		switch node := current.(type) {
		case map[string]interface{}:
			if isLast {
				node[part] = value
				return true
			}
			next, ok := node[part]
			if !ok {
				return false
			}
			current = next
		case map[interface{}]interface{}:
			if isLast {
				node[part] = value
				return true
			}
			next, ok := node[part]
			if !ok {
				return false
			}
			current = next
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return false
			}
			if isLast {
				node[idx] = value
				return true
			}
			current = node[idx]
		default:
			return false
		}
	}
	return false
}

func getAtPath(root map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch node := current.(type) {
		case map[string]interface{}:
			next, ok := node[part]
			if !ok {
				return nil, false
			}
			current = next
		case map[interface{}]interface{}:
			next, ok := node[part]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			current = node[idx]
		default:
			return nil, false
		}
	}
	return current, true
}

func addEditLink(instance map[string]interface{}, route string, recordParam string, id int64) {
	route = strings.TrimSpace(route)
	if route == "" {
		return
	}
	if _, exists := instance["_edit"]; exists {
		return
	}

	href := route
	param := strings.TrimSpace(recordParam)
	if param == "" {
		param = "id"
	}
	if strings.Contains(route, "?") {
		href = fmt.Sprintf("%s&%s=%d", route, param, id)
	} else {
		href = fmt.Sprintf("%s?%s=%d", route, param, id)
	}

	instance["_edit"] = map[string]interface{}{
		"@type": "<HTML>",
		"value": fmt.Sprintf(`<a class="content-records-edit-link" href="%s">Edit</a>`, html.EscapeString(href)),
	}
}
