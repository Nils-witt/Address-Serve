// Package spa serves a Vite-built single-page app out of an embedded
// filesystem: a real file (a JS/CSS chunk, favicon, etc.) is served
// directly, and any other path falls back to index.html so the app's own
// client-side router (react-router) can take over, including a deep link
// like /ui/streets/<id> or a browser refresh on /ui/login.
package spa

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

const notBuiltMessage = "The web UI has not been built into this binary. " +
	"Run `npm --prefix frontend ci && npm --prefix frontend run build` and rebuild.\n"

// Handler builds an http.Handler serving the SPA built into distFS at the
// "dist" subdirectory (see frontend/embed.go). If the UI was never built
// (only the placeholder is embedded), every request gets a 503 explaining
// how to build it, so the API itself still starts.
func Handler(distFS fs.FS) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("sub dist: %w", err)
	}

	index, err := fs.ReadFile(sub, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, notBuiltMessage, http.StatusServiceUnavailable)
		}), nil
	}

	if err != nil {
		return nil, fmt.Errorf("read dist/index.html: %w", err)
	}

	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestedFileExists(sub, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}), nil
}

func requestedFileExists(sub fs.FS, urlPath string) bool {
	name := strings.TrimPrefix(urlPath, "/")
	if name == "" {
		return false
	}

	info, err := fs.Stat(sub, name)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
