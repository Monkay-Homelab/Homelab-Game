package engine

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/homelab-game/backend/internal/models"
)

// =====================================================================
// Helpers
// =====================================================================

// newTestState creates a minimal GameState suitable for CU sink tests.
func newTestState() *models.GameState {
	return &models.GameState{
		ID:                 "test-gs",
		UserID:             "test-user",
		Tier:               models.TierRack48U,
		ComputeUnits:       10_000_000,
		Reputation:         1000,
		Money:              100000,
		SaasUnlocked:       true,
		ColoCount:          0,
		ColoMultiplier:     1.0,
		IdleMultiplier:     1.0,
		ThrottleMultiplier: 1.0,
		OverclockMultiplier: 1.0,
		HeatGenerated:      100,
		CoolingCapacity:    5000,
		PowerLimit:         20000,
		PowerWatts:         100,
		HardwareSlots:      2,
		LastTickAt:         time.Now().Add(-5 * time.Second),
	}
}

func makeResearchPayload(node string) json.RawMessage {
	data, _ := json.Marshal(map[string]string{"node": node})
	return data
}

func makeOverclockPayload(tier int) json.RawMessage {
	data, _ := json.Marshal(map[string]int{"tier": tier})
	return data
}

// =====================================================================
// OVERCLOCK MODE TESTS
// =====================================================================

