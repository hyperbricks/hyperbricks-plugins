```ini
upload_fragment = <FRAGMENT>
upload_fragment.route = upload
upload_fragment.nocache = true

upload_fragment.10 = <PLUGIN>
upload_fragment.10.plugin = Upload@2.0.0
upload_fragment.10.data.label = Upload a file
upload_fragment.10.data.button = Send
upload_fragment.10.data.upload_dir = resources/uploads
upload_fragment.10.data.allowed_exts = [ jpg, png, pdf ]
upload_fragment.10.data.max_mb = 20
```
