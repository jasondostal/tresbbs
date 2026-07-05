package auth

import (
	"sync"
	"time"
)

// RateLimiter tracks login attempts per IP address.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptEntry
	maxTries int
	lockout  time.Duration
}

type attemptEntry struct {
	count    int
	lastTry  time.Time
	blocked  bool
	blockEnd time.Time
}

// NewRateLimiter creates a new rate limiter.
// maxTries is the maximum failed attempts before lockout.
// lockout is the duration of the lockout.
func NewRateLimiter(maxTries int, lockout time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string]*attemptEntry),
		maxTries: maxTries,
		lockout:  lockout,
	}
}

// IsBlocked checks if an IP address is currently blocked.
func (r *RateLimiter) IsBlocked(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.attempts[ip]
	if !exists {
		return false
	}

	// Check if block has expired
	if entry.blocked && time.Now().After(entry.blockEnd) {
		entry.blocked = false
		entry.count = 0
		return false
	}

	return entry.blocked
}

// RecordFailure records a failed login attempt.
func (r *RateLimiter) RecordFailure(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.attempts[ip]
	if !exists {
		entry = &attemptEntry{}
		r.attempts[ip] = entry
	}

	entry.count++
	entry.lastTry = time.Now()

	// Block if too many attempts
	if entry.count >= r.maxTries {
		entry.blocked = true
		entry.blockEnd = time.Now().Add(r.lockout)
	}
}

// RecordSuccess resets the failure counter for an IP.
func (r *RateLimiter) RecordSuccess(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, ip)
}

// GetRemainingAttempts returns how many attempts remain before lockout.
func (r *RateLimiter) GetRemainingAttempts(ip string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.attempts[ip]
	if !exists {
		return r.maxTries
	}

	remaining := r.maxTries - entry.count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}
