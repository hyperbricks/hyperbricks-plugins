# HyperBricks Plugins

This repository publishes versioned plugin source packages for HyperBricks. Each plugin lives under `plugins/<name>/<version>/` and includes a `manifest.json`, Go source, module files, and a plugin-specific `readme.md` with an example HyperBricks YAML component.

## Current Plugins

| Plugin | Version | Runtime | Config name |
| --- | --- | --- | --- |
| esbuild | 2.0.0 | native Go plugin | `EsbuildPlugin@2.0.0` |
| upload | 2.0.0 | native Go plugin | `Upload@2.0.0` |
| tailwindcss | 2.0.0 | native Go plugin | `TailwindCssPlugin@2.0.0` |
| myplugin | 2.0.0 | native Go plugin | `MyPlugin@2.0.0` |
| myplugin_wasm | 1.0.0 | WASM | `MyPluginWasmPlugin@1.0.0` |
| markdown | 2.0.0 | native Go plugin | `MarkdownPlugin@2.0.0` |
| markdown-wasm | 1.0.0 | WASM | `MarkdownWasmPlugin@1.0.0` |
| loremipsum | 2.0.0 | native Go plugin | `LoremIpsumPlugin@2.0.0` |

## Manifest Index

`plugins.index.json` is generated from every `manifest.json` file by `scripts/build_index.go`:

```bash
go run scripts/build_index.go
```

The index keeps plugin identity, source file, version, compatibility range, description, and optional runtime metadata such as `runtime: "wasm"` and explicit `binary` names.

## Using A Plugin

Build the plugin from a HyperBricks checkout or installed CLI, enable the generated config name in `package.hyperbricks.yaml`, then reference the same name in a `type: plugin` component.

```yaml
hyperbricks:
  plugins:
    enabled:
      - MarkdownPlugin@2.0.0
```

```yaml
markdown_page:
  - type: hypermedia
  - route: markdown
  - title: Markdown demo
  - content:
      - type: plugin
      - plugin: MarkdownPlugin@2.0.0
      - data:
          content: |
            # Welcome

            This is **Markdown** content rendered by a plugin.
```
