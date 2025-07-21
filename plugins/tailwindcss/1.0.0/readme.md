## 📦 **TailwindCssPlugin for HyperBricks (Tailwind v4+)**
## Overview

**TailwindCssPlugin** builds Tailwind CSS using the [standalone CLI](https://tailwindcss.com/blog/standalone-cli).
It’s designed for Tailwind v4+ (which no longer uses CLI `--content`).

---

## 🚀 Quickstart

**1. Create your Tailwind config** (`tailwind.config.js`):

```js
module.exports = {
  content: [
    './modules/RoutineMaster/templates/**/*.html', // <--- adjust as needed
  ],
  // ...your Tailwind settings
}
```

**2. Reference the config in your input CSS**
*(Required if your config file isn’t named `tailwind.config.js` in project root)*

```css
@config "../../../../tailwind.config.js";
@tailwind base;
@tailwind components;
@tailwind utilities;
```

**3. Plugin config example:**

```
tailwind = <PLUGIN>
tailwind.plugin = TailwindCssPlugin
tailwind.data.input_css = modules/RoutineMaster/resources/src/css/base.css
tailwind.data.output_css = modules/RoutineMaster/static/css/front_page.css
tailwind.data.signal = true
tailwind.data.minify = true
tailwind.data.debug = true
tailwind.data.enclose = <link rel="stylesheet" href="|">

#optional for inline use css marker like this:
tailwind.data.enclose = <style>{{css}}</style>
```

---

## 🔍 Key Options

| Option       | Required | Purpose                                                                      |
| ------------ | -------- | ---------------------------------------------------------------------------- |
| `input_css`  | Yes      | Your CSS file that imports Tailwind (and optionally your config)             |
| `output_css` | Yes      | Where to write the compiled CSS                                              |
| `config`     | No       | Path to Tailwind config (usually handled via `@config` in input CSS for v4+) |
| `binary`     | No       | Path to Tailwind CLI binary (default: `tailwindcss`)                         |
| `minify`     | No       | If true, minifies output                                                     |
| `debug`      | No       | If true, prints full CLI stdout/stderr                                       |
| `signal`     | No       | If true, runs a sanity check to confirm CLI is working                       |
| `enclose`    | No       | If set, wraps output (not typical for static files)                          |

---

## 📝 Notes

* `output_css` **must** be set.
* Tailwind ‘content’ scanning is **always** controlled in your `tailwind.config.js` file.
* Use `@config` at the top of your input CSS if your config file isn’t in the project root.
* Use `debug: true` for troubleshooting—shows all CLI output.
* Use `signal: true` if you want the plugin to check Tailwind CLI availability at build time.

---

## Example Directory Structure

```
project-root/
  tailwind.config.js
  modules/
    RoutineMaster/
      templates/
      resources/
        src/css/base.css     <-- input_css
      static/css/front_page.css  <-- output_css
```

---

## 🔗 Reference

See [Tailwind Standalone CLI](https://tailwindcss.com/blog/standalone-cli)
See [Tailwind v4 ‘@config’](https://tailwindcss.com/docs/content-configuration#using-tailwind-without-node-js)
