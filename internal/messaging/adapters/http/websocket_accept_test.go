package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	websocket "github.com/coder/websocket"
)

func TestAcceptCoderWebSocketUsesOnlyConfiguredOrigin(t *testing.T) {
	const configuredOrigin = "http://localhost:15173"
	for name, test := range map[string]struct {
		origin string
		status int
	}{
		"vite proxy":     {origin: configuredOrigin, status: http.StatusSwitchingProtocols},
		"no origin curl": {origin: "", status: http.StatusSwitchingProtocols},
		"untrusted":      {origin: "https://evil.example.test", status: http.StatusForbidden},
		"prefix spoof":   {origin: configuredOrigin + ".evil", status: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			accepted := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				connection, err := acceptCoderWebSocket(writer, request, configuredOrigin)
				if err != nil {
					return
				}
				accepted <- struct{}{}
				_ = connection.Close("test complete")
			}))
			defer server.Close()

			header := http.Header{}
			if test.origin != "" {
				header.Set("Origin", test.origin)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			connection, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/user/messages/ws", &websocket.DialOptions{HTTPHeader: header})
			if test.status == http.StatusSwitchingProtocols {
				if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
					t.Fatalf("dial error=%v status=%v", err, responseStatus(response))
				}
				_ = connection.Close(websocket.StatusNormalClosure, "test complete")
				select {
				case <-accepted:
				case <-time.After(time.Second):
					t.Fatal("accepted origin did not reach server handler")
				}
				return
			}
			if err == nil || responseStatus(response) != test.status {
				t.Fatalf("dial error=%v status=%v, want %d", err, responseStatus(response), test.status)
			}
			select {
			case <-accepted:
				t.Fatal("untrusted origin reached server handler")
			default:
			}
		})
	}
}

func responseStatus(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