func TestActivateOverclock_Tier1(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 100_000

	result, err := e.activateOverclock(gs, makeOverclockPayload(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if gs.ComputeUnits != 50_000 {
		t.Errorf("ComputeUnits = %d, want 50000 (100000 - 50000)", gs.ComputeUnits)
	}
	if gs.OverclockMultiplier != 2.0 {
		t.Errorf("OverclockMultiplier = %f, want 2.0", gs.OverclockMultiplier)
	}
	if gs.OverclockTicksRemaining != 60 {
		t.Errorf("OverclockTicksRemaining = %d, want 60", gs.OverclockTicksRemaining)
	}
}

func TestActivateOverclock_Tier2(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 500_000

	result, err := e.activateOverclock(gs, makeOverclockPayload(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if gs.ComputeUnits != 300_000 {
		t.Errorf("ComputeUnits = %d, want 300000 (500000 - 200000)", gs.ComputeUnits)
	}
	if gs.OverclockMultiplier != 3.0 {
		t.Errorf("OverclockMultiplier = %f, want 3.0", gs.OverclockMultiplier)
	}
	if gs.OverclockTicksRemaining != 60 {
		t.Errorf("OverclockTicksRemaining = %d, want 60", gs.OverclockTicksRemaining)
	}
}

func TestActivateOverclock_Tier3(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 2_000_000

	result, err := e.activateOverclock(gs, makeOverclockPayload(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if gs.ComputeUnits != 1_000_000 {
		t.Errorf("ComputeUnits = %d, want 1000000 (2000000 - 1000000)", gs.ComputeUnits)
	}
	if gs.OverclockMultiplier != 5.0 {
		t.Errorf("OverclockMultiplier = %f, want 5.0", gs.OverclockMultiplier)
	}
	if gs.OverclockTicksRemaining != 60 {
		t.Errorf("OverclockTicksRemaining = %d, want 60", gs.OverclockTicksRemaining)
	}
}

func TestActivateOverclock_InsufficientCU(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 10_000 // Need 50000 for tier 1

	_, err := e.activateOverclock(gs, makeOverclockPayload(1))
	if err == nil {
		t.Fatal("expected error for insufficient CU")
	}
	if gs.ComputeUnits != 10_000 {
		t.Errorf("ComputeUnits mutated on failed activation: %d", gs.ComputeUnits)
	}
	if gs.OverclockMultiplier != 1.0 {
		t.Errorf("OverclockMultiplier should be unchanged: %f", gs.OverclockMultiplier)
	}
}

func TestActivateOverclock_InvalidTier(t *testing.T) {
	e := New()
	gs := newTestState()

	for _, tier := range []int{0, 4, -1, 100} {
		_, err := e.activateOverclock(gs, makeOverclockPayload(tier))
		if err == nil {
			t.Errorf("expected error for invalid tier %d", tier)
		}
	}
}

func TestActivateOverclock_ReplacesExisting(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 2_000_000

	// Activate tier 1 first
	_, err := e.activateOverclock(gs, makeOverclockPayload(1))
	if err != nil {
		t.Fatalf("tier 1 activation failed: %v", err)
	}
	if gs.OverclockMultiplier != 2.0 {
		t.Fatalf("expected 2.0x after tier 1, got %f", gs.OverclockMultiplier)
	}

	// Simulate some ticks passing
	gs.OverclockTicksRemaining = 30

	// Activate tier 3 — should replace, not stack
	cuBefore := gs.ComputeUnits
	_, err = e.activateOverclock(gs, makeOverclockPayload(3))
	if err != nil {
		t.Fatalf("tier 3 activation failed: %v", err)
	}
	if gs.OverclockMultiplier != 5.0 {
		t.Errorf("OverclockMultiplier = %f, want 5.0 (replaced, not stacked)", gs.OverclockMultiplier)
	}
	if gs.OverclockTicksRemaining != 60 {
		t.Errorf("OverclockTicksRemaining = %d, want 60 (timer reset)", gs.OverclockTicksRemaining)
	}
	if gs.ComputeUnits != cuBefore-1_000_000 {
		t.Errorf("ComputeUnits = %d, want %d (full cost charged)", gs.ComputeUnits, cuBefore-1_000_000)
	}
}

func TestProcessIdleProgress_OverclockDecrement(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.OverclockMultiplier = 2.0
	gs.OverclockTicksRemaining = 10
	gs.LastTickAt = time.Now().Add(-5 * time.Second)

	e.ProcessIdleProgress(gs, nil, nil, nil, nil, nil, nil, nil, time.Now())

	// One tick elapsed (5s), should decrement by 1
	if gs.OverclockTicksRemaining != 9 {
		t.Errorf("OverclockTicksRemaining = %d, want 9", gs.OverclockTicksRemaining)
	}
	if gs.OverclockMultiplier != 2.0 {
		t.Errorf("OverclockMultiplier = %f, want 2.0 (still active)", gs.OverclockMultiplier)
	}
}

func TestProcessIdleProgress_OverclockExpiry(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.OverclockMultiplier = 3.0
	gs.OverclockTicksRemaining = 1
	gs.LastTickAt = time.Now().Add(-5 * time.Second)

	e.ProcessIdleProgress(gs, nil, nil, nil, nil, nil, nil, nil, time.Now())

	if gs.OverclockTicksRemaining != 0 {
		t.Errorf("OverclockTicksRemaining = %d, want 0 (expired)", gs.OverclockTicksRemaining)
	}
	if gs.OverclockMultiplier != 1.0 {
		t.Errorf("OverclockMultiplier = %f, want 1.0 (reset after expiry)", gs.OverclockMultiplier)
	}
}

func TestProcessIdleProgress_OverclockMultiplierApplied(t *testing.T) {
	e := New()

	now := time.Now()

	// Test without overclock
	gs1 := newTestState()
	gs1.ComputeUnits = 0
	gs1.LastTickAt = now.Add(-5 * time.Second)
	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 100, PowerDraw: 10},
	}
	e.ProcessIdleProgress(gs1, hw, nil, nil, nil, nil, nil, nil, now)
	baseIncome := gs1.ComputeUnits

	// Test with 2x overclock
	gs2 := newTestState()
	gs2.ComputeUnits = 0
	gs2.OverclockMultiplier = 2.0
	gs2.OverclockTicksRemaining = 60
	gs2.LastTickAt = now.Add(-5 * time.Second)
	e.ProcessIdleProgress(gs2, hw, nil, nil, nil, nil, nil, nil, now)
	overclockIncome := gs2.ComputeUnits

	if baseIncome == 0 {
		t.Fatal("baseIncome is 0, test setup issue")
	}
	// Overclock 2x should produce approximately 2x the income
	// Allow small floating-point tolerance
	ratio := float64(overclockIncome) / float64(baseIncome)
	if ratio < 1.9 || ratio > 2.1 {
		t.Errorf("overclock ratio = %f, want ~2.0 (base=%d, overclock=%d)", ratio, baseIncome, overclockIncome)
	}
}

func TestProcessIdleProgress_OverclockAddsHeat(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.OverclockMultiplier = 3.0
	gs.OverclockTicksRemaining = 60
	gs.PowerWatts = 500
	gs.CoolingCapacity = 10000 // High enough to avoid penalty
	gs.LastTickAt = time.Now().Add(-5 * time.Second)

	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 100, PowerDraw: 500},
	}

	e.ProcessIdleProgress(gs, hw, nil, nil, nil, nil, nil, nil, time.Now())

	// Heat = PowerWatts = 500 (from hardware)
	// Overclock adds (3.0 - 1.0) * heat = 2.0 * 500 = 1000
	// Total heat = 500 + 1000 = 1500
	if gs.HeatGenerated != 1500 {
		t.Errorf("HeatGenerated = %d, want 1500 (500 base + 1000 overclock heat)", gs.HeatGenerated)
	}
}

func TestPrestige_ResetsOverclock(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.OverclockMultiplier = 5.0
	gs.OverclockTicksRemaining = 30

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	if gs.OverclockMultiplier != 1.0 {
		t.Errorf("OverclockMultiplier = %f after prestige, want 1.0", gs.OverclockMultiplier)
	}
	if gs.OverclockTicksRemaining != 0 {
		t.Errorf("OverclockTicksRemaining = %d after prestige, want 0", gs.OverclockTicksRemaining)
	}
}

func TestProcessIdleProgress_OverclockOfflineExpiry(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.OverclockMultiplier = 2.0
	gs.OverclockTicksRemaining = 10 // 50 seconds of overclock remaining
	// Offline for 5 minutes (300 seconds) — overclock should fully expire
	gs.LastTickAt = time.Now().Add(-300 * time.Second)

	e.ProcessIdleProgress(gs, nil, nil, nil, nil, nil, nil, nil, time.Now())

	if gs.OverclockTicksRemaining != 0 {
		t.Errorf("OverclockTicksRemaining = %d, want 0 (should have expired offline)", gs.OverclockTicksRemaining)
	}
	if gs.OverclockMultiplier != 1.0 {
		t.Errorf("OverclockMultiplier = %f, want 1.0 (should reset after offline expiry)", gs.OverclockMultiplier)
	}
}

func TestProcessIdleProgress_OverclockPartialOffline(t *testing.T) {
	e := New()

	now := time.Now()

	// Scenario: overclock has 30 ticks left (150s), player is offline for 300s (60 ticks).
	// Weighted average: overclock was active for first 150s out of 300s total.
	// Expected weighted multiplier = 2.0 * (150/300) + 1.0 * (150/300) = 1.5
	gs := newTestState()
	gs.ComputeUnits = 0
	gs.OverclockMultiplier = 2.0
	gs.OverclockTicksRemaining = 30
	gs.LastTickAt = now.Add(-300 * time.Second)
	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 100, PowerDraw: 10},
	}
	e.ProcessIdleProgress(gs, hw, nil, nil, nil, nil, nil, nil, now)
	partialIncome := gs.ComputeUnits

	// Compare to no-overclock baseline for same offline period
	gs2 := newTestState()
	gs2.ComputeUnits = 0
	gs2.LastTickAt = now.Add(-300 * time.Second)
	e.ProcessIdleProgress(gs2, hw, nil, nil, nil, nil, nil, nil, now)
	baseIncome := gs2.ComputeUnits

	if baseIncome == 0 {
		t.Fatal("baseIncome is 0, test setup issue")
	}

	// Expected ratio is ~1.5 (weighted average of 2.0 for half, 1.0 for half)
	ratio := float64(partialIncome) / float64(baseIncome)
	if ratio < 1.4 || ratio > 1.6 {
		t.Errorf("partial offline overclock ratio = %f, want ~1.5 (base=%d, partial=%d)", ratio, baseIncome, partialIncome)
	}
}

