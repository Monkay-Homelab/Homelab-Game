package engine

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/homelab-game/backend/internal/models"
)

// helper to create a minimal GameState for bitcoin tests.
func newBitcoinTestState() *models.GameState {
	return &models.GameState{
		ID:             "test-gs",
		UserID:         "test-user",
		Tier:           models.TierRack48U,
		Money:          100000,
		BitcoinBalance: 10,
		ComputeUnits:   10_000_000,
		SaasUnlocked:   true,
	}
}

func makePayload(amount int64) json.RawMessage {
	data, _ := json.Marshal(map[string]int64{"amount": amount})
	return data
}

// ======================================================================
// buyBitcoin tests
// ======================================================================

func TestBuyBitcoin_Success(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 50000
	gs.BitcoinBalance = 0

	result, err := e.buyBitcoin(gs, makePayload(3), 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if gs.Money != 20000 {
		t.Errorf("Money = %d, want 20000 (50000 - 3*10000)", gs.Money)
	}
	if gs.BitcoinBalance != 3 {
		t.Errorf("BitcoinBalance = %d, want 3", gs.BitcoinBalance)
	}
}

func TestBuyBitcoin_ExactBalance(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 30000
	gs.BitcoinBalance = 0

	_, err := e.buyBitcoin(gs, makePayload(3), 10000)
	if err != nil {
		t.Fatalf("should succeed with exact balance: %v", err)
	}
	if gs.Money != 0 {
		t.Errorf("Money = %d, want 0", gs.Money)
	}
	if gs.BitcoinBalance != 3 {
		t.Errorf("BitcoinBalance = %d, want 3", gs.BitcoinBalance)
	}
}

func TestBuyBitcoin_InsufficientFunds(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 15000

	_, err := e.buyBitcoin(gs, makePayload(3), 10000)
	if err == nil {
		t.Fatal("expected error for insufficient funds")
	}
	expected := "not enough money (need $30000, have $15000)"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
	// State should be unchanged.
	if gs.Money != 15000 {
		t.Errorf("Money mutated on failed buy: %d", gs.Money)
	}
}

func TestBuyBitcoin_ZeroAmount(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.buyBitcoin(gs, makePayload(0), 10000)
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err.Error() != "amount must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "amount must be positive")
	}
}

func TestBuyBitcoin_NegativeAmount(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.buyBitcoin(gs, makePayload(-5), 10000)
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
	if err.Error() != "amount must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "amount must be positive")
	}
}

func TestBuyBitcoin_PriceZero(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.buyBitcoin(gs, makePayload(1), 0)
	if err == nil {
		t.Fatal("expected error for price <= 0")
	}
	if err.Error() != "bitcoin market unavailable" {
		t.Errorf("error = %q, want %q", err.Error(), "bitcoin market unavailable")
	}
}

func TestBuyBitcoin_PriceNegative(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.buyBitcoin(gs, makePayload(1), -1000)
	if err == nil {
		t.Fatal("expected error for negative price")
	}
	if err.Error() != "bitcoin market unavailable" {
		t.Errorf("error = %q, want %q", err.Error(), "bitcoin market unavailable")
	}
}

func TestBuyBitcoin_IntegerOverflow(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = math.MaxInt64

	// amount * price would overflow int64.
	_, err := e.buyBitcoin(gs, makePayload(math.MaxInt64), 2)
	if err == nil {
		t.Fatal("expected error for integer overflow")
	}
	if err.Error() != "amount too large" {
		t.Errorf("error = %q, want %q", err.Error(), "amount too large")
	}
}

func TestBuyBitcoin_OverflowBoundary(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = math.MaxInt64
	gs.ComputeUnits = math.MaxInt64

	// With CU cost of 100000 per BTC, the tighter overflow boundary is
	// MaxInt64 / 100000 = 92233720368547 (CU multiplication).
	// The money boundary at price 10000 is MaxInt64 / 10000 = 922337203685477.
	// Use the CU-safe boundary to verify neither multiplication overflows.
	maxSafe := math.MaxInt64 / int64(100000)
	_, err := e.buyBitcoin(gs, makePayload(maxSafe), 10000)
	if err != nil {
		t.Fatalf("should not overflow at boundary: %v", err)
	}
}

