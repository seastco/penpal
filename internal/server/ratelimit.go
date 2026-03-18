package server

import (
	"sync"
	"time"
)

const (
	maxConnsPerIP     = 50
	maxRegsPerIP      = 10
	regWindowDuration = 1 * time.Hour
	cleanupInterval   = 10 * time.Minute
)

// IPRateLimiter tracks per-IP connection and registration counts.
type IPRateLimiter struct {
	mu       sync.Mutex
	counters map[string]*ipCounter
}

type ipCounter struct {
	conns      int       // current concurrent connections
	regs       int       // registrations in current window
	regWindow  time.Time // start of current registration window
	lastActive time.Time
}

// NewIPRateLimiter creates a new rate limiter.
func NewIPRateLimiter() *IPRateLimiter {
	return &IPRateLimiter{
		counters: make(map[string]*ipCounter),
	}
}

// StartCleanup runs periodic eviction of stale entries. Call as a goroutine.
func (l *IPRateLimiter) StartCleanup(done <-chan struct{}) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

func (l *IPRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-regWindowDuration)
	for ip, c := range l.counters {
		if c.conns <= 0 && c.lastActive.Before(cutoff) {
			delete(l.counters, ip)
		}
	}
}

func (l *IPRateLimiter) get(ip string) *ipCounter {
	c, ok := l.counters[ip]
	if !ok {
		c = &ipCounter{regWindow: time.Now(), lastActive: time.Now()}
		l.counters[ip] = c
	}
	c.lastActive = time.Now()
	return c
}

// AllowConn returns true if the IP has not exceeded the connection limit.
// On success, increments the connection count (caller must call ReleaseConn).
func (l *IPRateLimiter) AllowConn(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.get(ip)
	if c.conns >= maxConnsPerIP {
		return false
	}
	c.conns++
	return true
}

// ReleaseConn decrements the connection count for an IP.
func (l *IPRateLimiter) ReleaseConn(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if c, ok := l.counters[ip]; ok {
		c.conns--
	}
}

// AllowRegistration returns true if the IP has not exceeded the registration limit.
func (l *IPRateLimiter) AllowRegistration(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.get(ip)
	now := time.Now()

	// Reset window if expired
	if now.Sub(c.regWindow) > regWindowDuration {
		c.regs = 0
		c.regWindow = now
	}

	if c.regs >= maxRegsPerIP {
		return false
	}
	c.regs++
	return true
}
