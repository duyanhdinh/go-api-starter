package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/internal/platform/config"
	"server/internal/platform/http/response"
)

type tokenBucket struct {
	tokens    float64
	updatedAt time.Time
	lastSeen  time.Time
}

type rateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]tokenBucket
	requestsPerSec float64
	burst          float64
	maxClients     int
	entryTTL       time.Duration
	nextCleanup    time.Time
	now            func() time.Time
}

func newRateLimiter(configuration config.RateLimitConfig) *rateLimiter {
	return &rateLimiter{
		buckets:        make(map[string]tokenBucket),
		requestsPerSec: configuration.RequestsPerSecond,
		burst:          float64(configuration.Burst),
		maxClients:     configuration.MaxClients,
		entryTTL:       configuration.EntryTTL,
		now:            time.Now,
	}
}

func RateLimit(next http.Handler, configuration config.RateLimitConfig, applicationLogger *slog.Logger, exempt func(*http.Request) bool) http.Handler {
	if !configuration.Enabled {
		return next
	}
	return withRateLimiter(next, newRateLimiter(configuration), applicationLogger, exempt)
}

func withRateLimiter(next http.Handler, limiter *rateLimiter, applicationLogger *slog.Logger, exempt func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if exempt != nil && exempt(request) {
			next.ServeHTTP(writer, request)
			return
		}
		allowed, retryAfter := limiter.allow(clientIP(request.RemoteAddr))
		if !allowed {
			writer.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
			response.Error(writer, request, applicationLogger, http.StatusTooManyRequests, "rate_limited", "Rate limit exceeded")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func clientIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	if address := net.ParseIP(strings.Trim(host, "[]")); address != nil {
		return address.String()
	}
	return "unknown"
}

func (limiter *rateLimiter) allow(client string) (bool, int64) {
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if limiter.nextCleanup.IsZero() || !now.Before(limiter.nextCleanup) {
		limiter.removeExpired(now)
	}
	bucket, exists := limiter.buckets[client]
	if exists && now.Sub(bucket.lastSeen) >= limiter.entryTTL {
		delete(limiter.buckets, client)
		exists = false
	}
	if !exists {
		if len(limiter.buckets) >= limiter.maxClients {
			limiter.removeExpired(now)
			if len(limiter.buckets) >= limiter.maxClients {
				return false, limiter.capacityRetryAfter(now)
			}
		}
		limiter.buckets[client] = tokenBucket{tokens: limiter.burst - 1, updatedAt: now, lastSeen: now}
		return true, 0
	}

	elapsed := now.Sub(bucket.updatedAt).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(limiter.burst, bucket.tokens+elapsed*limiter.requestsPerSec)
		bucket.updatedAt = now
	}
	bucket.lastSeen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		limiter.buckets[client] = bucket
		return true, 0
	}
	limiter.buckets[client] = bucket
	return false, retryAfterSeconds((1 - bucket.tokens) / limiter.requestsPerSec)
}

func (limiter *rateLimiter) capacityRetryAfter(now time.Time) int64 {
	var earliest time.Time
	for _, bucket := range limiter.buckets {
		if earliest.IsZero() || bucket.lastSeen.Before(earliest) {
			earliest = bucket.lastSeen
		}
	}
	return retryAfterSeconds(limiter.entryTTL.Seconds() - now.Sub(earliest).Seconds())
}

func (limiter *rateLimiter) removeExpired(now time.Time) {
	for client, bucket := range limiter.buckets {
		if now.Sub(bucket.lastSeen) >= limiter.entryTTL {
			delete(limiter.buckets, client)
		}
	}
	cleanupInterval := limiter.entryTTL / 2
	if cleanupInterval > time.Minute {
		cleanupInterval = time.Minute
	}
	if cleanupInterval <= 0 {
		cleanupInterval = time.Nanosecond
	}
	limiter.nextCleanup = now.Add(cleanupInterval)
}

func retryAfterSeconds(seconds float64) int64 {
	if math.IsInf(seconds, 0) || seconds >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	if seconds <= 1 {
		return 1
	}
	return int64(math.Ceil(seconds))
}