func TestBuyBitcoin_InvalidPayload(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.buyBitcoin(gs, json.RawMessage(`invalid`), 10000)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// ======================================================================
// sellBitcoin tests
// ======================================================================

func TestSellBitcoin_Success(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.BitcoinBalance = 5
	gs.Money = 0

	result, err := e.sellBitcoin(gs, makePayload(3), 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if gs.BitcoinBalance != 2 {
		t.Errorf("BitcoinBalance = %d, want 2", gs.BitcoinBalance)
	}
	if gs.Money != 30000 {
		t.Errorf("Money = %d, want 30000", gs.Money)
	}
}

func TestSellBitcoin_ExactBalance(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.BitcoinBalance = 2
	gs.Money = 0

	_, err := e.sellBitcoin(gs, makePayload(2), 10000)
	if err != nil {
		t.Fatalf("should succeed selling exact balance: %v", err)
	}
	if gs.BitcoinBalance != 0 {
		t.Errorf("BitcoinBalance = %d, want 0", gs.BitcoinBalance)
	}
	if gs.Money != 20000 {
		t.Errorf("Money = %d, want 20000", gs.Money)
	}
}

func TestSellBitcoin_InsufficientBalance(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.BitcoinBalance = 1

	_, err := e.sellBitcoin(gs, makePayload(2), 10000)
	if err == nil {
		t.Fatal("expected error for insufficient bitcoin")
	}
	expected := "not enough bitcoin (need 2, have 1)"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
	// State should be unchanged.
	if gs.BitcoinBalance != 1 {
		t.Errorf("BitcoinBalance mutated on failed sell: %d", gs.BitcoinBalance)
	}
}

func TestSellBitcoin_ZeroAmount(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.sellBitcoin(gs, makePayload(0), 10000)
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err.Error() != "amount must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "amount must be positive")
	}
}

func TestSellBitcoin_NegativeAmount(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.sellBitcoin(gs, makePayload(-1), 10000)
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
	if err.Error() != "amount must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "amount must be positive")
	}
}

func TestSellBitcoin_PriceZero(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.sellBitcoin(gs, makePayload(1), 0)
	if err == nil {
		t.Fatal("expected error for price <= 0")
	}
	if err.Error() != "bitcoin market unavailable" {
		t.Errorf("error = %q, want %q", err.Error(), "bitcoin market unavailable")
	}
}

func TestSellBitcoin_PriceNegative(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.sellBitcoin(gs, makePayload(1), -500)
	if err == nil {
		t.Fatal("expected error for negative price")
	}
	if err.Error() != "bitcoin market unavailable" {
		t.Errorf("error = %q, want %q", err.Error(), "bitcoin market unavailable")
	}
}

func TestSellBitcoin_IntegerOverflow(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.BitcoinBalance = math.MaxInt64

	_, err := e.sellBitcoin(gs, makePayload(math.MaxInt64), 2)
	if err == nil {
		t.Fatal("expected error for integer overflow")
	}
	if err.Error() != "amount too large" {
		t.Errorf("error = %q, want %q", err.Error(), "amount too large")
	}
}

