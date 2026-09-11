# MyPlugin 2.0.0

MyPlugin is the smallest native Go plugin example in this repository. It decodes `data.message` from a HyperBricks plugin component and renders it inside a simple HTML wrapper.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/myplugin",
  "source": "my_plugin.go",
  "binary": "MyPlugin",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.7.8-alpha"
  ],
  "description": "Minimal native Go plugin example for rendering configured data."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - MyPlugin@2.0.0
```

## Example HyperBricks YAML

```yaml
myplugin_page:
  - type: hypermedia
  - route: myplugin
  - title: MyPlugin demo
  - content:
      - type: plugin
      - plugin: MyPlugin@2.0.0
      - data:
          message: Hello from a native HyperBricks plugin.
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `message` | no | Text rendered inside `<div class="my_plugin-content">`. |