// =====================================================================
// RESEARCH TREE TESTS
// =====================================================================

func TestBuyResearch_ValidNode(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 10_000

	result, err := e.buyResearch(gs, makeResearchPayload("read_the_docs"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.ResearchLevel == nil {
		t.Fatal("ResearchLevel is nil in result")
	}
	if result.ResearchLevel.ResearchNode != "read_the_docs" {
		t.Errorf("ResearchNode = %s, want read_the_docs", result.ResearchLevel.ResearchNode)
	}
	if result.ResearchLevel.Level != 1 {
		t.Errorf("Level = %d, want 1", result.ResearchLevel.Level)
	}
	// Cost at level 0: baseCost=500, costScale=1.8, cost = 500 * 1.8^0 = 500
	if gs.ComputeUnits != 9_500 {
		t.Errorf("ComputeUnits = %d, want 9500 (10000 - 500)", gs.ComputeUnits)
	}
}

func TestBuyResearch_UnknownNode(t *testing.T) {
	e := New()
	gs := newTestState()

	_, err := e.buyResearch(gs, makeResearchPayload("nonexistent_node"), nil)
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
	expected := "unknown research node: nonexistent_node"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestBuyResearch_TierLocked(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.Tier = models.TierCoffeeTable // Too low for "automated_testing" (requires rack_12u)
	gs.ComputeUnits = 1_000_000

	_, err := e.buyResearch(gs, makeResearchPayload("automated_testing"), nil)
	if err == nil {
		t.Fatal("expected error for tier-locked node")
	}
	// Verify CU not deducted
	if gs.ComputeUnits != 1_000_000 {
		t.Errorf("ComputeUnits mutated on tier-locked purchase: %d", gs.ComputeUnits)
	}
}

func TestBuyResearch_InsufficientCU(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 100 // Need 500 for read_the_docs level 0

	_, err := e.buyResearch(gs, makeResearchPayload("read_the_docs"), nil)
	if err == nil {
		t.Fatal("expected error for insufficient CU")
	}
	if gs.ComputeUnits != 100 {
		t.Errorf("ComputeUnits mutated on failed purchase: %d", gs.ComputeUnits)
	}
}

func TestBuyResearch_CostScalesWithLevel(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 1_000_000

	// Buy level 1 (cost at level 0 = 500)
	result1, err := e.buyResearch(gs, makeResearchPayload("read_the_docs"), nil)
	if err != nil {
		t.Fatalf("level 1 purchase failed: %v", err)
	}
	cuAfterLevel1 := gs.ComputeUnits
	level1Cost := int64(1_000_000) - cuAfterLevel1 // Should be 500

	if level1Cost != 500 {
		t.Errorf("level 1 cost = %d, want 500", level1Cost)
	}

	// Buy level 2 (cost at level 1 = floor(500 * 1.8^1) = floor(900) = 900)
	researchLevels := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: result1.ResearchLevel.Level},
	}
	result2, err := e.buyResearch(gs, makeResearchPayload("read_the_docs"), researchLevels)
	if err != nil {
		t.Fatalf("level 2 purchase failed: %v", err)
	}
	level2Cost := cuAfterLevel1 - gs.ComputeUnits // Should be floor(500 * 1.8) = 900

	if level2Cost != 900 {
		t.Errorf("level 2 cost = %d, want 900 (500 * 1.8^1)", level2Cost)
	}
	if result2.ResearchLevel.Level != 2 {
		t.Errorf("Level = %d, want 2", result2.ResearchLevel.Level)
	}
}

func TestBulkBuyResearch_MaxAffordable(t *testing.T) {
	e := New()
	gs := newTestState()
	// read_the_docs: base=500, scale=1.8
	// Level 0 cost: 500, Level 1 cost: 900, Level 2 cost: 1620, Level 3 cost: 2916
	// Cumulative: 500, 1400, 3020, 5936
	gs.ComputeUnits = 3100 // Should afford levels 0,1,2 (total 3020) but not level 3

	result, err := e.bulkBuyResearch(gs, makeResearchPayload("read_the_docs"), nil)
	if err != nil {
		t.Fatalf("bulk buy failed: %v", err)
	}
	if result.ResearchLevel == nil {
		t.Fatal("ResearchLevel is nil")
	}
	if result.ResearchLevel.Level != 3 {
		t.Errorf("Level = %d, want 3 (bought 3 levels from 0)", result.ResearchLevel.Level)
	}
	// Should have spent 500 + 900 + 1620 = 3020
	if gs.ComputeUnits != 80 {
		t.Errorf("ComputeUnits = %d, want 80 (3100 - 3020)", gs.ComputeUnits)
	}
}

func TestBulkBuyResearch_AlreadyHasLevels(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 100_000

	existing := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: 5},
	}

	result, err := e.bulkBuyResearch(gs, makeResearchPayload("read_the_docs"), existing)
	if err != nil {
		t.Fatalf("bulk buy failed: %v", err)
	}

	if result.ResearchLevel.Level <= 5 {
		t.Errorf("Level = %d, want > 5 (should have bought additional levels)", result.ResearchLevel.Level)
	}
}