func TestSellBitcoin_InvalidPayload(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()

	_, err := e.sellBitcoin(gs, json.RawMessage(`{bad`), 10000)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// ======================================================================
// ProcessAction dispatch tests
// ======================================================================

func TestProcessAction_BuyBitcoinDispatch(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 50000
	gs.BitcoinBalance = 0

	_, err := e.ProcessAction(gs, "buy_bitcoin", makePayload(2), nil, nil, nil, nil, nil, 10000)
	if err != nil {
		t.Fatalf("buy_bitcoin dispatch failed: %v", err)
	}
	if gs.BitcoinBalance != 2 {
		t.Errorf("BitcoinBalance = %d, want 2", gs.BitcoinBalance)
	}
}

func TestProcessAction_SellBitcoinDispatch(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.BitcoinBalance = 5
	gs.Money = 0

	_, err := e.ProcessAction(gs, "sell_bitcoin", makePayload(3), nil, nil, nil, nil, nil, 10000)
	if err != nil {
		t.Fatalf("sell_bitcoin dispatch failed: %v", err)
	}
	if gs.Money != 30000 {
		t.Errorf("Money = %d, want 30000", gs.Money)
	}
}

func TestProcessAction_NonBitcoinActionIgnoresPrice(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	originalMoney := gs.Money

	// run_job should work fine with bitcoin price = 0.
	_, err := e.ProcessAction(gs, "run_job", nil, nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("run_job failed: %v", err)
	}
	// run_job adds compute, not money. Money should be unchanged.
	if gs.Money != originalMoney {
		t.Errorf("Money changed by run_job: %d (was %d)", gs.Money, originalMoney)
	}
}

// ======================================================================
// Prestige persistence tests
// ======================================================================

func TestPrestige_BitcoinBalancePersists(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = true
	gs.BitcoinBalance = 42
	gs.ColoCount = 0

	result, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	// BitcoinBalance should survive prestige.
	if gs.BitcoinBalance != 42 {
		t.Errorf("BitcoinBalance = %d after prestige, want 42 (should persist)", gs.BitcoinBalance)
	}
}

func TestPrestige_MoneyIsReset(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = true
	gs.Money = 999999
	gs.ColoCount = 0

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	// Money should be reset to 0.
	if gs.Money != 0 {
		t.Errorf("Money = %d after prestige, want 0 (should be reset)", gs.Money)
	}
}

func TestPrestige_KnowledgePointsPersist(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = true
	gs.KnowledgePoints = 15
	gs.ColoCount = 0

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	if gs.KnowledgePoints != 15 {
		t.Errorf("KnowledgePoints = %d after prestige, want 15", gs.KnowledgePoints)
	}
}

func TestPrestige_ComputeUnitsReset(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = true
	gs.ComputeUnits = 999999
	gs.ColoCount = 0

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	if gs.ComputeUnits != 0 {
		t.Errorf("ComputeUnits = %d after prestige, want 0", gs.ComputeUnits)
	}
}

// ======================================================================
// Round-trip: buy then sell at same price should restore money
// ======================================================================

func TestBuySellRoundTrip(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 100000
	gs.BitcoinBalance = 0

	// Buy 5 BTC at 10000.
	_, err := e.buyBitcoin(gs, makePayload(5), 10000)
	if err != nil {
		t.Fatalf("buy error: %v", err)
	}
	if gs.Money != 50000 || gs.BitcoinBalance != 5 {
		t.Fatalf("after buy: Money=%d, BTC=%d", gs.Money, gs.BitcoinBalance)
	}

	// Sell 5 BTC at 10000.
	_, err = e.sellBitcoin(gs, makePayload(5), 10000)
	if err != nil {
		t.Fatalf("sell error: %v", err)
	}
	if gs.Money != 100000 || gs.BitcoinBalance != 0 {
		t.Errorf("round-trip: Money=%d (want 100000), BTC=%d (want 0)", gs.Money, gs.BitcoinBalance)
	}
}

// Test selling at higher price yields profit.
func TestBuySellWithPriceIncrease(t *testing.T) {
	e := New()
	gs := newBitcoinTestState()
	gs.Money = 100000
	gs.BitcoinBalance = 0

	// Buy 5 BTC at 10000.
	_, err := e.buyBitcoin(gs, makePayload(5), 10000)
	if err != nil {
		t.Fatalf("buy error: %v", err)
	}

	// Sell 5 BTC at 15000.
	_, err = e.sellBitcoin(gs, makePayload(5), 15000)
	if err != nil {
		t.Fatalf("sell error: %v", err)
	}
	// Spent 50000, received 75000. Net: 100000 - 50000 + 75000 = 125000.
	if gs.Money != 125000 {
		t.Errorf("Money = %d, want 125000", gs.Money)
	}
}
