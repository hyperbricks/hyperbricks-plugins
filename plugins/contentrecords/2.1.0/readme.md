# ContentRecords Plugin for Hyperbricks

## Overview
ContentRecords is a **template-driven content store**. You define a Hyperbricks template, add `@bind` mappings, and the plugin stores those fields in SQLite. It can then **render** or **edit** records using the same template.

### Core concepts
- **Template**: a Hyperbricks `<TREE>` (or `<TEMPLATE>` inside a tree) that defines structure.
- **Bind**: `@bind { field, path }` mapping that connects a record field to a template path.
- **Record**: a single row of stored fields (all values stored as strings).
- **Store**: a SQLite database file.

## Views & actions
Instead of `cms|render|edit`, this plugin uses a **two-axis** model:

- `view = list | single`
- `action = render | edit`

Examples:
- **list + render** → render a list of records
- **list + edit** → list editor (rows with `@list` fields + Edit/Delete)
- **single + render** → render a single record
- **single + edit** → edit one record

## Config fields

| Key | Required | Type | Description |
| --- | --- | --- | --- |
| `template` | ✓ | tree | Template tree to clone for render. |
| `view` |  | string | `list` (default) or `single`. |
| `action` |  | string | `render` (default) or `edit`. |
| `store` | ✓ | string | Path to SQLite DB. |
| `schema` |  | object | Form schema for edit views (binds, labels, types, optional `order`). |
| `query` |  | string | SQL used to select record IDs (first column). |
| `id` |  | string/int | Record id for `view=single`. |
| `ids` |  | list | Record ids for `view=list`. |
| `teaser` |  | bool | When true, render only nodes marked `@teaser = true` (fallback to full template if none). |
| `inline` |  | bool | Enable inline edit wrappers in render views (active when `inline_param` query param is truthy). |
| `inline_param` |  | string | Query param name for inline mode (default `edit`). |
| `preview` |  | bool | Show preview panel in edit views (default `true`). Set to `false` to hide. |
| `seed` |  | bool | Insert a record from template values when DB is empty. |
| `editable` |  | bool | Adds edit link to rendered items. |
| `edit_route` |  | string | Base path for edit links. |
| `list_route` |  | string | Redirect target after save/delete in `view=single` + `action=edit` (defaults to `edit_route`). |
| `record_param` |  | string | Query param name used for edit links (default `id`). |
| `upload_dir` |  | string | Folder for file uploads (e.g. `{{RESOURCES}}/images/`). |
| `upload` |  | string | Alias for `upload_dir`. |

Legacy aliases (optional): `type`, `fields`, `sql`, `route`, `mode`.

## Example (template + list render + list edit)

```ini
article = <TREE>
article {
    10 = <TEXT>
    10.value = Hello World
    10.@bind {
        field = title
        path = value
    }
    10.@list = true
    10.@teaser = true

    15 = <PLUGIN>
    15.plugin = MarkdownPlugin@1.0.0
    15.data.class = md
    15.data.content = <<[
        # Welcome this is **Markdown** content.
    ]>>
    15.@bind {
        field = content
        path = data.content
    }

    20 = <IMAGE>
    20.src = {{RESOURCES}}/images/GitHub_Logo_White.png
    20.height = 100
    20.quality = 100
    20.class = article_image
    20.alt = article alt
    20.enclose = <a href="index.html">|</a>
    20.@bind {
        field = image
        path = src
    }
    20.@list = true
    20.@teaser = true
}

articles_list_edit = <PLUGIN>
articles_list_edit.plugin = ContentRecords@2.1.0
articles_list_edit.data.template < article
articles_list_edit.data.view = list
articles_list_edit.data.action = edit
articles_list_edit.data.store = {{RESOURCES}}/database/articles.db
articles_list_edit.data.upload_dir = {{RESOURCES}}/images/
articles_list_edit.data.schema = {
    title {
        type = text
        path = 10.value
        label = article title
        order = 10
    }
    content {
        type = markdown
        path = 15.data.content
        label = article content
        order = 20
    }
    image {
        type = image
        path = 20.src
        label = upload your image
        order = 30
    }
}

articles_list_render = <PLUGIN>
articles_list_render.plugin = ContentRecords@2.1.0
articles_list_render.data.template < article
articles_list_render.data.view = list
articles_list_render.data.action = render
articles_list_render.data.store = {{RESOURCES}}/database/articles.db
articles_list_render.data.schema < articles_list_edit.data.schema
articles_list_render.data.seed = true
articles_list_render.data.editable = true
articles_list_render.data.edit_route = articles/cms/edit
articles_list_render.data.teaser = true
articles_list_render.data.query = <<[
    select id from records where type = 'article' order by id
]>>
```

