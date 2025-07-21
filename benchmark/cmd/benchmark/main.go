package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryosan-470/tokenbucket"
	benchmark "github.com/ryosan-470/tokenbucket/benchmark"
)

type Config struct {
	Scenario    string
	Concurrency int
	Duration    time.Duration
	Capacity    int64
	FillRate    int64
	Dimensions  int
	WithLock    bool
	OutputFile  string
}

func main() {
	cfg := parseFlags()

	log.Printf("Starting 60-second benchmark with config: %+v", cfg)

	// Setup harness
	harness := benchmark.GetHarness()
	ctx := context.Background()

	if err := harness.Setup(ctx); err != nil {
		log.Fatalf("Failed to setup harness: %v", err)
	}

	defer func() {
		if err := harness.Cleanup(ctx); err != nil {
			log.Printf("Warning: Failed to cleanup harness: %v", err)
		}
	}()

	// Run benchmark based on scenario
	var report benchmark.Report
	var err error

	switch cfg.Scenario {
	case "single":
		report, err = runSingleDimensionTest(harness, cfg)
	case "multi":
		report, err = runMultiDimensionTest(harness, cfg)
	case "lock-comparison":
		report, err = runLockComparisonTest(harness, cfg)
	default:
		log.Fatalf("Unknown scenario: %s", cfg.Scenario)
	}

	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}

	// Print results
	printReport(report, cfg)

	// Save results if output file specified
	if cfg.OutputFile != "" {
		if err := saveReport(report, cfg); err != nil {
			log.Printf("Failed to save report: %v", err)
		}
	}
}

func parseFlags() Config {
	var cfg Config

	flag.StringVar(&cfg.Scenario, "scenario", "single", "Benchmark scenario (single, multi, lock-comparison)")
	flag.IntVar(&cfg.Concurrency, "concurrency", 10, "Number of concurrent goroutines")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.Int64Var(&cfg.Capacity, "capacity", 1000, "Token bucket capacity")
	flag.Int64Var(&cfg.FillRate, "fill-rate", 100, "Token fill rate per second")
	flag.IntVar(&cfg.Dimensions, "dimensions", 10, "Number of dimensions for multi-dimension test")
	flag.BoolVar(&cfg.WithLock, "with-lock", true, "Use distributed locking")
	flag.StringVar(&cfg.OutputFile, "output", "", "Output file for results (optional)")

	flag.Parse()

	return cfg
}

func runSingleDimensionTest(harness *benchmark.BenchmarkHarness, cfg Config) (benchmark.Report, error) {
	var opts []tokenbucket.Option
	if !cfg.WithLock {
		opts = append(opts, tokenbucket.WithoutLock())
	}

	bucket, err := harness.CreateBucket(cfg.Capacity, cfg.FillRate, "load-test-single", opts...)
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to create bucket: %w", err)
	}

	metrics := benchmark.NewMetrics()

	// Run load test
	return runLoadTest(bucket, metrics, cfg)
}

func runMultiDimensionTest(harness *benchmark.BenchmarkHarness, cfg Config) (benchmark.Report, error) {
	var opts []tokenbucket.Option
	if !cfg.WithLock {
		opts = append(opts, tokenbucket.WithoutLock())
	}

	// Create buckets for each dimension
	buckets := make([]*tokenbucket.Bucket, cfg.Dimensions)
	for i := 0; i < cfg.Dimensions; i++ {
		bucket, err := harness.CreateBucket(cfg.Capacity/5, cfg.FillRate/5, fmt.Sprintf("load-test-multi-%d", i), opts...)
		if err != nil {
			return benchmark.Report{}, fmt.Errorf("failed to create bucket %d: %w", i, err)
		}
		buckets[i] = bucket
	}

	metrics := benchmark.NewMetrics()

	// Run multi-dimensional load test
	return runMultiDimensionalLoadTest(buckets, metrics, cfg)
}

func runLockComparisonTest(harness *benchmark.BenchmarkHarness, cfg Config) (benchmark.Report, error) {
	// Run both with and without lock
	log.Println("Running with lock...")
	bucketWithLock, err := harness.CreateBucket(cfg.Capacity, cfg.FillRate, "load-test-with-lock")
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to create bucket with lock: %w", err)
	}

	metricsWithLock := benchmark.NewMetrics()
	reportWithLock, err := runLoadTest(bucketWithLock, metricsWithLock, cfg)
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to run with-lock test: %w", err)
	}

	log.Println("Running without lock...")
	bucketWithoutLock, err := harness.CreateBucket(cfg.Capacity, cfg.FillRate, "load-test-without-lock", tokenbucket.WithoutLock())
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to create bucket without lock: %w", err)
	}

	metricsWithoutLock := benchmark.NewMetrics()
	reportWithoutLock, err := runLoadTest(bucketWithoutLock, metricsWithoutLock, cfg)
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to run without-lock test: %w", err)
	}

	// Print comparison
	printLockComparison(reportWithLock, reportWithoutLock)

	// Return the with-lock report as primary result
	return reportWithLock, nil
}

