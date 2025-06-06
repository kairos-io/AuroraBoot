# Web UI

## Building frontend assets

### Tailwind CSS

**You need to have the tailwind CSS CLI installed.**

To install it you can run:

```
npm install tailwindcss @tailwindcss/cli --save-dev
```

**You need to have the esbuild CLI installed.**

To build the assets, run the following commands in the `web/app` directory.

Install depencencies with

```
npm install
```

Generate the output.css file

```bash
npx tailwindcss -i ./assets/css/tailwind.css -o ./output.css
```

Generate the bundle.js file

```bash
esbuild index.js --bundle --outfile=bundle.js
```

### OpenAPI spec (swagger)

The web server hosts the API's [openAPI spec](https://www.openapis.org/what-is-openapi) using [redoc](https://github.com/Redocly/redoc). To update the swagger.json specification run the following command:

```
docker run --rm -v $(pwd):/go/src/app -w /go/src/app golang:1.24 \
  sh -c "go install github.com/swaggo/swag/cmd/swag@latest && swag init -g main.go --output internal/web/app --parseDependency --parseInternal --parseDepth 1 --parseVendor && rm internal/web/app/swagger.yaml internal/web/app/docs.go"
```

## Docker

See the README.md at the root of the project