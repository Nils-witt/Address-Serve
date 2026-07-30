package main

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

const docsPageHTML = `<!doctype html>
<html>
<head>
  <title>address-serv API docs</title>
  <meta charset="utf-8" />
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      SwaggerUIBundle({
        url: "/docs/openapi.yaml",
        dom_id: "#swagger-ui",
      });
    };
  </script>
</body>
</html>`

func registerDocsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsPageHTML))
	})

	mux.HandleFunc("GET /docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openapiSpec)
	})
}
