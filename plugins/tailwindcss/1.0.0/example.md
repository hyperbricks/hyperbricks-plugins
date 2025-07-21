**example**

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