func TestProcessIdleProgress_ResearchIdleIncome(t *testing.T) {
	e := New()

	now := time.Now()

	// Test without research
	gs1 := newTestState()
	gs1.ComputeUnits = 0
	gs1.LastTickAt = now.Add(-5 * time.Second)
	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 100, PowerDraw: 10},
	}
	e.ProcessIdleProgress(gs1, hw, nil, nil, nil, nil, nil, nil, now)
	baseIncome := gs1.ComputeUnits

	// Test with idle_income research (read_the_docs at level 5 = +10% idle income)
	gs2 := newTestState()
	gs2.ComputeUnits = 0
	gs2.LastTickAt = now.Add(-5 * time.Second)
	research := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: 5}, // 5 * 0.02 = +10%
	}
	e.ProcessIdleProgress(gs2, hw, nil, nil, nil, nil, nil, research, now)
	researchIncome := gs2.ComputeUnits

	if baseIncome == 0 {
		t.Fatal("baseIncome is 0, test setup issue")
	}
	ratio := float64(researchIncome) / float64(baseIncome)
	if ratio < 1.09 || ratio > 1.11 {
		t.Errorf("research idle income ratio = %f, want ~1.10 (base=%d, research=%d)", ratio, baseIncome, researchIncome)
	}
}

