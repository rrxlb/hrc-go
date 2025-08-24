package utils

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

// DiscordMetrics tracks Discord API performance
type DiscordMetrics struct {
	TotalRequests    int64
	SuccessfulReqs   int64
	FailedRequests   int64
	TimeoutRequests  int64
	AverageLatency   int64 // in milliseconds
	MaxLatency       int64
	MinLatency       int64
	lastLatencySum   int64
	lastLatencyCount int64
}

// DiscordOptimizer manages optimized Discord API interactions
type DiscordOptimizer struct {
	metrics     *DiscordMetrics
	rateLimiter *RateLimiter
	mutex       sync.RWMutex
}

// RateLimiter implements basic rate limiting for Discord API
type RateLimiter struct {
	requests chan struct{}
	reset    time.Time
	mutex    sync.RWMutex
}

// Global Discord optimizer instance
var DiscordOpt = &DiscordOptimizer{
	metrics: &DiscordMetrics{
		MinLatency: 999999, // Initialize with high value
	},
	rateLimiter: &RateLimiter{
		requests: make(chan struct{}, 50), // 50 requests per second bucket
	},
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		requests: make(chan struct{}, requestsPerSecond),
	}
}

// Wait waits for rate limit clearance
func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case rl.requests <- struct{}{}:
		// Schedule request release after 1 second
		go func() {
			time.Sleep(1 * time.Second)
			<-rl.requests
		}()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// OptimizedInteractionRespond performs an optimized interaction response
func (do *DiscordOptimizer) OptimizedInteractionRespond(
	session *discordgo.Session,
	interaction *discordgo.Interaction,
	response *discordgo.InteractionResponse,
	timeout time.Duration,
) error {
	startTime := time.Now()

	// Increment total requests counter
	atomic.AddInt64(&do.metrics.TotalRequests, 1)

	// Apply rate limiting
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := do.rateLimiter.Wait(ctx); err != nil {
		atomic.AddInt64(&do.metrics.TimeoutRequests, 1)
		return fmt.Errorf("rate limit timeout: %w", err)
	}

	// Execute Discord API call with timeout
	type result struct {
		err error
	}
	resultChan := make(chan result, 1)

	go func() {
		err := session.InteractionRespond(interaction, response)
		resultChan <- result{err}
	}()

	select {
	case res := <-resultChan:
		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		do.recordLatency(latency)

		if res.err != nil {
			atomic.AddInt64(&do.metrics.FailedRequests, 1)
			return res.err
		}

		atomic.AddInt64(&do.metrics.SuccessfulReqs, 1)
		return nil

	case <-ctx.Done():
		atomic.AddInt64(&do.metrics.TimeoutRequests, 1)
		return fmt.Errorf("Discord API timeout after %v", timeout)
	}
}

// OptimizedInteractionResponseEdit performs an optimized interaction response edit
func (do *DiscordOptimizer) OptimizedInteractionResponseEdit(
	session *discordgo.Session,
	interaction *discordgo.Interaction,
	edit *discordgo.WebhookEdit,
	timeout time.Duration,
) (*discordgo.Message, error) {
	startTime := time.Now()

	// Increment total requests counter
	atomic.AddInt64(&do.metrics.TotalRequests, 1)

	// Apply rate limiting
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := do.rateLimiter.Wait(ctx); err != nil {
		atomic.AddInt64(&do.metrics.TimeoutRequests, 1)
		return nil, fmt.Errorf("rate limit timeout: %w", err)
	}

	// Execute Discord API call with timeout
	type result struct {
		msg *discordgo.Message
		err error
	}
	resultChan := make(chan result, 1)

	go func() {
		msg, err := session.InteractionResponseEdit(interaction, edit)
		resultChan <- result{msg, err}
	}()

	select {
	case res := <-resultChan:
		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		do.recordLatency(latency)

		if res.err != nil {
			atomic.AddInt64(&do.metrics.FailedRequests, 1)
			return nil, res.err
		}

		atomic.AddInt64(&do.metrics.SuccessfulReqs, 1)
		return res.msg, nil

	case <-ctx.Done():
		atomic.AddInt64(&do.metrics.TimeoutRequests, 1)
		return nil, fmt.Errorf("Discord API timeout after %v", timeout)
	}
}

