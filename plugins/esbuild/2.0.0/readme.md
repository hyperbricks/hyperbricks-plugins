# Esbuild Plugin 2.0.0

The esbuild plugin bundles JavaScript, TypeScript, and CSS from a module's `resources` directory into its `static` directory. It can use the embedded Go esbuild API by default, or an explicit esbuild CLI binary when `data.binary` is configured.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/esbuild",
  "source": "esbuild_plugin.go",
  "binary": "EsbuildPlugin",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.7.8-alpha"
  ],
  "description": "Bundles JavaScript, TypeScript, and CSS assets with esbuild and optional caching."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - EsbuildPlugin@2.0.0
```

## Example HyperBricks YAML

Create `resources/js/app.js` and render the component inside a page head or body:

```yaml
browser_script:
  - type: plugin
  - plugin: EsbuildPlugin@2.0.0
  - data:
      entry: js/app.js
      outfile: js/app.bundle.js
      minify: true
      minify_identifiers: true
      sourcemap: false
      cache: true
      enclose: '<script src="|" defer></script>'

page:
  - type: hypermedia
  - route: index
  - title: Esbuild demo
  - head:
      - inherit: browser_script
  - content:
      - type: html
      - value: '<main id="app">Bundled by esbuild.</main>'
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `entry` | yes | Path below the module `resources` directory. |
| `outfile` | yes | Output path below the module `static` directory. |
| `binary` | no | Optional esbuild CLI path. Empty uses the Go API. |
| `enclose` | no | Wrapper where `|` is replaced by the generated static URL. |
| `minify` | no | Minifies whitespace and syntax. |
| `minify_identifiers` | no | Minifies identifiers. |
| `mangle` | no | Enables property mangling. Use carefully. |
| `sourcemap` | no | Writes a linked source map. |
| `debug` | no | Logs build options and details. |
| `cache` | no | Caches the rendered include string in memory. |