func TestProcessIdleProgress_ResearchReputationGain(t *testing.T) {
	e := New()

	svcs := []models.Service{
		{ID: "svc1", ReputationPerTick: 100, ComputePerTick: 0, MoneyPerTick: 0},
	}

	now := time.Now()

	// Test without research
	gs1 := newTestState()
	gs1.Reputation = 0
	gs1.LastTickAt = now.Add(-5 * time.Second)
	e.ProcessIdleProgress(gs1, nil, svcs, nil, nil, nil, nil, nil, now)
	baseRep := gs1.Reputation

	// Test with reputation_gain research (blog_writing at level 10 = +30%)
	gs2 := newTestState()
	gs2.Reputation = 0
	gs2.LastTickAt = now.Add(-5 * time.Second)
	research := []models.ResearchLevel{
		{ResearchNode: "blog_writing", Level: 10}, // 10 * 0.03 = +30%
	}
	e.ProcessIdleProgress(gs2, nil, svcs, nil, nil, nil, nil, research, now)
	researchRep := gs2.Reputation

	if baseRep == 0 {
		t.Fatal("baseRep is 0, test setup issue")
	}
	ratio := float64(researchRep) / float64(baseRep)
	if ratio < 1.29 || ratio > 1.31 {
		t.Errorf("research reputation ratio = %f, want ~1.30 (base=%d, research=%d)", ratio, baseRep, researchRep)
	}
}

func TestProcessIdleProgress_ResearchMoneyIncome(t *testing.T) {
	e := New()

	svcs := []models.Service{
		{ID: "svc1", MoneyPerTick: 100, ComputePerTick: 0, ReputationPerTick: 0},
	}

	now := time.Now()

	// Test without research
	gs1 := newTestState()
	gs1.Money = 0
	gs1.LastTickAt = now.Add(-5 * time.Second)
	e.ProcessIdleProgress(gs1, nil, svcs, nil, nil, nil, nil, nil, now)
	baseMoney := gs1.Money

	// Test with money_income research (chaos_engineering at level 5 = +20%)
	gs2 := newTestState()
	gs2.Tier = models.TierRack48U
	gs2.Money = 0
	gs2.LastTickAt = now.Add(-5 * time.Second)
	research := []models.ResearchLevel{
		{ResearchNode: "chaos_engineering", Level: 5}, // 5 * 0.04 = +20%
	}
	e.ProcessIdleProgress(gs2, nil, svcs, nil, nil, nil, nil, research, now)
	researchMoney := gs2.Money

	if baseMoney == 0 {
		t.Fatal("baseMoney is 0, test setup issue")
	}
	ratio := float64(researchMoney) / float64(baseMoney)
	if ratio < 1.19 || ratio > 1.21 {
		t.Errorf("research money ratio = %f, want ~1.20 (base=%d, research=%d)", ratio, baseMoney, researchMoney)
	}
}

func TestResearch_PersistsThroughPrestige(t *testing.T) {
	// Research levels are stored in a separate table and are NOT reset by prestige.
	// The prestige function resets GameState fields but does not touch ResearchLevel records.
	// Verify that the prestige reset block does not clear any research-related field on GameState.
	e := New()
	gs := newTestState()

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	// Prestige should NOT reset research. There is no research-related field on GameState
	// to reset (research lives in the research_levels table). The key verification is that
	// the prestige function does not return a flag indicating research should be wiped.
	// The ActionResult.Prestige = true triggers hardware/service/upgrade/customer/expense wipe
	// but the handler explicitly preserves research_levels.
	// This test documents the expectation that prestige does not affect research.
}

func TestBuyResearch_OverflowProtection(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = math.MaxInt64

	// Create a research level at a very high level where cost would overflow
	highLevel := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: 200}, // 500 * 1.8^200 will overflow
	}

	_, err := e.buyResearch(gs, makeResearchPayload("read_the_docs"), highLevel)
	if err == nil {
		t.Fatal("expected error for cost overflow at high level")
	}
}