// recordLatency records latency metrics thread-safely
func (do *DiscordOptimizer) recordLatency(latencyMs int64) {
	do.mutex.Lock()
	defer do.mutex.Unlock()

	// Update min/max latency
	if latencyMs < do.metrics.MinLatency {
		do.metrics.MinLatency = latencyMs
	}
	if latencyMs > do.metrics.MaxLatency {
		do.metrics.MaxLatency = latencyMs
	}

	// Update average latency calculation
	do.metrics.lastLatencySum += latencyMs
	do.metrics.lastLatencyCount++

	// Calculate rolling average
	if do.metrics.lastLatencyCount > 0 {
		do.metrics.AverageLatency = do.metrics.lastLatencySum / do.metrics.lastLatencyCount
	}
}

// GetMetrics returns current Discord API metrics
func (do *DiscordOptimizer) GetMetrics() DiscordMetrics {
	do.mutex.RLock()
	defer do.mutex.RUnlock()
	return *do.metrics
}

// ResetMetrics resets all metrics counters
func (do *DiscordOptimizer) ResetMetrics() {
	do.mutex.Lock()
	defer do.mutex.Unlock()

	do.metrics = &DiscordMetrics{
		MinLatency: 999999,
	}
}

// LogPerformanceMetrics logs current performance metrics
func (do *DiscordOptimizer) LogPerformanceMetrics() {
	metrics := do.GetMetrics()

	if metrics.TotalRequests > 0 {
		successRate := float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
		timeoutRate := float64(metrics.TimeoutRequests) / float64(metrics.TotalRequests) * 100

		BotLogf("DISCORD_PERF",
			"Discord API Performance: Total=%d, Success=%.1f%%, Timeout=%.1f%%, AvgLatency=%dms, MaxLatency=%dms, MinLatency=%dms",
			metrics.TotalRequests,
			successRate,
			timeoutRate,
			metrics.AverageLatency,
			metrics.MaxLatency,
			metrics.MinLatency,
		)
	}
}

// HealthCheck performs a basic Discord API health check
func (do *DiscordOptimizer) HealthCheck(session *discordgo.Session) error {
	// Simple health check by getting bot user info
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultChan := make(chan error, 1)
	go func() {
		_, err := session.User("@me")
		resultChan <- err
	}()

	select {
	case err := <-resultChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("Discord API health check timeout")
	}
}

// StartPerformanceMonitoring starts background performance monitoring
func (do *DiscordOptimizer) StartPerformanceMonitoring(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			do.LogPerformanceMetrics()
		}
	}()
}

// ConnectionPool manages Discord session connections for better performance
type ConnectionPool struct {
	sessions []*discordgo.Session
	current  int
	mutex    sync.RWMutex
}

// NewConnectionPool creates a pool of Discord sessions for load balancing
func NewConnectionPool(token string, poolSize int) (*ConnectionPool, error) {
	pool := &ConnectionPool{
		sessions: make([]*discordgo.Session, poolSize),
	}

	// Create multiple sessions for load balancing
	for i := 0; i < poolSize; i++ {
		session, err := discordgo.New("Bot " + token)
		if err != nil {
			return nil, fmt.Errorf("failed to create session %d: %w", i, err)
		}

		// Configure session for optimal performance
		session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages
		session.StateEnabled = false // Disable state caching for better memory usage

		pool.sessions[i] = session
	}

	return pool, nil
}

// GetSession returns the next available session in round-robin fashion
func (cp *ConnectionPool) GetSession() *discordgo.Session {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	session := cp.sessions[cp.current]
	cp.current = (cp.current + 1) % len(cp.sessions)
	return session
}

// BulkInteractionResponse handles multiple interactions efficiently
func (do *DiscordOptimizer) BulkInteractionResponse(
	responses []struct {
		Session     *discordgo.Session
		Interaction *discordgo.Interaction
		Response    *discordgo.InteractionResponse
	},
	timeout time.Duration,
) []error {
	errors := make([]error, len(responses))

	// Process responses concurrently with controlled concurrency
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent requests
	var wg sync.WaitGroup

	for i, resp := range responses {
		wg.Add(1)
		go func(index int, r struct {
			Session     *discordgo.Session
			Interaction *discordgo.Interaction
			Response    *discordgo.InteractionResponse
		}) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			errors[index] = do.OptimizedInteractionRespond(r.Session, r.Interaction, r.Response, timeout)
		}(i, resp)
	}

	wg.Wait()
	return errors
}
