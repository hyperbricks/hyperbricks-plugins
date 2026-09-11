# Markdown WASM Plugin 1.0.0

The Markdown WASM plugin is a small WebAssembly renderer for the HyperBricks WASM plugin runtime. It supports a focused markdown subset: an initial `# Heading`, paragraphs, line breaks, and `**strong**` inline text.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/markdown-wasm",
  "source": "markdown_wasm_plugin.go",
  "runtime": "wasm",
  "binary": "MarkdownWasmPlugin",
  "version": "1.0.0",
  "compatible_hyperbricks": [
    ">=1.2.1-beta"
  ],
  "description": "WASM markdown renderer proof of concept for the HyperBricks WASM plugin runtime."
}
```

## Build And Enable

```bash
hyperbricks plugin build markdown-wasm@1.0.0
```

```yaml
hyperbricks:
  plugins:
    enabled:
      - MarkdownWasmPlugin@1.0.0
```

## Example HyperBricks YAML

```yaml
markdown_wasm_page:
  - type: hypermedia
  - route: markdown-wasm
  - title: Markdown WASM
  - content:
      - type: plugin
      - plugin: MarkdownWasmPlugin@1.0.0
      - data:
          class: markdown-wasm-content
          content: |
            # Hello from MarkdownWasmPlugin

            This paragraph was rendered by **WebAssembly**.
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `content` | no | Markdown text to render. |
| `class` | no | Wrapper class. Defaults to `markdown_plugin-content`. |
