package server

import "testing"

func TestAllowConn_WithinLimit(t *testing.T) {
	l := NewIPRateLimiter()
	ip := "1.2.3.4"

	for i := 0; i < maxConnsPerIP; i++ {
		if !l.AllowConn(ip) {
			t.Fatalf("AllowConn rejected connection %d, limit is %d", i+1, maxConnsPerIP)
		}
	}
}

func TestAllowConn_Exceeded(t *testing.T) {
	l := NewIPRateLimiter()
	ip := "1.2.3.4"

	for i := 0; i < maxConnsPerIP; i++ {
		l.AllowConn(ip)
	}
	if l.AllowConn(ip) {
		t.Fatal("AllowConn should reject when at limit")
	}
}

func TestReleaseConn(t *testing.T) {
	l := NewIPRateLimiter()
	ip := "1.2.3.4"

	// Fill to capacity
	for i := 0; i < maxConnsPerIP; i++ {
		l.AllowConn(ip)
	}
	if l.AllowConn(ip) {
		t.Fatal("should be at capacity")
	}

	// Release one, should allow again
	l.ReleaseConn(ip)
	if !l.AllowConn(ip) {
		t.Fatal("AllowConn should succeed after ReleaseConn")
	}
}

func TestAllowRegistration_WithinLimit(t *testing.T) {
	l := NewIPRateLimiter()
	ip := "1.2.3.4"

	for i := 0; i < maxRegsPerIP; i++ {
		if !l.AllowRegistration(ip) {
			t.Fatalf("AllowRegistration rejected registration %d, limit is %d", i+1, maxRegsPerIP)
		}
	}
}

func TestAllowRegistration_Exceeded(t *testing.T) {
	l := NewIPRateLimiter()
	ip := "1.2.3.4"

	for i := 0; i < maxRegsPerIP; i++ {
		l.AllowRegistration(ip)
	}
	if l.AllowRegistration(ip) {
		t.Fatal("AllowRegistration should reject when at limit")
	}
}

func TestAllowConn_IndependentIPs(t *testing.T) {
	l := NewIPRateLimiter()

	// Fill one IP to capacity
	for i := 0; i < maxConnsPerIP; i++ {
		l.AllowConn("1.1.1.1")
	}

	// Different IP should still be allowed
	if !l.AllowConn("2.2.2.2") {
		t.Fatal("different IP should not be affected")
	}
}