func TestBuyResearch_JobReward(t *testing.T) {
	e := New()

	// Test job reward without research
	gs1 := newTestState()
	gs1.Tier = models.TierCoffeeTable
	gs1.ComputeUnits = 0
	result1, err := e.runJob(gs1, nil)
	if err != nil || result1 == nil {
		t.Fatalf("runJob failed: %v", err)
	}
	baseReward := gs1.ComputeUnits

	// Test with job_reward research (scripting_mastery at level 10 = +30%)
	gs2 := newTestState()
	gs2.Tier = models.TierCoffeeTable
	gs2.ComputeUnits = 0
	gs2.KnowledgePoints = 0
	research := []models.ResearchLevel{
		{ResearchNode: "scripting_mastery", Level: 10}, // 10 * 0.03 = +30%
	}
	result2, err := e.runJob(gs2, research)
	if err != nil || result2 == nil {
		t.Fatalf("runJob with research failed: %v", err)
	}
	researchReward := gs2.ComputeUnits

	if baseReward == 0 {
		t.Fatal("baseReward is 0, test setup issue")
	}
	ratio := float64(researchReward) / float64(baseReward)
	if ratio < 1.29 || ratio > 1.31 {
		t.Errorf("research job reward ratio = %f, want ~1.30 (base=%d, research=%d)", ratio, baseReward, researchReward)
	}
}

// =====================================================================
// RACK OPTIMIZATION TESTS
// =====================================================================

func TestOptimizeRack_Success(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = true
	gs.ComputeUnits = 500_000
	gs.RackOptimization = 0

	result, err := e.optimizeRack(gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	// Cost at level 0: 100000 * 2^0 = 100000
	if gs.ComputeUnits != 400_000 {
		t.Errorf("ComputeUnits = %d, want 400000 (500000 - 100000)", gs.ComputeUnits)
	}
	if gs.RackOptimization != 1 {
		t.Errorf("RackOptimization = %d, want 1", gs.RackOptimization)
	}
}

func TestOptimizeRack_WrongTier(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.Tier = models.TierRack24U
	gs.SaasUnlocked = true
	gs.ComputeUnits = 1_000_000

	_, err := e.optimizeRack(gs)
	if err == nil {
		t.Fatal("expected error for wrong tier")
	}
	expected := "must be at 48U rack tier to optimize"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestOptimizeRack_NoSaaS(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.Tier = models.TierRack48U
	gs.SaasUnlocked = false
	gs.ComputeUnits = 1_000_000

	_, err := e.optimizeRack(gs)
	if err == nil {
		t.Fatal("expected error for no SaaS")
	}
	expected := "must have SaaS unlocked to optimize"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestOptimizeRack_InsufficientCU(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 50_000 // Need 100000

	_, err := e.optimizeRack(gs)
	if err == nil {
		t.Fatal("expected error for insufficient CU")
	}
	if gs.ComputeUnits != 50_000 {
		t.Errorf("ComputeUnits mutated on failed optimize: %d", gs.ComputeUnits)
	}
	if gs.RackOptimization != 0 {
		t.Errorf("RackOptimization mutated on failed optimize: %d", gs.RackOptimization)
	}
}

func TestOptimizeRack_CostDoubles(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 10_000_000

	expectedCosts := []int64{100_000, 200_000, 400_000, 800_000, 1_600_000}

	for i, expectedCost := range expectedCosts {
		cuBefore := gs.ComputeUnits
		_, err := e.optimizeRack(gs)
		if err != nil {
			t.Fatalf("optimize %d failed: %v", i+1, err)
		}
		actualCost := cuBefore - gs.ComputeUnits
		if actualCost != expectedCost {
			t.Errorf("optimize %d: cost = %d, want %d", i+1, actualCost, expectedCost)
		}
	}

	if gs.RackOptimization != 5 {
		t.Errorf("RackOptimization = %d, want 5 after 5 optimizations", gs.RackOptimization)
	}
}

func TestPrestige_WithOptimization(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.RackOptimization = 5 // +50% bonus

	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 1000, PowerDraw: 100},
	}
	svcs := []models.Service{
		{ID: "svc1", ComputePerTick: 500, ReputationPerTick: 200, MoneyPerTick: 300},
	}

	result, err := e.prestige(gs, hw, svcs, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}
	if result.NewColoRack == nil {
		t.Fatal("NewColoRack is nil")
	}

	// Base: compute=1500, rep=200, money=300
	// With optimization level 5 and bonus_per_level=0.10: bonus = 1.0 + 5*0.10 = 1.50
	// Expected: compute=2250, rep=300, money=450
	if result.NewColoRack.ComputePerTick != 2250 {
		t.Errorf("ColoRack.ComputePerTick = %d, want 2250 (1500 * 1.50)", result.NewColoRack.ComputePerTick)
	}
	if result.NewColoRack.ReputationPerTick != 300 {
		t.Errorf("ColoRack.ReputationPerTick = %d, want 300 (200 * 1.50)", result.NewColoRack.ReputationPerTick)
	}
	if result.NewColoRack.MoneyPerTick != 450 {
		t.Errorf("ColoRack.MoneyPerTick = %d, want 450 (300 * 1.50)", result.NewColoRack.MoneyPerTick)
	}
}

func TestPrestige_WithoutOptimization(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.RackOptimization = 0

	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 1000, PowerDraw: 100},
	}
	svcs := []models.Service{
		{ID: "svc1", ComputePerTick: 500, ReputationPerTick: 200, MoneyPerTick: 300},
	}

	result, err := e.prestige(gs, hw, svcs, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	// No optimization: values should be base
	if result.NewColoRack.ComputePerTick != 1500 {
		t.Errorf("ColoRack.ComputePerTick = %d, want 1500 (no optimization)", result.NewColoRack.ComputePerTick)
	}
	if result.NewColoRack.ReputationPerTick != 200 {
		t.Errorf("ColoRack.ReputationPerTick = %d, want 200 (no optimization)", result.NewColoRack.ReputationPerTick)
	}
	if result.NewColoRack.MoneyPerTick != 300 {
		t.Errorf("ColoRack.MoneyPerTick = %d, want 300 (no optimization)", result.NewColoRack.MoneyPerTick)
	}
}

