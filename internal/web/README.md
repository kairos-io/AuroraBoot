# Web UI

## Building frontend assets

### Tailwind CSS

**You need to have the tailwind CSS CLI installed.**

To build the Tailwind CSS assets, run the following command:

```bash
tailwindcss -i ./web/assets/tailwind.css -o ./web/app/output.css
```

During development you can use the watch command:

```bash
tailwindcss -i ./web/assets/tailwind.css -o ./web/app/output.css --watch
```

### JavaScript

**You need to have the esbuild CLI installed.**

To build the JavaScript assets, run the following command in the `web/app` directory:

```bash
esbuild index.js --bundle --outfile=bundle.js
```

or using docker (from within this directory):

```bash
docker run --rm -v $PWD/app:/work --workdir /work node:23 /bin/bash -c 'npm ci && pwd && echo 'y' | npx esbuild index.js --bundle --outfile=bundle.js'
```
