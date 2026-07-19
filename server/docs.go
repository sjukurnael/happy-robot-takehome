package main

import (
	_ "embed"
	"net/http"
)

// The OpenAPI spec is embedded in the binary so the docs are always the
// docs of the server actually running — no separate artifact to deploy or
// let drift out of a release.
//
//go:embed openapi.yaml
var openapiSpec []byte

// Minimal Swagger UI shell pointing at the embedded spec. The UI assets
// come from a CDN: this is a dev/reviewer convenience page, not part of
// the app, so vendoring ~2MB of swagger-ui-dist into the binary isn't
// worth it.
const docsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Task Manager API — docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/api/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      defaultModelsExpandDepth: 0,
    });
  </script>
</body>
</html>`

func serveOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(openapiSpec)
}

func serveDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(docsHTML))
}
