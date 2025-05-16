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

### OpenAPI spec (swagger)

The web server hosts the API's [openAPI spec](https://www.openapis.org/what-is-openapi) using [redoc](https://github.com/Redocly/redoc). To update the swagger.json specification run the following command:

```
docker run --rm -v $(pwd):/go/src/app -w /go/src/app golang:1.24 \
  sh -c "go install github.com/swaggo/swag/cmd/swag@latest && swag init -g main.go --output internal/web/app --parseDependency --parseInternal --parseDepth 1 --parseVendor && rm internal/web/app/swagger.yaml internal/web/app/docs.go"
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

### Running locally

You can easily start the web UI locally using docker:

```bash
go build -o build/auroraboot . && \
docker run --privileged --net host \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $PWD/build/auroraboot:/bin/auroraboot \
  --entrypoint /bin/auroraboot \
  quay.io/kairos/auroraboot:latest web
```

After running the above command, you can trigger the cypress tests from within the `e2e/web` directory with:

```
npx cypress run
```