## Single record render

```ini
article_single = <PLUGIN>
article_single.plugin = ContentRecords@2.1.0
article_single.data.template < article
article_single.data.view = single
article_single.data.action = render
article_single.data.store = {{RESOURCES}}/database/articles.db
article_single.data.id = 15
```

## Single record edit

```ini
article_edit_single = <PLUGIN>
article_edit_single.plugin = ContentRecords@2.1.0
article_edit_single.data.template < article
article_edit_single.data.view = single
article_edit_single.data.action = edit
article_edit_single.data.store = {{RESOURCES}}/database/articles.db
article_edit_single.data.record_param = id
article_edit_single.data.upload_dir = {{RESOURCES}}/images/
```

## Inline editing (render views)
Inline editing is available in render views when `data.inline = true`.
It activates only when the inline query param is present (default `?edit=1`).
Inline inputs are chosen from `schema` types (`markdown` → textarea, `image` → upload); otherwise fields default to text.

Make sure the inline script is bundled (see `modules/docs/resources/js/main.js`).

```ini
articles_inline = <PLUGIN>
articles_inline.plugin = ContentRecords@2.1.0
articles_inline.data.template < article
articles_inline.data.view = list
articles_inline.data.action = render
articles_inline.data.store = {{RESOURCES}}/database/articles.db
articles_inline.data.schema < articles_list_edit.data.schema
articles_inline.data.inline = true
articles_inline.data.inline_param = edit
articles_inline.data.upload_dir = {{RESOURCES}}/images/
```

## Notes
- The plugin **returns a map**; Hyperbricks renders it (no HTML here).
- SQLite schema is created automatically on first hit:
  - `records(id, type, created_at, updated_at)`
  - `record_fields(record_id, bind_key, value)`
- Record **`type`** is stored from the template’s `@name` (if present).  
  If `@name` is missing, `records.type` will be empty.
- If you provide custom `query`, **type filtering is your responsibility**.
- Bind keys must be **unique per template** (first one wins).
- Values are stored as **strings**; Hyperbricks handles typing at render time.
- `seed = true` inserts one record only when the DB is empty.
- `@list = true` marks fields for **list edit** (CMS rows).
- `@teaser = true` marks fields for **teaser render** (used when `data.teaser = true`).
- `schema` supports an optional **`order`** field to control form and list row ordering.
- `inline = true` wraps bound nodes with inline editor attributes (requires `?edit=1` and the inline script).
- Nested ContentRecords: ContentRecords plugin nodes are treated as traversal boundaries automatically. Unknown inline binds pass through when a boundary exists, and the inline JSON body is preserved so nested plugins can parse the same request. For nested inline editing, enable `data.inline = true` on the nested renderer as well. To mark other plugin subtrees as boundaries, set `data.content_record_boundary = true`.

## List + teaser flags
Use template-level flags to avoid creating separate templates:

- `@list = true` → include this node in **list edit** rows.  
  If no `@list` flags exist, the list shows all fields.
- `@teaser = true` → include this node in **teaser render** when `data.teaser = true`.  
  If no `@teaser` flags exist, render falls back to the full template.

## CMS inline template values (`view=list`, `action=edit`)
When list editing, the plugin returns a `<TEMPLATE>` node with `inline` HTML.
The template receives:

- `type` — resolved content type (from template `@name`).
- `view`, `action` — the current mode.
- `store` — SQLite path.
- `query` — query string used to select records (if any).
- `edit_route` — base route for edit links (used by list UI).
- `record_param` — query param name for edit links (default `id`).
- `records` — record id → `{ id, fields }` (all values strings).
- `record_ids` — ordered list of record ids.
- `fields` — schema map: `{ name, label, type, bind, path }` per field.
- `field_ids` — ordered list of schema keys.
- `list_field_ids` — ordered list of fields shown in list rows.
- `show_preview` — boolean flag (default `true`) controlling preview visibility.
- `preview` — prebuilt `<TREE>` list of records.

## File uploads (image fields)
If a field has `type = image`, the edit forms use `multipart/form-data` and accept a file upload.
When a file is uploaded, the plugin stores it in `data.upload_dir` and saves the resulting path
into the record field (as a string). If no file is uploaded, the existing value is preserved.
