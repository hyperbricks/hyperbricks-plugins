# Markdown Plugin 2.0.0

The markdown plugin renders configured Markdown content to HTML using Blackfriday and wraps the result in a configurable class. Use it for trusted, configured content rather than arbitrary unreviewed user input.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/markdown",
  "source": "markdown_plugin.go",
  "binary": "MarkdownPlugin",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.7.8-alpha"
  ],
  "description": "Renders Markdown content to HTML with an optional wrapper class."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - MarkdownPlugin@2.0.0
```

## Example HyperBricks YAML

```yaml
markdown_page:
  - type: hypermedia
  - route: markdown
  - title: Markdown demo
  - content:
      - type: plugin
      - plugin: MarkdownPlugin@2.0.0
      - data:
          class: markdown-content
          content: |
            # Welcome

            This is **Markdown** content rendered by a plugin.
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `content` | no | Markdown text to render. |
| `class` | no | Wrapper class. Defaults to `markdown_plugin-content`. |
