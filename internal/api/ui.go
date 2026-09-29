package api

import (
	"net/http"
)

// UIConfig is what the web UI needs to know before anyone has signed in:
// where to send the user for an OpenID Connect login and which build it is
// talking to. It is served unauthenticated at GET /ui-config.
type UIConfig struct {
	OIDC    UIOIDCConfig `json:"oidc"`
	Version string       `json:"version"`
	Commit  string       `json:"commit"`
}

// UIOIDCConfig describes the public OIDC client the browser signs in with
// (authorization code flow with PKCE). The access token it obtains is sent to
// this API, so the provider must put the API's audience (OIDC_AUDIENCE) in it.
type UIOIDCConfig struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"clientId"`
	Scope    string `json:"scope"`
}

// registerUIRoutes mounts the SPA under /ui/ and its public config endpoint
// on mux, which is served without the auth middleware: the UI has to load
// before the user can sign in. "/" redirects to the UI.
func registerUIRoutes(mux *http.ServeMux, ui http.Handler, cfg UIConfig) {
	mux.Handle("/ui/", http.StripPrefix("/ui", ui))
	mux.HandleFunc("GET /ui-config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cfg)
	})
	mux.Handle("GET /{$}", http.RedirectHandler("/ui/", http.StatusFound))
}
