# Lorem Ipsum Plugin 2.0.0

The lorem ipsum plugin generates placeholder paragraphs for layout prototypes, examples, and content demos. It renders generated text inside `<div class="lorem_ipsum_plugin-content">`.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/loremipsum",
  "source": "lorem_ipsum_plugin.go",
  "binary": "LoremIpsumPlugin",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.7.8-alpha"
  ],
  "description": "Generates placeholder lorem ipsum paragraphs for prototypes and demos."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - LoremIpsumPlugin@2.0.0
```

## Example HyperBricks YAML

```yaml
lorem_page:
  - type: hypermedia
  - route: lorem
  - title: Lorem ipsum demo
  - content:
      - type: plugin
      - plugin: LoremIpsumPlugin@2.0.0
      - data:
          paragraphs: 3
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `paragraphs` | no | Number of paragraphs to generate. |
