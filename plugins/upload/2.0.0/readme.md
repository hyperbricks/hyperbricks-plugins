# Upload Plugin 2.0.0

The upload plugin renders a multipart upload form on `GET` and stores uploaded files on `POST`. It is intended for trusted modules where the runtime host may write to the configured upload directory.

## Manifest

```json
{
  "plugin": "github.com/hyperbricks/plugins/upload",
  "source": "upload_plugin.go",
  "binary": "Upload",
  "version": "2.0.0",
  "compatible_hyperbricks": [
    ">=0.8.0-alpha"
  ],
  "description": "Renders multipart upload forms and stores validated uploads on the local filesystem."
}
```

## Enable The Plugin

```yaml
hyperbricks:
  plugins:
    enabled:
      - Upload@2.0.0
```

## Example HyperBricks YAML

Use a route owner with `nocache: true` so form state and upload responses are not served from the rendered-output cache.

```yaml
upload_page:
  - type: hypermedia
  - route: upload
  - title: Upload
  - nocache: true
  - content:
      - type: plugin
      - plugin: Upload@2.0.0
      - data:
          label: Choose a file
          button: Upload
          upload_dir: resources/uploads
          input_name: file
          allowed_exts: [jpg, png, pdf]
          max_mb: 20
          multiple: false
```

## Data Fields

| Field | Required | Description |
| --- | ---: | --- |
| `upload_dir` | no | Destination directory. Defaults to `resources/uploads`. |
| `input_name` | no | Multipart field name. Defaults to `file`. |
| `label` | no | Label text. Defaults to `Choose file`. |
| `button` | no | Submit button text. Defaults to `Upload`. |
| `action` | no | Form action. Defaults to the current request path. |
| `accept` | no | Raw browser accept values. |
| `allowed_exts` | no | Extensions accepted and validated, for example `[jpg, png]`. |
| `allowed_types` | no | MIME types accepted and validated. |
| `max_mb` | no | Max request size in MiB. Defaults to `20`. |
| `multiple` | no | Allow multiple files. |
| `show_saved_dir` | no | Show saved paths instead of only filenames after upload. |
