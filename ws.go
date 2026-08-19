package ravenixfi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectWebSocket opens a WebSocket connection to the virexon real-time event stream.
// It authenticates with the configured API key and returns a channel of WsEvent.
// The caller MUST read from the channel to avoid blocking the connection.
// Close the context to shut down the connection and close the channel.
func (c *virexonClient) ConnectWebSocket(ctx context.Context) (<-chan WsEvent, error) {
	u, err := url.Parse(c.config.WsURL)
	if err != nil {
		return nil, fmt.Errorf("ravenixfi: parse ws url: %w", err)
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("ravenixfi: dial ws: %w", err)
	}

	// Send auth message
	authMsg, _ := json.Marshal(map[string]string{
		"type":   "auth",
		"apiKey": c.config.APIKey,
	})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ravenixfi: ws auth: %w", err)
	}

	ch := make(chan WsEvent, 64)

	go func() {
		defer close(ch)
		defer conn.Close()

		// Close when context is done
		go func() {
			<-ctx.Done()
			conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				// Connection closed or errored — stop the reader
				return
			}

			var event WsEvent
			if err := json.Unmarshal(msg, &event); err != nil {
				log.Printf("ravenixfi: ws unmarshal: %v", err)
				continue
			}

			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// ConnectWebSocketReconnecting opens a WebSocket connection with automatic reconnection.
// On disconnect, reconnects with exponential backoff (1s → 2s → 4s … capped at 30s).
// Close the context to stop reconnection and close the channel.
func (c *virexonClient) ConnectWebSocketReconnecting(ctx context.Context, cfg ReconnectConfig) (<-chan WsEvent, error) {
	if cfg == (ReconnectConfig{}) {
		cfg = DefaultReconnectConfig()
	}

	ch := make(chan WsEvent, 64)

	go func() {
		defer close(ch)

		backoff := cfg.ReconnectDelay
		maxBackoff := 30 * time.Second
		reconnectCount := 0

		for {
			// Create a sub-context for this connection attempt
			wsCtx, wsCancel := context.WithCancel(ctx)

			u, err := url.Parse(c.config.WsURL)
			if err != nil {
				log.Printf("ravenixfi: ws reconnect parse url: %v", err)
				wsCancel()
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				continue
			}

			dialer := websocket.DefaultDialer
			conn, _, err := dialer.DialContext(wsCtx, u.String(), nil)
			if err != nil {
				log.Printf("ravenixfi: ws reconnect dial: %v (attempt %d)", err, reconnectCount+1)
				wsCancel()
				if cfg.MaxReconnectAttempts > 0 && reconnectCount >= cfg.MaxReconnectAttempts {
					return
				}
				reconnectCount++
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff = min(backoff*2, maxBackoff)
				continue
			}

			// Send auth message
			authMsg, _ := json.Marshal(map[string]string{
				"type":   "auth",
				"apiKey": c.config.APIKey,
			})
			if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
				log.Printf("ravenixfi: ws reconnect auth: %v", err)
				conn.Close()
				wsCancel()
				reconnectCount++
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff = min(backoff*2, maxBackoff)
				continue
			}

			// Reset backoff on successful connection
			backoff = cfg.ReconnectDelay
			reconnectCount = 0

			// Read loop
			func() {
				defer conn.Close()
				defer wsCancel()

				// Context cancel monitor
				go func() {
					<-ctx.Done()
					conn.WriteMessage(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					)
				}()

				for {
					_, msg, err := conn.ReadMessage()
					if err != nil {
						return // connection lost → reconnect
					}

					var event WsEvent
					if err := json.Unmarshal(msg, &event); err != nil {
						log.Printf("ravenixfi: ws unmarshal: %v", err)
						continue
					}

					select {
					case ch <- event:
					case <-ctx.Done():
						return
					}
				}
			}()

			// Check if context was cancelled during read
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	return ch, nil
}
