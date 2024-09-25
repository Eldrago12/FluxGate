package limiter

import (
	"math"
	"sync"
	"time"

	"github.com/Eldrago12/FluxGate/internal/utils"
	"github.com/rcrowley/go-metrics"
	"github.com/shirou/gopsutil/v3/cpu"
)

type DynamicLimiterMetrics struct {
	CPUUsage       float64
	RequestLatency float64
	ErrorRate      float64
	RequestRate    float64
}

type DynamicLimiter struct {
	baseRate       float64
	baseBucketSize float64
	tokens         float64
	lastRefill     time.Time
	metrics        *DynamicLimiterMetrics
	latencyTimer   metrics.Timer
	errorMeter     metrics.Meter
	requestMeter   metrics.Meter
	mu             sync.Mutex
}

func NewDynamicLimiter(baseRate, baseBucketSize float64) *DynamicLimiter {
	return &DynamicLimiter{
		baseRate:       baseRate,
		baseBucketSize: baseBucketSize,
		tokens:         baseBucketSize,
		lastRefill:     time.Now(),
		metrics:        &DynamicLimiterMetrics{},
		latencyTimer:   metrics.NewTimer(),
		errorMeter:     metrics.NewMeter(),
		requestMeter:   metrics.NewMeter(),
	}
}

func (dl *DynamicLimiter) Allow() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	now := time.Now()
	dl.refill(now)

	if dl.tokens >= 1 {
		dl.tokens--
		return true
	}
	return false
}

func (dl *DynamicLimiter) refill(now time.Time) {
	elapsed := now.Sub(dl.lastRefill).Seconds()
	dl.tokens = utils.Min(dl.baseBucketSize, dl.tokens+elapsed*dl.baseRate)
	dl.lastRefill = now
}

func (dl *DynamicLimiter) RecordMetrics(latency time.Duration, isError bool) {
	dl.latencyTimer.Update(latency)
	dl.requestMeter.Mark(1)
	if isError {
		dl.errorMeter.Mark(1)
	}
}

func (dl *DynamicLimiter) UpdateMetrics() {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		dl.metrics.CPUUsage = cpuPercent[0]
	}

	dl.metrics.RequestLatency = float64(dl.latencyTimer.Percentile(0.95))
	dl.metrics.ErrorRate = dl.errorMeter.Rate1()
	dl.metrics.RequestRate = dl.requestMeter.Rate1()
}

func (dl *DynamicLimiter) CalculateNewLimits() (float64, float64) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	hourOfDay := float64(time.Now().Hour())
	timeOfDayFactor := 1.0 + 0.5*math.Sin((hourOfDay-12)*math.Pi/12)
	cpuFactor := math.Max(0.5, 1.0-dl.metrics.CPUUsage/100.0)
	normalizedLatency := math.Min(dl.metrics.RequestLatency, 1000) / 1000
	latencyFactor := math.Max(0.5, 1.0-normalizedLatency)
	errorFactor := math.Max(0.5, 1.0-dl.metrics.ErrorRate)

	adjustmentFactor := timeOfDayFactor * cpuFactor * latencyFactor * errorFactor
	newRate := dl.baseRate * adjustmentFactor

	requestRateFactor := math.Max(1.0, dl.metrics.RequestRate/dl.baseRate)
	newBucketSize := dl.baseBucketSize * requestRateFactor * adjustmentFactor

	newRate = math.Max(dl.baseRate*0.5, math.Min(dl.baseRate*1.5, newRate))
	newBucketSize = math.Max(dl.baseBucketSize*0.5, math.Min(dl.baseBucketSize*1.5, newBucketSize))

	// Update the current rate and bucket size
	dl.baseRate = newRate
	dl.baseBucketSize = newBucketSize

	return newRate, newBucketSize
}

func (dl *DynamicLimiter) Start(updateInterval time.Duration) {
	ticker := time.NewTicker(updateInterval)
	go func() {
		for range ticker.C {
			dl.UpdateMetrics()
			newRate, newBucketSize := dl.CalculateNewLimits()
			// Log the new limits
			println("New rate:", newRate, "New bucket size:", newBucketSize)
		}
	}()
}
