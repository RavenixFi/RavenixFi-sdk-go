// Package ravenixfi provides the Go SDK for the RAVENIX Fi privacy-native neo bank protocol.
package ravenixfi

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

// ── Core Types ──────────────────────────────────────────────────

// Asset represents a supported on-chain asset.
type Asset string

const (
	AssetUSDC Asset = "USDC"
	AssetUSDT Asset = "USDT"
	AssetEURC Asset = "EURC"
	AssetSOL  Asset = "SOL"
)

// CardType represents the type of debit card.
type CardType string

const (
	CardTypeVirtual  CardType = "virtual"
	CardTypePhysical CardType = "physical"
)

// StrategyName identifies a pre-built agent strategy.
type StrategyName string

const (
	StrategyDCA            StrategyName = "dca"
	StrategyGrid           StrategyName = "grid"
	StrategyYieldMaximizer StrategyName = "yield_maximizer"
	StrategyRiskParity     StrategyName = "risk_parity"
	StrategyMomentum       StrategyName = "momentum"
	StrategyMeanReversion  StrategyName = "mean_reversion"
)

// Interval defines how often an agent trades.
type Interval string

const (
	Interval1h  Interval = "1h"
	Interval6h  Interval = "6h"
	Interval12h Interval = "12h"
	Interval1d  Interval = "1d"
	Interval1w  Interval = "1w"
	Interval1m  Interval = "1m"
)

// CardStatus is the lifecycle state of a card.
type CardStatus string

const (
	CardStatusActive CardStatus = "active"
	CardStatusFrozen CardStatus = "frozen"
	CardStatusClosed CardStatus = "closed"
)

// AgentStatus is the lifecycle state of an agent.
type AgentStatus string

const (
	AgentStatusRunning AgentStatus = "running"
	AgentStatusPaused  AgentStatus = "paused"
	AgentStatusStopped AgentStatus = "stopped"
)

// ── Request / Response Shapes ───────────────────────────────────

// CreateAccountRequest is the input for creating a shielded account.
type CreateAccountRequest struct {
	Label string `json:"label"`
	Asset Asset  `json:"asset,omitempty"`
}

// Account is a shielded on-chain account.
type Account struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Asset           Asset  `json:"asset"`
	ShieldedBalance string `json:"shieldedBalance"`
	CreatedAt       string `json:"createdAt"`
}

// DepositRequest is the input for depositing into a shielded account.
type DepositRequest struct {
	Asset  Asset   `json:"asset"`
	Amount float64 `json:"amount"`
	Source string  `json:"source,omitempty"`
}

// DepositReceipt confirms a shielded deposit.
type DepositReceipt struct {
	TxID      string  `json:"txId"`
	Asset     Asset   `json:"asset"`
	Amount    float64 `json:"amount"`
	ZKProof   string  `json:"zkProof"`
	SettledAt string  `json:"settledAt"`
}

// IssueCardRequest is the input for issuing a virtual or physical card.
type IssueCardRequest struct {
	Type  CardType `json:"type"`
	Limit float64  `json:"limit"`
	Label string   `json:"label,omitempty"`
}

// Card is a debit card issued from a shielded account.
type Card struct {
	ID       string     `json:"id"`
	Type     CardType   `json:"type"`
	Last4    string     `json:"last4"`
	Limit    float64    `json:"limit"`
	Spent    float64    `json:"spent"`
	Status   CardStatus `json:"status"`
	IssuedAt string     `json:"issuedAt"`
}

// SpendRequest is the input for making a card payment.
type SpendRequest struct {
	Merchant string  `json:"merchant"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency,omitempty"`
}