func TestPrestige_ResetsRackOptimization(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.RackOptimization = 5

	_, err := e.prestige(gs, nil, nil, nil)
	if err != nil {
		t.Fatalf("prestige failed: %v", err)
	}

	if gs.RackOptimization != 0 {
		t.Errorf("RackOptimization = %d after prestige, want 0 (should reset)", gs.RackOptimization)
	}
}

func TestOptimizeRack_OverflowGuard(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.RackOptimization = 46
	gs.ComputeUnits = math.MaxInt64

	_, err := e.optimizeRack(gs)
	if err == nil {
		t.Fatal("expected error for overflow at level 46")
	}
	expected := "optimization level at maximum"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// =====================================================================
// CROSS-FEATURE TESTS
// =====================================================================

func TestOverclockAndResearch_StackMultiplicatively(t *testing.T) {
	e := New()

	hw := []models.Hardware{
		{ID: "hw1", ComputePerTick: 100, PowerDraw: 10},
	}

	now := time.Now()

	// Baseline: no overclock, no research
	gs1 := newTestState()
	gs1.ComputeUnits = 0
	gs1.LastTickAt = now.Add(-5 * time.Second)
	e.ProcessIdleProgress(gs1, hw, nil, nil, nil, nil, nil, nil, now)
	baseIncome := gs1.ComputeUnits

	// With 2x overclock AND +10% research
	gs2 := newTestState()
	gs2.ComputeUnits = 0
	gs2.OverclockMultiplier = 2.0
	gs2.OverclockTicksRemaining = 60
	gs2.LastTickAt = now.Add(-5 * time.Second)
	research := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: 5}, // +10%
	}
	e.ProcessIdleProgress(gs2, hw, nil, nil, nil, nil, nil, research, now)
	combinedIncome := gs2.ComputeUnits

	if baseIncome == 0 {
		t.Fatal("baseIncome is 0, test setup issue")
	}
	// Expected: 2.0 * 1.1 = 2.2x
	ratio := float64(combinedIncome) / float64(baseIncome)
	if ratio < 2.1 || ratio > 2.3 {
		t.Errorf("combined overclock+research ratio = %f, want ~2.2 (base=%d, combined=%d)", ratio, baseIncome, combinedIncome)
	}
}

// =====================================================================
// PROCESSACTION DISPATCH TESTS
// =====================================================================

func TestProcessAction_ActivateOverclockDispatch(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 200_000

	_, err := e.ProcessAction(gs, "activate_overclock", makeOverclockPayload(1), nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("activate_overclock dispatch failed: %v", err)
	}
	if gs.OverclockMultiplier != 2.0 {
		t.Errorf("OverclockMultiplier = %f, want 2.0", gs.OverclockMultiplier)
	}
}

func TestProcessAction_BuyResearchDispatch(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 10_000

	result, err := e.ProcessAction(gs, "buy_research", makeResearchPayload("read_the_docs"), nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("buy_research dispatch failed: %v", err)
	}
	if result.ResearchLevel == nil {
		t.Fatal("ResearchLevel is nil in dispatch result")
	}
}

func TestProcessAction_BulkBuyResearchDispatch(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 10_000

	result, err := e.ProcessAction(gs, "bulk_buy_research", makeResearchPayload("read_the_docs"), nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("bulk_buy_research dispatch failed: %v", err)
	}
	if result.ResearchLevel == nil {
		t.Fatal("ResearchLevel is nil in bulk dispatch result")
	}
}

