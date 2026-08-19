package ravenixfi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	c := NewClient(RAVENIXConfig{APIKey: "test-key"})
	if c.config.BaseURL != defaultBaseURL {
		t.Errorf("expected baseURL %q, got %q", defaultBaseURL, c.config.BaseURL)
	}
	if c.config.WsURL != defaultWsURL {
		t.Errorf("expected wsURL %q, got %q", defaultWsURL, c.config.WsURL)
	}
	if c.config.Timeout != defaultTimeout {
		t.Errorf("expected timeout %d, got %d", defaultTimeout, c.config.Timeout)
	}
}

func TestNewClientCustomConfig(t *testing.T) {
	c := NewClient(RAVENIXConfig{
		APIKey:  "custom-key",
		BaseURL: "https://custom.api.com",
		WsURL:   "wss://custom.ws.com",
		Timeout: 5000,
	})
	if c.config.BaseURL != "https://custom.api.com" {
		t.Errorf("expected custom baseURL, got %q", c.config.BaseURL)
	}
	if c.config.Timeout != 5000 {
		t.Errorf("expected timeout 5000, got %d", c.config.Timeout)
	}
}

// ── Retry defaults ──────────────────────────────────────────────

func TestRetryConfigDefaults(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 300*time.Millisecond {
		t.Errorf("expected initialDelay=300ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Errorf("expected maxDelay=10s, got %v", cfg.MaxDelay)
	}
}

func TestClientRetryConfigPropagation(t *testing.T) {
	customRetry := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
	}
	c := NewClient(RAVENIXConfig{
		APIKey: "test-key",
		Retry:  &customRetry,
	})
	if c.retryCfg.MaxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", c.retryCfg.MaxRetries)
	}
	if c.retryCfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected initialDelay=100ms, got %v", c.retryCfg.InitialDelay)
	}
}

// ── Idempotency defaults ────────────────────────────────────────

func TestIdempotencyDefaults(t *testing.T) {
	c := NewClient(RAVENIXConfig{APIKey: "test-key"})
	if c.idempotency.Enabled {
		t.Error("idempotency should be disabled by default")
	}

	// Enable
	c2 := NewClient(RAVENIXConfig{
		APIKey:      "test-key",
		Idempotency: &IdempotencyConfig{Enabled: true},
	})
	if !c2.idempotency.Enabled {
		t.Error("idempotency should be enabled")
	}
}

func TestIdempotencyKeyGeneration(t *testing.T) {
	key1 := generateIdempotencyKey()
	key2 := generateIdempotencyKey()
	key3 := generateIdempotencyKey()

	if key1 == key2 || key2 == key3 || key1 == key3 {
		t.Error("idempotency keys should be unique")
	}
	if len(key1) < 6 {
		t.Errorf("key should have prefix 'RAVENIX-', got %q", key1)
	}
}

// ── Hooks ───────────────────────────────────────────────────────

func TestHooksSetAndFire(t *testing.T) {
	var reqFired bool
	var respFired bool

	c := NewClient(RAVENIXConfig{
		APIKey: "test-key",
		Hooks: &HooksConfig{
			OnRequest: func(req HookRequest) {
				reqFired = true
			},
			OnResponse: func(resp HookResponse) {
				respFired = true
			},
		},
	})

	if c.hooks.OnRequest == nil {
		t.Error("OnRequest hook should be set")
	}
	if c.hooks.OnResponse == nil {
		t.Error("OnResponse hook should be set")
	}

	// Fire them manually to verify wiring
	c.hooks.OnRequest(HookRequest{Method: "GET", URL: "/test"})
	c.hooks.OnResponse(HookResponse{Status: 200, URL: "/test"})
	if !reqFired {
		t.Error("OnRequest should have fired")
	}
	if !respFired {
		t.Error("OnResponse should have fired")
	}
}

// ── Health ──────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/health" {
			t.Errorf("expected /v1/health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{
			Status:  "ok",
			Version: "1.0.0",
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", health.Status)
	}
	if health.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", health.Version)
	}
}

// ── Accounts ────────────────────────────────────────────────────

func TestCreateAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		if r.URL.Path != "/v1/accounts" {
			t.Errorf("expected path /v1/accounts, got %s", r.URL.Path)
		}

		var req CreateAccountRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Label != "payroll" {
			t.Errorf("expected label 'payroll', got %q", req.Label)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Account{
			ID:              "acc_abc123",
			Label:           "payroll",
			Asset:           AssetUSDC,
			ShieldedBalance: "zk:0xabc...",
			CreatedAt:       "2026-06-21T00:00:00Z",
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	acct, err := c.CreateAccount(context.Background(), CreateAccountRequest{
		Label: "payroll",
		Asset: AssetUSDC,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "acc_abc123" {
		t.Errorf("expected id 'acc_abc123', got %q", acct.ID)
	}
	if acct.Asset != AssetUSDC {
		t.Errorf("expected asset USDC, got %q", acct.Asset)
	}
}

func TestCreateAccountError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RAVENIXError{
			Code:    "unauthorized",
			Message: "invalid API key",
			Status:  401,
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "bad-key", BaseURL: server.URL})
	_, err := c.CreateAccount(context.Background(), CreateAccountRequest{
		Label: "test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(RAVENIXError)
	if !ok {
		t.Fatalf("expected RAVENIXError, got %T: %v", err, err)
	}
	if apiErr.Code != "unauthorized" {
		t.Errorf("expected code 'unauthorized', got %q", apiErr.Code)
	}
	if apiErr.Status != 401 {
		t.Errorf("expected status 401, got %d", apiErr.Status)
	}
}

func TestListAccounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Account]{
			Data: []Account{
				{ID: "acc_1", Label: "savings", Asset: AssetUSDC},
				{ID: "acc_2", Label: "trading", Asset: AssetSOL},
			},
			Cursor:  "cursor_next",
			HasMore: true,
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	result, err := c.ListAccounts(context.Background(), PaginationParams{Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(result.Data))
	}
	if result.Data[0].ID != "acc_1" {
		t.Errorf("expected acc_1, got %q", result.Data[0].ID)
	}
	if !result.HasMore {
		t.Error("expected HasMore to be true")
	}
	if result.Cursor != "cursor_next" {
		t.Errorf("expected cursor 'cursor_next', got %q", result.Cursor)
	}
}

// ── Auto-Pagination ─────────────────────────────────────────────

func TestListAllAccounts(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		switch pageCount {
		case 1:
			json.NewEncoder(w).Encode(PaginatedResponse[Account]{
				Data:    []Account{{ID: "acc_1"}, {ID: "acc_2"}},
				Cursor:  "page2",
				HasMore: true,
			})
		case 2:
			json.NewEncoder(w).Encode(PaginatedResponse[Account]{
				Data:    []Account{{ID: "acc_3"}},
				Cursor:  "",
				HasMore: false,
			})
		}
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	all, err := c.ListAllAccounts(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 accounts across pages, got %d", len(all))
	}
	if all[0].ID != "acc_1" || all[2].ID != "acc_3" {
		t.Errorf("unexpected account order: %v", all)
	}
}

// ── Deposit ─────────────────────────────────────────────────────

func TestDeposit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/acc_1/deposit" {
			t.Errorf("expected path /v1/accounts/acc_1/deposit, got %s", r.URL.Path)
		}
		var req DepositRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Amount != 1000 {
			t.Errorf("expected amount 1000, got %f", req.Amount)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DepositReceipt{
			TxID:      "tx_dep_001",
			Asset:     AssetUSDC,
			Amount:    1000,
			ZKProof:   "zk:proof...",
			SettledAt: "2026-06-21T00:00:00Z",
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	receipt, err := c.Deposit(context.Background(), "acc_1", DepositRequest{
		Asset:  AssetUSDC,
		Amount: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receipt.TxID != "tx_dep_001" {
		t.Errorf("expected tx_dep_001, got %q", receipt.TxID)
	}
}

// ── Cards ───────────────────────────────────────────────────────

func TestListCards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/acc_1/cards" {
			t.Errorf("expected /v1/accounts/acc_1/cards, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Card]{
			Data: []Card{
				{ID: "card_1", Type: CardTypeVirtual, Last4: "4242"},
			},
			HasMore: false,
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	result, err := c.ListCards(context.Background(), "acc_1", PaginationParams{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Errorf("expected 1 card, got %d", len(result.Data))
	}
	if result.Data[0].ID != "card_1" {
		t.Errorf("expected card_1, got %q", result.Data[0].ID)
	}
}

func TestListAllCards(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")
		switch pageCount {
		case 1:
			json.NewEncoder(w).Encode(PaginatedResponse[Card]{
				Data:    []Card{{ID: "card_1"}, {ID: "card_2"}},
				Cursor:  "p2",
				HasMore: true,
			})
		case 2:
			json.NewEncoder(w).Encode(PaginatedResponse[Card]{
				Data:    []Card{{ID: "card_3"}},
				HasMore: false,
			})
		}
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	all, err := c.ListAllCards(context.Background(), "acc_1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 cards, got %d", len(all))
	}
}

// ── Agents ──────────────────────────────────────────────────────

func TestDeployAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/acc_1/agents" {
			t.Errorf("expected path /v1/accounts/acc_1/agents, got %s", r.URL.Path)
		}

		var req DeployAgentRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Strategy != StrategyDCA {
			t.Errorf("expected strategy dca, got %q", req.Strategy)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Agent{
			ID:              "agt_001",
			Strategy:        StrategyDCA,
			Asset:           AssetSOL,
			Amount:          100,
			Interval:        Interval1w,
			Status:          AgentStatusRunning,
			AttestationHash: "0xattest...",
			DeployedAt:      "2026-06-21T00:00:00Z",
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	agent, err := c.DeployAgent(context.Background(), "acc_1", DeployAgentRequest{
		Strategy: StrategyDCA,
		Asset:    AssetSOL,
		Amount:   100,
		Interval: Interval1w,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.ID != "agt_001" {
		t.Errorf("expected id agt_001, got %q", agent.ID)
	}
	if agent.Status != AgentStatusRunning {
		t.Errorf("expected running, got %q", agent.Status)
	}
}

func TestListAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/acc_1/agents" {
			t.Errorf("expected /v1/accounts/acc_1/agents, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Agent]{
			Data: []Agent{
				{ID: "agt_1", Strategy: StrategyDCA},
			},
			HasMore: false,
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	result, err := c.ListAgents(context.Background(), "acc_1", PaginationParams{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Errorf("expected 1 agent, got %d", len(result.Data))
	}
}

func TestListAllAgents(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")
		switch pageCount {
		case 1:
			json.NewEncoder(w).Encode(PaginatedResponse[Agent]{
				Data:    []Agent{{ID: "agt_1"}, {ID: "agt_2"}},
				Cursor:  "p2",
				HasMore: true,
			})
		case 2:
			json.NewEncoder(w).Encode(PaginatedResponse[Agent]{
				Data:    []Agent{{ID: "agt_3"}},
				HasMore: false,
			})
		}
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	all, err := c.ListAllAgents(context.Background(), "acc_1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 agents, got %d", len(all))
	}
}

// ── Pause/Resume/Stop Agent ─────────────────────────────────────

func TestAgentLifecycle(t *testing.T) {
	for _, action := range []struct {
		name string
		path string
	}{
		{"pause", "/v1/agents/agt_1/pause"},
		{"resume", "/v1/agents/agt_1/resume"},
		{"stop", "/v1/agents/agt_1/stop"},
	} {
		t.Run(action.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != action.path {
					t.Errorf("expected %s, got %s", action.path, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(Agent{
					ID:              "agt_1",
					Strategy:        StrategyDCA,
					Status:          AgentStatusPaused,
					AttestationHash: "0xrenewed...",
				})
			}))
			defer server.Close()

			c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})

			var agt Agent
			var err error
			switch action.name {
			case "pause":
				agt, err = c.PauseAgent(context.Background(), "agt_1")
			case "resume":
				agt, err = c.ResumeAgent(context.Background(), "agt_1")
			case "stop":
				agt, err = c.StopAgent(context.Background(), "agt_1")
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if agt.ID != "agt_1" {
				t.Errorf("expected agt_1, got %q", agt.ID)
			}
		})
	}
}

// ── Swap ────────────────────────────────────────────────────────

func TestSwap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/swaps" {
			t.Errorf("expected path /v1/swaps, got %s", r.URL.Path)
		}

		var req SwapRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.From != AssetSOL || req.To != AssetUSDC {
			t.Errorf("expected SOL->USDC, got %s->%s", req.From, req.To)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SwapReceipt{
			TxID:       "tx_swap_001",
			From:       AssetSOL,
			To:         AssetUSDC,
			FromAmount: 10,
			ToAmount:   1450.50,
			Price:      145.05,
			ZKProof:    "zk:swap_proof...",
			SettledAt:  "2026-06-21T00:00:00Z",
		})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{APIKey: "test-key", BaseURL: server.URL})
	receipt, err := c.Swap(context.Background(), SwapRequest{
		From:   AssetSOL,
		To:     AssetUSDC,
		Amount: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receipt.TxID != "tx_swap_001" {
		t.Errorf("expected tx_swap_001, got %q", receipt.TxID)
	}
	if receipt.ToAmount != 1450.50 {
		t.Errorf("expected 1450.50, got %f", receipt.ToAmount)
	}
}

// ── Error types ─────────────────────────────────────────────────

func TestRAVENIXErrorImplementsError(t *testing.T) {
	err := RAVENIXError{Code: "test", Message: "test msg", Status: 400}
	if err.Error() == "" {
		t.Error("Error() returned empty string")
	}
}

// ── WS event unmarshal ──────────────────────────────────────────

func TestWsEventUnmarshal(t *testing.T) {
	raw := []byte(`{"type":"balance_update","accountId":"acc_1","shieldedBalance":"zk:0xdef..."}`)
	var event WsEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event.Type != "balance_update" {
		t.Errorf("expected balance_update, got %q", event.Type)
	}
	if event.Payload == nil {
		t.Error("expected payload to be non-nil")
	}

	var balEvt BalanceUpdateEvent
	if err := json.Unmarshal(event.Payload, &balEvt); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if balEvt.AccountID != "acc_1" {
		t.Errorf("expected acc_1, got %q", balEvt.AccountID)
	}
}

// ── Retry behavior (mock) ───────────────────────────────────────

func TestRetryOn5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RAVENIXError{Code: "INTERNAL", Message: "boom", Status: 500})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Account{ID: "acc_retry", Label: "success"})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Retry:   &RetryConfig{MaxRetries: 3, InitialDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
	})
	acct, err := c.GetAccount(context.Background(), "any")
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if acct.ID != "acc_retry" {
		t.Errorf("expected acc_retry, got %q", acct.ID)
	}
	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RAVENIXError{Code: "BAD_REQUEST", Message: "invalid", Status: 400})
	}))
	defer server.Close()

	c := NewClient(RAVENIXConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Retry:   &RetryConfig{MaxRetries: 3, InitialDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
	})
	_, err := c.GetAccount(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 4xx), got %d", attempts)
	}
}

