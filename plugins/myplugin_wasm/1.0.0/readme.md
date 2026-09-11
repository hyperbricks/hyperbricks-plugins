# MyPlugin WASM 1.0.0

This is a compact Go/WASM plugin example for the HyperBricks WASM runtime. It renders a configurable card and demonstrates the v1 WASM ABI used by HyperBricks: `memory`, `alloc`, and `render`.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/myplugin_wasm",
  "source": "myplugin_wasm.go",
  "runtime": "wasm",
  "binary": "MyPluginWasmPlugin",
  "version": "1.0.0",
  "compatible_hyperbricks": [
    ">=1.2.1-beta"
  ],
  "description": "Basic Go/WASM plugin example for HyperBricks."
}
```

## Build And Enable

```bash
hyperbricks plugin build myplugin_wasm@1.0.0
```

```yaml
hyperbricks:
  plugins:
    enabled:
      - MyPluginWasmPlugin@1.0.0
```

## Example HyperBricks YAML

```yaml
wasm_card_page:
  - type: hypermedia
  - route: myplugin-wasm
  - title: MyPlugin WASM
  - content:
      - type: plugin
      - plugin: MyPluginWasmPlugin@1.0.0
      - data:
          eyebrow: Plugin example
          title: Hello WASM plugin
          message: This card was rendered by a Go/WASM plugin.
          cta_label: Read docs
          cta_href: /docs/plugins
          accent: '#2563eb'
          class: myplugin-wasm-card
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `title` | no | Card heading. |
| `message` | no | Body copy. |
| `eyebrow` | no | Small label above the heading. |
| `cta_label` | no | Link text. |
| `cta_href` | no | Safe link target: `http://`, `https://`, `mailto:`, `/`, or `#`. |
| `accent` | no | Basic CSS color value. |
| `class` | no | Wrapper class. Defaults to `myplugin-wasm-card`. |
