## 📦 **EsbuildPlugin for HyperBricks**

### **Features**

* Bundles and processes **JavaScript and TypeScript**.
* Uses either [esbuild Go API](https://esbuild.github.io/api/) (default) or CLI (if `binary` is set).
* Supports `minify`, `minifyident`, `mangle`, and `sourcemap` toggles.
* Input/output directories are derived from HyperBricks' `hbConfig`.
* Optionally wraps output with custom HTML via `enclose`.
* Full debug logging of esbuild options.

---

### **How It Works**

* **Bundles** (and optionally minifies/mangles) all code starting from an entrypoint (e.g. a “bundle-entry.js” file that imports all your actual sources).
* Output is written to your module’s `static/` directory.
* Outputs the final path, optionally wrapped for HTML inclusion.
* Supports **both CLI and API mode** for maximum portability.

---

### **Plugin Config Fields**

| Key           | Required | Type   | Description                                      |
| ------------- | -------- | ------ | ------------------------------------------------ |
| `entry`       | ✓        | string | Entry file path (relative to `resources_dir`).   |
| `outfile`     | ✓        | string | Output filename (relative to `static_dir`).      |
| `minify`      |          | bool   | Minify output (whitespace, syntax).              |
| `minifyident` |          | bool   | Minify (mangle) identifiers.                     |
| `mangle`      |          | bool   | Mangle properties (aggressively shortens names). |
| `sourcemap`   |          | bool   | Generate source maps.                            |
| `binary`      |          | string | Optional: path to esbuild CLI binary.            |
| `enclose`     |          | string | Optional: HTML wrapper string.                   |
| `debug`       |          | bool   | Enable debug/verbose logging.                    |

---

### **Example HyperBricks Config**

```ini
esbuild = <PLUGIN>
esbuild.plugin = EsbuildPlugin

# These directories are derived from hbConfig, so "entry" and "outfile" are relative:
esbuild.data.entry     = js/esbuild-bundle-entry.js
esbuild.data.outfile   = js/bundle.min.esbuild.js
esbuild.data.minify    = true
esbuild.data.minifyident = true
esbuild.data.mangle    = false
esbuild.data.sourcemap = true
esbuild.data.debug     = true
# esbuild.data.binary  = "/usr/local/bin/esbuild"   # (optional CLI override)
esbuild.data.enclose   = <script src="|" defer></script>
```

---

### **Directory Conventions**

Suppose you have:

```
modules/MyModule/resources/js/
modules/MyModule/static/js/
```

* `entry` like `js/esbuild-bundle-entry.js` is resolved as:

  * `modules/MyModule/resources/js/esbuild-bundle-entry.js`
* `outfile` like `js/bundle.min.esbuild.js` is written as:

  * `modules/MyModule/static/js/bundle.min.esbuild.js`

---

### **HTML Enclose Example**

If you set:

```
esbuild.data.enclose = <script src="|" defer></script>
```

and your output file is `static/js/bundle.min.esbuild.js`,
the plugin returns:

```html
<script src="static/js/bundle.min.esbuild.js" defer></script>
```

---

### **Entry Point (Multi-file Bundling)**

To bundle multiple sources, create an entry file (e.g. `esbuild-bundle-entry.js`):

```js
import './rm-schema.js';
import './rm-parser.js';
import './js/rm-speakit.js';
import './js/rm-main.js';
```

**All files will be included in the bundle!**

---

### **Advanced: CLI Mode**

Set `binary` to use esbuild CLI instead of Go-native.
All other options are mapped (minify, mangle, sourcemap).

---

### **Debugging**

With `debug = true`, logs all options and errors for troubleshooting.

---

### **Common Issues & Solutions**

* **Output path:** Make sure `outfile` is *relative* to your static dir.
* **Entry point not found:** Use the correct path and check hbConfig-derived directories.
* **Properties not mangled:** Set `mangle = true` for full property name mangling (aggressive, use with caution!).

---

### **Summary Table**

| Use Case                | How                                  |                    |
| ----------------------- | ------------------------------------ | ------------------ |
| Simple bundle           | Set `entry` and `outfile`.           |                    |
| Minify bundle           | Set `minify = true`.                 |                    |
| Mangle identifiers only | Set `minifyident = true`.            |                    |
| Mangle property names   | Set `mangle = true`.                 |                    |
| Enable source maps      | Set `sourcemap = true`.              |                    |
| Debug mode              | Set `debug = true`.                  |                    |
| Use esbuild CLI         | Set `binary` to esbuild binary path. |                    |
| Custom HTML include     | Set `enclose` with \`                | \` as file marker. |

---

### **Sample Output for Enclose**

For this config:

```
esbuild.data.outfile = js/bundle.min.js
esbuild.data.enclose = <script src="|" defer></script>
```

Returns:

```html
<script src="static/js/bundle.min.js" defer></script>
```

---

**Need CLI command-line equivalents or more troubleshooting tips? Just ask!**