// ── Jitter ──────────────────────────────────────────────────────

func TestJitterRange(t *testing.T) {
	base := 100 * time.Millisecond
	for i := 0; i < 100; i++ {
		result := jitter(base)
		low := time.Duration(0.75 * float64(base))
		high := time.Duration(1.25 * float64(base))
		if result < low || result > high {
			t.Errorf("jitter(%v)=%v not in [%v, %v]", base, result, low, high)
		}
	}
}

// ── ReconnectConfig ─────────────────────────────────────────────

func TestReconnectConfigDefaults(t *testing.T) {
	cfg := DefaultReconnectConfig()
	if cfg.MaxReconnectAttempts != 10 {
		t.Errorf("expected 10 maxReconnectAttempts, got %d", cfg.MaxReconnectAttempts)
	}
	if cfg.ReconnectDelay != 1*time.Second {
		t.Errorf("expected 1s reconnectDelay, got %v", cfg.ReconnectDelay)
	}
}

// ── Pagination query building ───────────────────────────────────

func TestAppendPaginationQuery(t *testing.T) {
	tests := []struct {
		name     string
		params   PaginationParams
		expected string
	}{
		{"empty", PaginationParams{}, "/test"},
		{"limit only", PaginationParams{Limit: 10}, "/test?limit=10"},
		{"cursor only", PaginationParams{Cursor: "abc"}, "/test?cursor=abc"},
		{"both", PaginationParams{Limit: 5, Cursor: "xyz"}, "/test?limit=5&cursor=xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendPaginationQuery("/test", tt.params)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
