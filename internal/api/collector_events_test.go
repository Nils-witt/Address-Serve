package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebSocketToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		upgrade   string
		protocols []string
		want      string
		wantOK    bool
	}{
		{"token after the protocol", "websocket", []string{"address-serv.v1, bearer.abc.def"}, "abc.def", true},
		{"token in its own header", "WebSocket", []string{"address-serv.v1", "bearer.xyz"}, "xyz", true},
		{"no token", "websocket", []string{"address-serv.v1"}, "", false},
		{"empty token", "websocket", []string{"bearer."}, "", false},
		{"not an upgrade", "", []string{"bearer.abc"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/collector-events", nil)
			r.Header.Set("Upgrade", tt.upgrade)

			for _, p := range tt.protocols {
				r.Header.Add("Sec-WebSocket-Protocol", p)
			}

			got, ok := webSocketToken(r)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("webSocketToken = %q, %v, want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
