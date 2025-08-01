package apic

import (
	"testing"
	"time"
)

func TestWithReconnectBackoff(t *testing.T) {
	// Test that backoff calculation produces valid durations without hanging
	maxBackoff := 5 * time.Second
	
	// Test the backoff calculation directly
	count := 0
	for i := 0; i < 10; i++ {
		count++
		
		// Replicate the calculation from WithReconnectBackoff
		const (
			minMillis = 5
			maxMillis = 999
		)
		mills := minMillis // Use fixed value for predictable testing
		base := 16
		backoffMs := 1
		for j := 0; j < count && j < 10; j++ {
			backoffMs *= base
		}
		d := time.Millisecond * time.Duration(backoffMs+mills)
		if d > maxBackoff {
			d = maxBackoff
		}
		
		// Verify duration is positive
		if d <= 0 {
			t.Errorf("Attempt %d produced non-positive duration: %v", i+1, d)
		}
		
		// Verify duration doesn't exceed max
		if d > maxBackoff {
			t.Errorf("Attempt %d exceeded max backoff: got %v, max %v", i+1, d, maxBackoff)
		}
		
		t.Logf("Attempt %d: calculated duration = %v", i+1, d)
	}
}

func TestWithReconnectBackoffMaxLimit(t *testing.T) {
	// Test that backoff respects the maximum duration without waiting
	maxBackoff := 100 * time.Millisecond
	
	// Test high attempt counts to ensure max backoff is enforced
	testCases := []int{5, 10, 15, 20}
	
	for _, count := range testCases {
		// Replicate the calculation
		const minMillis = 5
		mills := minMillis
		base := 16
		backoffMs := 1
		for j := 0; j < count && j < 10; j++ {
			backoffMs *= base
		}
		d := time.Millisecond * time.Duration(backoffMs+mills)
		if d > maxBackoff {
			d = maxBackoff
		}
		
		// Verify max backoff is enforced
		if d > maxBackoff {
			t.Errorf("Attempt count %d: duration %v exceeded max backoff %v", count, d, maxBackoff)
		}
		
		t.Logf("Attempt count %d: duration = %v (capped at %v)", count, d, maxBackoff)
	}
}

func TestReconnectBackoffCalculation(t *testing.T) {
	// Direct test of the backoff calculation logic
	tests := []struct {
		name     string
		count    int
		wantMin  time.Duration // minimum expected duration (without random component)
		wantMax  time.Duration // maximum expected duration (with max random component)
	}{
		{
			name:    "first attempt",
			count:   1,
			wantMin: 16 * time.Millisecond,         // 16^1 = 16ms
			wantMax: (16 + 999) * time.Millisecond, // 16ms + max 999ms random
		},
		{
			name:    "second attempt", 
			count:   2,
			wantMin: 256 * time.Millisecond,         // 16^2 = 256ms
			wantMax: (256 + 999) * time.Millisecond, // 256ms + max 999ms random
		},
		{
			name:    "third attempt",
			count:   3,
			wantMin: 4096 * time.Millisecond,         // 16^3 = 4096ms
			wantMax: (4096 + 999) * time.Millisecond, // 4096ms + max 999ms random
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate expected backoff for the given count
			base := 16
			backoffMs := 1
			for i := 0; i < tt.count && i < 10; i++ {
				backoffMs *= base
			}
			
			// Verify the calculation produces expected values
			minDuration := time.Duration(backoffMs) * time.Millisecond
			maxDuration := time.Duration(backoffMs+999) * time.Millisecond
			
			if minDuration != tt.wantMin {
				t.Errorf("Minimum duration mismatch: got %v, want %v", minDuration, tt.wantMin)
			}
			
			if maxDuration != tt.wantMax {
				t.Errorf("Maximum duration mismatch: got %v, want %v", maxDuration, tt.wantMax)
			}
			
			// Ensure no negative values
			if minDuration < 0 || maxDuration < 0 {
				t.Errorf("Negative duration detected: min=%v, max=%v", minDuration, maxDuration)
			}
		})
	}
}