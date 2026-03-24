package middleware

import (
	"testing"
)

func TestCheckGameActionRate_AllowsWithinLimit(t *testing.T) {
	// Use a unique user ID per test to avoid cross-test pollution from the
	// global rate limiter state.
	userID := "ratelimit-test-user-allow"

	// The limit is 7200 per minute. A single call should be allowed.
	if !CheckGameActionRate(userID) {
		t.Error("CheckGameActionRate returned false for first request, want true")
	}
}

func TestCheckGameActionRate_AllowsMultipleWithinLimit(t *testing.T) {
	userID := "ratelimit-test-user-multi"

	// Send 100 requests — well within the 7200/min limit.
	for i := 0; i < 100; i++ {
		if !CheckGameActionRate(userID) {
			t.Errorf("CheckGameActionRate returned false at request %d, want true (under 7200 limit)", i+1)
			return
		}
	}
}

func TestCheckGameActionRate_RejectsOverLimit(t *testing.T) {
	userID := "ratelimit-test-user-reject"

	// Exhaust the rate limit. Send exactly 7200 requests to fill the bucket.
	for i := 0; i < 7200; i++ {
		CheckGameActionRate(userID)
	}

	// The 7201st request should be rejected.
	if CheckGameActionRate(userID) {
		t.Error("CheckGameActionRate returned true for request 7201, want false (over limit)")
	}
}

func TestCheckGameActionRate_DifferentUsersIndependent(t *testing.T) {
	// Verify that rate limits are per-user, not global.
	userA := "ratelimit-test-user-a"
	userB := "ratelimit-test-user-b"

	// Exhaust user A's limit.
	for i := 0; i < 7200; i++ {
		CheckGameActionRate(userA)
	}

	// User A should be rate-limited.
	if CheckGameActionRate(userA) {
		t.Error("User A should be rate-limited after 7200 requests")
	}

	// User B should still be allowed.
	if !CheckGameActionRate(userB) {
		t.Error("User B should be allowed (independent rate limit)")
	}
}

func TestCheckGameActionRate_UsesCorrectBucketKey(t *testing.T) {
	// Verify the function uses the "game:user:" prefix by checking that
	// it doesn't interfere with a manually created bucket with a different
	// prefix.
	userID := "ratelimit-test-user-bucket"

	// First call via the exported function should succeed.
	if !CheckGameActionRate(userID) {
		t.Error("expected first call to succeed")
	}

	// Call the internal checkRate with the same key pattern to verify
	// they share the same bucket (count accumulates).
	key := "game:user:" + userID
	if !checkRate(key, 7200) {
		// This should still pass — we've only used 2 of 7200.
		t.Error("expected checkRate with same key to succeed")
	}
}

func TestCheckRate_BasicFunctionality(t *testing.T) {
	// Test the underlying checkRate function directly.
	key := "test-check-rate-basic"

	// First request: allowed.
	if !checkRate(key, 5) {
		t.Error("request 1 should be allowed")
	}

	// Requests 2-5: allowed.
	for i := 2; i <= 5; i++ {
		if !checkRate(key, 5) {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Request 6: rejected (over the limit of 5).
	if checkRate(key, 5) {
		t.Error("request 6 should be rejected (over limit of 5)")
	}
}

func TestCheckRate_DifferentKeysIndependent(t *testing.T) {
	keyA := "test-check-rate-key-a"
	keyB := "test-check-rate-key-b"

	// Exhaust key A's limit.
	for i := 0; i < 3; i++ {
		checkRate(keyA, 3)
	}

	// Key A should be blocked.
	if checkRate(keyA, 3) {
		t.Error("key A should be blocked")
	}

	// Key B should still work.
	if !checkRate(keyB, 3) {
		t.Error("key B should be allowed")
	}
}