// SpendReceipt confirms a card payment.
type SpendReceipt struct {
	TxID      string  `json:"txId"`
	Merchant  string  `json:"merchant"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	CardID    string  `json:"cardId"`
	SettledAt string  `json:"settledAt"`
}

// DeployAgentRequest is the input for deploying a TEE-guarded AI agent.
type DeployAgentRequest struct {
	Strategy StrategyName     `json:"strategy"`
	Asset    Asset            `json:"asset"`
	Amount   float64          `json:"amount"`
	Interval Interval         `json:"interval"`
	Config   *json.RawMessage `json:"config,omitempty"`
}

// Agent is a TEE-guarded autonomous trading agent.
type Agent struct {
	ID              string       `json:"id"`
	Strategy        StrategyName `json:"strategy"`
	Asset           Asset        `json:"asset"`
	Amount          float64      `json:"amount"`
	Interval        Interval     `json:"interval"`
	Status          AgentStatus  `json:"status"`
	AttestationHash string       `json:"attestationHash"`
	DeployedAt      string       `json:"deployedAt"`
}

// SwapRequest is the input for a darkpool swap.
type SwapRequest struct {
	From     Asset   `json:"from"`
	To       Asset   `json:"to"`
	Amount   float64 `json:"amount"`
	Slippage *int    `json:"slippage,omitempty"` // basis points, default 50
}

// SwapReceipt confirms a darkpool swap.
type SwapReceipt struct {
	TxID       string  `json:"txId"`
	From       Asset   `json:"from"`
	To         Asset   `json:"to"`
	FromAmount float64 `json:"fromAmount"`
	ToAmount   float64 `json:"toAmount"`
	Price      float64 `json:"price"`
	ZKProof    string  `json:"zkProof"`
	SettledAt  string  `json:"settledAt"`
}

// ── Health ──────────────────────────────────────────────────────

// HealthResponse is the response from /v1/health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ── Pagination ──────────────────────────────────────────────────

// PaginationParams controls cursor-based pagination.
type PaginationParams struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// PaginatedResponse wraps a page of results.
type PaginatedResponse[T any] struct {
	Data    []T    `json:"data"`
	Cursor  string `json:"cursor"`
	HasMore bool   `json:"hasMore"`
}

// ── WebSocket Events ────────────────────────────────────────────

// WsEvent is a real-time event pushed over the WebSocket connection.
type WsEvent struct {
	Type string `json:"type"`
	// Payload holds the raw event-specific JSON; use json.Unmarshal to decode.
	Payload json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes a WsEvent, capturing the raw per-event payload.
func (e *WsEvent) UnmarshalJSON(data []byte) error {
	raw := struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Type = raw.Type

	e.Payload = make(json.RawMessage, len(data))
	copy(e.Payload, data)
	return nil
}

// BalanceUpdateEvent is payload for "balance_update" events.
type BalanceUpdateEvent struct {
	AccountID       string `json:"accountId"`
	ShieldedBalance string `json:"shieldedBalance"`
}

// AgentUpdateEvent is payload for "agent_update" events.
type AgentUpdateEvent struct {
	AgentID string `json:"agentId"`
	Status  string `json:"status"`
}

// CardSwipeEvent is payload for "card_swipe" events.
type CardSwipeEvent struct {
	CardID   string  `json:"cardId"`
	Merchant string  `json:"merchant"`
	Amount   float64 `json:"amount"`
}

// SwapFillEvent is payload for "swap_fill" events.
type SwapFillEvent struct {
	TxID       string  `json:"txId"`
	FromAmount float64 `json:"fromAmount"`
	ToAmount   float64 `json:"toAmount"`
}

// ── Error ───────────────────────────────────────────────────────

// RAVENIXError is the structured error returned by the RAVENIX API.
type RAVENIXError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Status  int         `json:"status"`
	Details interface{} `json:"details,omitempty"`
}

// Error implements the error interface.
func (e RAVENIXError) Error() string {
	return fmt.Sprintf("ravenixfi: [%d] %s — %s", e.Status, e.Code, e.Message)
}

// ── Retry Configuration ─────────────────────────────────────────

// RetryConfig controls retry behavior.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration // default 300ms
	MaxDelay     time.Duration // default 10s
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 300 * time.Millisecond,
		MaxDelay:     10 * time.Second,
	}
}

// ── Idempotency Configuration ───────────────────────────────────

// IdempotencyConfig controls idempotency-key behavior.
type IdempotencyConfig struct {
	Enabled bool
}

// ── Hooks ───────────────────────────────────────────────────────

// HookRequest contains metadata for on-request hooks.
type HookRequest struct {
	Method  string
	URL     string
	Headers map[string]string
}

// HookResponse contains metadata for on-response hooks.
type HookResponse struct {
	Status   int
	URL      string
	Duration time.Duration
}

// HookError contains metadata for on-error hooks.
type HookError struct {
	Error    error
	URL      string
	Duration time.Duration
}

// HooksConfig holds hook callbacks.
type HooksConfig struct {
	OnRequest  func(req HookRequest)
	OnResponse func(resp HookResponse)
	OnError    func(err HookError)
}

// ── Client Config ───────────────────────────────────────────────

// RAVENIXConfig holds the configuration for an RAVENIXClient.
type RAVENIXConfig struct {
	APIKey       string
	BaseURL      string
	WsURL        string
	Timeout      int // milliseconds, default 30_000
	Retry        *RetryConfig
	Idempotency  *IdempotencyConfig
	Hooks        *HooksConfig
}

// ── WebSocket Config ────────────────────────────────────────────

// ReconnectConfig controls WebSocket auto-reconnect behavior.
type ReconnectConfig struct {
	MaxReconnectAttempts int           // default 10
	ReconnectDelay       time.Duration // default 1s
}

// DefaultReconnectConfig returns sensible defaults.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		MaxReconnectAttempts: 10,
		ReconnectDelay:       1 * time.Second,
	}
}

// ── Helpers ─────────────────────────────────────────────────────

// clamp returns value clamped to [min, max].
func clamp(val, lo, hi float64) float64 {
	return math.Max(lo, math.Min(val, hi))
}

// jitter returns a value in [0.75*base, 1.25*base].
func jitter(base time.Duration) time.Duration {
	factor := 0.75 + rand.Float64()*0.5 // ±25%
	return time.Duration(float64(base) * factor)
}

// isRetryableStatus returns true for 5xx, 429, or 0 (network error).
func isRetryableStatus(status int) bool {
	return status >= 500 || status == 429 || status == 0
}

// parseRetryAfter parses the Retry-After header (seconds or HTTP-date).
func parseRetryAfter(val string) (time.Duration, bool) {
	// Try as seconds
	if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second, true
	}
	// Try as HTTP-date
	if t, err := time.Parse(time.RFC1123, val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d, true
		}
	}
	return 0, false
}