func runLoadTest(bucket *tokenbucket.Bucket, metrics *benchmark.Metrics, cfg Config) (benchmark.Report, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup

	// Start periodic snapshot collection
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				snapshot := metrics.TakeSnapshot()
				log.Printf("Snapshot: %d ops, %.2f ops/sec, Success: %d, Failed: %d",
					snapshot.TotalOperations, snapshot.OpsPerSecond,
					snapshot.SuccessfulTakes, snapshot.FailedTakes)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start worker goroutines
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					start := time.Now()
					err := bucket.Take(ctx)
					latency := time.Since(start)

					metrics.RecordTake(latency, err == nil)

					// Small delay to prevent overwhelming the system
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()

	return metrics.GenerateReport(), nil
}

func runMultiDimensionalLoadTest(buckets []*tokenbucket.Bucket, metrics *benchmark.Metrics, cfg Config) (benchmark.Report, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	var dimensionIndex int64

	// Start periodic snapshot collection
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				snapshot := metrics.TakeSnapshot()
				log.Printf("Multi-dimension Snapshot: %d ops, %.2f ops/sec, Success: %d, Failed: %d",
					snapshot.TotalOperations, snapshot.OpsPerSecond,
					snapshot.SuccessfulTakes, snapshot.FailedTakes)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start worker goroutines
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Round-robin across dimensions
					idx := atomic.AddInt64(&dimensionIndex, 1) % int64(len(buckets))
					bucket := buckets[idx]

					start := time.Now()
					err := bucket.Take(ctx)
					latency := time.Since(start)

					metrics.RecordTake(latency, err == nil)

					// Slightly longer delay for multi-dimensional test
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()

	return metrics.GenerateReport(), nil
}

func printReport(report benchmark.Report, cfg Config) {
	fmt.Printf("\n=== Benchmark Results ===\n")
	fmt.Printf("Scenario: %s\n", cfg.Scenario)
	fmt.Printf("Concurrency: %d\n", cfg.Concurrency)
	fmt.Printf("Duration: %v\n", cfg.Duration)
	fmt.Printf("With Lock: %v\n", cfg.WithLock)
	fmt.Printf("\n")

	fmt.Printf("Total Operations: %d\n", report.TotalOperations)
	fmt.Printf("Successful Takes: %d\n", report.SuccessfulTakes)
	fmt.Printf("Failed Takes: %d\n", report.FailedTakes)
	fmt.Printf("Success Rate: %.2f%%\n", report.SuccessRate)
	fmt.Printf("Average Ops/Second: %.2f\n", report.AvgOpsPerSecond)
	fmt.Printf("\n")

	fmt.Printf("Latency Statistics:\n")
	fmt.Printf("  Mean: %v\n", report.LatencyMean)
	fmt.Printf("  P50:  %v\n", report.LatencyP50)
	fmt.Printf("  P95:  %v\n", report.LatencyP95)
	fmt.Printf("  P99:  %v\n", report.LatencyP99)
	fmt.Printf("  Max:  %v\n", report.LatencyMax)
	fmt.Printf("  Min:  %v\n", report.LatencyMin)
}

func printLockComparison(withLock, withoutLock benchmark.Report) {
	fmt.Printf("\n=== Lock Comparison ===\n")
	fmt.Printf("%-20s | %-15s | %-15s | %s\n", "Metric", "With Lock", "Without Lock", "Difference")
	fmt.Printf("%s\n", "--------------------------------------------------------------------------------")

	fmt.Printf("%-20s | %-15.2f | %-15.2f | %.2fx\n",
		"Ops/Second", withLock.AvgOpsPerSecond, withoutLock.AvgOpsPerSecond,
		withoutLock.AvgOpsPerSecond/withLock.AvgOpsPerSecond)

	fmt.Printf("%-20s | %-15.2f | %-15.2f | %.2fx\n",
		"Success Rate (%)", withLock.SuccessRate, withoutLock.SuccessRate,
		withoutLock.SuccessRate/withLock.SuccessRate)

	fmt.Printf("%-20s | %-15s | %-15s | %.2fx\n",
		"P95 Latency", withLock.LatencyP95, withoutLock.LatencyP95,
		float64(withLock.LatencyP95)/float64(withoutLock.LatencyP95))
}

func saveReport(report benchmark.Report, cfg Config) error {
	// TODO: Implement JSON/CSV output
	log.Printf("Saving report to %s (not implemented yet)", cfg.OutputFile)
	return nil
}
