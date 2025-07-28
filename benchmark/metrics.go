package benchmark

import (
	"sort"
	"sync"
	"time"
)

// MetricSnapshot represents a point-in-time snapshot of metrics
type MetricSnapshot struct {
	Timestamp        time.Time
	TotalOperations  int64
	SuccessfulTakes  int64
	FailedTakes      int64
	OpsPerSecond     float64
	AvgLatencyMs     float64  // Average latency per operation in milliseconds
}

// Metrics collects and analyzes benchmark performance data
type Metrics struct {
	mu sync.RWMutex
	
	// Raw data
	latencies         []time.Duration
	successCount      int64
	failureCount      int64
	startTime         time.Time
	
	// Time series data for 60-second tests
	snapshots         []MetricSnapshot
}

// NewMetrics creates a new metrics collector
func NewMetrics() *Metrics {
	return &Metrics{
		latencies: make([]time.Duration, 0, 10000),
		startTime: time.Now(),
	}
}

// RecordTake records the result of a Take() operation
func (m *Metrics) RecordTake(latency time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.latencies = append(m.latencies, latency)
	if success {
		m.successCount++
	} else {
		m.failureCount++
	}
}

// TakeSnapshot captures current metrics state
func (m *Metrics) TakeSnapshot() MetricSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	now := time.Now()
	elapsed := now.Sub(m.startTime).Seconds()
	total := m.successCount + m.failureCount
	
	var opsPerSec float64
	if elapsed > 0 {
		opsPerSec = float64(total) / elapsed
	}
	
	// Calculate average latency in milliseconds
	var avgLatencyMs float64
	if len(m.latencies) > 0 {
		var sum time.Duration
		for _, lat := range m.latencies {
			sum += lat
		}
		avgLatency := sum / time.Duration(len(m.latencies))
		avgLatencyMs = float64(avgLatency.Nanoseconds()) / 1e6 // Convert to milliseconds
	}
	
	snapshot := MetricSnapshot{
		Timestamp:        now,
		TotalOperations:  total,
		SuccessfulTakes:  m.successCount,
		FailedTakes:      m.failureCount,
		OpsPerSecond:     opsPerSec,
		AvgLatencyMs:     avgLatencyMs,
	}
	
	m.snapshots = append(m.snapshots, snapshot)
	return snapshot
}

// FinalReport generates a comprehensive metrics report
type Report struct {
	Duration          time.Duration
	TotalOperations   int64
	SuccessfulTakes   int64
	FailedTakes       int64
	SuccessRate       float64
	AvgOpsPerSecond   float64
	AvgLatencyMs      float64  // Average latency per operation in milliseconds
	
	// Latency statistics
	LatencyMean       time.Duration
	LatencyP50        time.Duration
	LatencyP95        time.Duration
	LatencyP99        time.Duration
	LatencyMax        time.Duration
	LatencyMin        time.Duration
	
	// Time series data
	Snapshots         []MetricSnapshot
}

// GenerateReport creates a final performance report
func (m *Metrics) GenerateReport() Report {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	total := m.successCount + m.failureCount
	duration := time.Since(m.startTime)
	
	var successRate float64
	if total > 0 {
		successRate = float64(m.successCount) / float64(total) * 100
	}
	
	avgOps := float64(total) / duration.Seconds()
	
	// Calculate average latency in milliseconds
	var avgLatencyMs float64
	if len(m.latencies) > 0 {
		var sum time.Duration
		for _, lat := range m.latencies {
			sum += lat
		}
		avgLatency := sum / time.Duration(len(m.latencies))
		avgLatencyMs = float64(avgLatency.Nanoseconds()) / 1e6 // Convert to milliseconds
	}
	
	// Calculate latency percentiles
	latenciesCopy := make([]time.Duration, len(m.latencies))
	copy(latenciesCopy, m.latencies)
	sort.Slice(latenciesCopy, func(i, j int) bool {
		return latenciesCopy[i] < latenciesCopy[j]
	})
	
	var mean, p50, p95, p99, max, min time.Duration
	if len(latenciesCopy) > 0 {
		// Mean
		var sum time.Duration
		for _, lat := range latenciesCopy {
			sum += lat
		}
		mean = sum / time.Duration(len(latenciesCopy))
		
		// Percentiles
		p50 = percentile(latenciesCopy, 50)
		p95 = percentile(latenciesCopy, 95)
		p99 = percentile(latenciesCopy, 99)
		max = latenciesCopy[len(latenciesCopy)-1]
		min = latenciesCopy[0]
	}
	
	// Copy snapshots
	snapshots := make([]MetricSnapshot, len(m.snapshots))
	copy(snapshots, m.snapshots)
	
	return Report{
		Duration:        duration,
		TotalOperations: total,
		SuccessfulTakes: m.successCount,
		FailedTakes:     m.failureCount,
		SuccessRate:     successRate,
		AvgOpsPerSecond: avgOps,
		AvgLatencyMs:    avgLatencyMs,
		
		LatencyMean: mean,
		LatencyP50:  p50,
		LatencyP95:  p95,
		LatencyP99:  p99,
		LatencyMax:  max,
		LatencyMin:  min,
		
		Snapshots: snapshots,
	}
}

// percentile calculates the nth percentile from a sorted slice
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	
	index := float64(len(sorted)-1) * float64(p) / 100.0
	lower := int(index)
	upper := lower + 1
	
	if upper >= len(sorted) {
		return sorted[lower]
	}
	
	weight := index - float64(lower)
	return time.Duration(float64(sorted[lower]) + weight*float64(sorted[upper]-sorted[lower]))
}

// Reset clears all collected metrics
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.latencies = m.latencies[:0]
	m.successCount = 0
	m.failureCount = 0
	m.startTime = time.Now()
	m.snapshots = m.snapshots[:0]
}