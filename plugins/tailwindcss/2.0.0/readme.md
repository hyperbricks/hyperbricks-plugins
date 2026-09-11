# Tailwind CSS Plugin 2.0.0

The Tailwind CSS plugin runs the Tailwind standalone CLI to compile an input CSS file into the module's static assets. It is designed for Tailwind v4 style input files and can optionally minify and cache the rendered include result.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/tailwindcss",
  "source": "tailwindcss_plugin.go",
  "binary": "TailwindCssPlugin",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.7.8-alpha"
  ],
  "description": "Builds Tailwind CSS with the standalone Tailwind CLI and optional caching."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - TailwindCssPlugin@2.0.0
```

## Example HyperBricks YAML

Create `resources/css/app.css` with your Tailwind imports, then compile it into `static/css/app.css`:

```yaml
styles:
  - type: plugin
  - plugin: TailwindCssPlugin@2.0.0
  - data:
      input_css:
        path: {base: resources, path: css/app.css}
      output_css:
        path: {base: static, path: css/app.css}
      binary: tailwindcss
      minify: true
      signal: false
      debug: false
      cache: true
      enclose: '<link rel="stylesheet" href="|">'

page:
  - type: hypermedia
  - route: index
  - title: Tailwind demo
  - head:
      - inherit: styles
  - content:
      - type: html
      - value: '<main class="mx-auto max-w-3xl p-8">Tailwind is compiled.</main>'
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `input_css` | yes | Input CSS file passed to `tailwindcss -i`. |
| `output_css` | yes | Output CSS file passed to `tailwindcss -o`. |
| `config` | no | Optional config path; Tailwind v4 projects often use `@config` in CSS instead. |
| `binary` | no | Tailwind CLI binary. Defaults to `tailwindcss`. |
| `signal` | no | Runs a quick CLI availability check. |
| `enclose` | no | Wrapper for the generated CSS URL, or `<style>{{css}}</style>` for inline CSS. |
| `minify` | no | Adds `--minify`. |
| `debug` | no | Streams CLI output to the HyperBricks logger. |
| `cache` | no | Caches the rendered include result in memory. |
