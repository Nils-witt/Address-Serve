package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/nils-witt/address-serve/internal/collector"
	"github.com/nils-witt/address-serve/internal/store"
)

const (
	// eventsProtocol is the WebSocket subprotocol of /api/collector-events.
	// The client offers it next to its token, and the server picks it:
	// browsers fail a handshake that picks none of the offered ones, and
	// picking the token would echo it back.
	eventsProtocol = "address-serv.v1"
	// tokenProtocolPrefix marks the subprotocol that carries the bearer
	// token, since browsers can't set an Authorization header on a WebSocket.
	tokenProtocolPrefix = "bearer."

	// eventsPingInterval keeps proxies from closing an idle connection and
	// notices a peer that is gone without closing it.
	eventsPingInterval = 30 * time.Second
	eventsWriteTimeout = 10 * time.Second

	// statusSessionExpired closes the connection when the token it was
	// opened with expires; the client reconnects with its renewed one.
	statusSessionExpired websocket.StatusCode = 4001
)

var errFellBehind = errors.New("fell behind")

// collectorEvent is a message on /api/collector-events.
type collectorEvent struct {
	// Type is "run": a run started, changed phase, saved more of its log,
	// or ended.
	Type string             `json:"type"`
	Run  store.CollectorRun `json:"run"`
}

// isWebSocketUpgrade reports whether r is a WebSocket handshake.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// webSocketToken returns the bearer token a WebSocket handshake carries as
// the subprotocol "bearer.<token>".
func webSocketToken(r *http.Request) (string, bool) {
	if !isWebSocketUpgrade(r) {
		return "", false
	}

	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for proto := range strings.SplitSeq(header, ",") {
			token, ok := strings.CutPrefix(strings.TrimSpace(proto), tokenProtocolPrefix)
			if ok && token != "" {
				return token, true
			}
		}
	}

	return "", false
}

// handleCollectorEvents pushes run updates over a WebSocket, so the UI can
// follow runs without polling. Updates missed while disconnected are not
// replayed: a client reloads the runs whenever it (re)connects.
func handleCollectorEvents(runner *collector.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The server's read and write timeouts are meant for one request,
		// not for a connection that stays open.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{eventsProtocol},
			// Origin checks guard against pages riding on credentials the
			// browser adds by itself, such as cookies. The token here is
			// sent explicitly, as on every other route, whose CORS policy
			// lets any origin call them.
			InsecureSkipVerify: true,
		})
		if err != nil {
			return // Accept has answered the handshake.
		}
		defer func() { _ = conn.CloseNow() }()

		if conn.Subprotocol() != eventsProtocol {
			_ = conn.Close(websocket.StatusPolicyViolation, "offer the subprotocol "+eventsProtocol)
			return
		}

		// The client sends nothing; reading only handles its pongs and close.
		ctx := conn.CloseRead(r.Context())

		p, _ := principalFrom(r.Context())
		if exp, err := p.Claims.GetExpirationTime(); err == nil && exp != nil {
			var cancel context.CancelFunc

			ctx, cancel = context.WithDeadline(ctx, exp.Time)
			defer cancel()
		}

		runs, unsubscribe := runner.Subscribe()
		defer unsubscribe()

		err = sendCollectorEvents(ctx, conn, runs)

		switch {
		case errors.Is(err, errFellBehind):
			_ = conn.Close(websocket.StatusTryAgainLater, "fell behind; reconnect and reload")
		// Not err: a write that timed out wraps DeadlineExceeded too.
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			_ = conn.Close(statusSessionExpired, "token expired")
		}
	}
}

// sendCollectorEvents writes each run update to conn, pinging it while idle,
// until ctx ends, a write fails, or the subscription is dropped.
func sendCollectorEvents(ctx context.Context, conn *websocket.Conn, runs <-chan store.CollectorRun) error {
	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case run, ok := <-runs:
			if !ok {
				return errFellBehind
			}

			if err := writeWithTimeout(ctx, func(ctx context.Context) error {
				return wsjson.Write(ctx, conn, collectorEvent{Type: "run", Run: run})
			}); err != nil {
				return err
			}
		case <-ping.C:
			if err := writeWithTimeout(ctx, conn.Ping); err != nil {
				return err
			}
		}
	}
}

func writeWithTimeout(ctx context.Context, write func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, eventsWriteTimeout)
	defer cancel()

	return write(ctx)
}