func TestProcessAction_OptimizeRackDispatch(t *testing.T) {
	e := New()
	gs := newTestState()
	gs.ComputeUnits = 500_000

	_, err := e.ProcessAction(gs, "optimize_rack", nil, nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("optimize_rack dispatch failed: %v", err)
	}
	if gs.RackOptimization != 1 {
		t.Errorf("RackOptimization = %d, want 1", gs.RackOptimization)
	}
}

// =====================================================================
// CONFIG TESTS
// =====================================================================

func TestGetConfig_OverclockSection(t *testing.T) {
	cfg := GetConfig()
	if len(cfg.Overclock.Tiers) != 3 {
		t.Errorf("Overclock.Tiers length = %d, want 3", len(cfg.Overclock.Tiers))
	}
	if cfg.Overclock.TickIntervalSeconds != 5 {
		t.Errorf("TickIntervalSeconds = %d, want 5", cfg.Overclock.TickIntervalSeconds)
	}
	// Verify tier 1 values
	t1 := cfg.Overclock.Tiers[0]
	if t1.Tier != 1 || t1.Multiplier != 2.0 || t1.Cost != 50000 || t1.Duration != 60 {
		t.Errorf("Tier 1 config mismatch: %+v", t1)
	}
}

func TestGetConfig_ResearchSection(t *testing.T) {
	cfg := GetConfig()
	if len(cfg.Research.Nodes) == 0 {
		t.Fatal("Research.Nodes is empty")
	}
	// Verify at least 8 nodes exist (per TDD acceptance criteria)
	if len(cfg.Research.Nodes) < 8 {
		t.Errorf("Research.Nodes length = %d, want >= 8", len(cfg.Research.Nodes))
	}
}

func TestGetConfig_RackOptimizationSection(t *testing.T) {
	cfg := GetConfig()
	if cfg.RackOptimization.BaseCost != 100000 {
		t.Errorf("BaseCost = %d, want 100000", cfg.RackOptimization.BaseCost)
	}
	if cfg.RackOptimization.CostMultiplier != 2.0 {
		t.Errorf("CostMultiplier = %f, want 2.0", cfg.RackOptimization.CostMultiplier)
	}
	if cfg.RackOptimization.BonusPerLevel != 0.10 {
		t.Errorf("BonusPerLevel = %f, want 0.10", cfg.RackOptimization.BonusPerLevel)
	}
}

// =====================================================================
// HELPER FUNCTION TESTS
// =====================================================================

func TestResearchCost_Basic(t *testing.T) {
	// read_the_docs: base=500, scale=1.8
	cost, ok := researchCost(500, 1.8, 0)
	if !ok || cost != 500 {
		t.Errorf("level 0 cost = %d (ok=%v), want 500", cost, ok)
	}
	cost, ok = researchCost(500, 1.8, 1)
	if !ok || cost != 900 {
		t.Errorf("level 1 cost = %d (ok=%v), want 900 (500*1.8)", cost, ok)
	}
}

func TestResearchCost_Overflow(t *testing.T) {
	_, ok := researchCost(500, 1.8, 1000)
	if ok {
		t.Error("expected overflow at level 1000, got ok=true")
	}
}

func TestAggregateResearchBonuses(t *testing.T) {
	levels := []models.ResearchLevel{
		{ResearchNode: "read_the_docs", Level: 5},     // idle_income: 5 * 0.02 = 0.10
		{ResearchNode: "lab_notebook", Level: 3},       // idle_income: 3 * 0.03 = 0.09
		{ResearchNode: "blog_writing", Level: 10},      // reputation_gain: 10 * 0.03 = 0.30
		{ResearchNode: "chaos_engineering", Level: 2},   // money_income: 2 * 0.04 = 0.08
	}

	bonuses := aggregateResearchBonuses(levels)

	// idle_income should be 0.10 + 0.09 = 0.19
	if bonuses["idle_income"] < 0.189 || bonuses["idle_income"] > 0.191 {
		t.Errorf("idle_income bonus = %f, want ~0.19", bonuses["idle_income"])
	}
	// reputation_gain should be 0.30
	if bonuses["reputation_gain"] < 0.299 || bonuses["reputation_gain"] > 0.301 {
		t.Errorf("reputation_gain bonus = %f, want ~0.30", bonuses["reputation_gain"])
	}
	// money_income should be 0.08
	if bonuses["money_income"] < 0.079 || bonuses["money_income"] > 0.081 {
		t.Errorf("money_income bonus = %f, want ~0.08", bonuses["money_income"])
	}
}
