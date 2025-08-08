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
	"github.com/ryosan-470/tokenbucket/storage/memory"

	"github.com/ryosan-470/tokenbucket/benchmark"
	benchmarkstorage "github.com/ryosan-470/tokenbucket/benchmark/storage"
)

type Config struct {
	Scenario     string
	Concurrency  int
	Duration     time.Duration
	Capacity     int64
	FillRate     int64
	Dimensions   int
	BackendType  string
	ProviderType string

	dimension string
}

func main() {
	cfg := parseFlags()

	log.Printf("Starting 60-second benchmark with config: %+v", cfg)

	// Setup provider based on backend
	provider, err := getProvider(cfg)
	if err != nil {
		log.Fatalf("Failed to get provider: %v", err)
	}
	ctx := context.Background()

	if err := provider.Setup(ctx); err != nil {
		log.Fatalf("Failed to setup provider: %v", err)
	}

	defer func() {
		if err := provider.Cleanup(ctx); err != nil {
			log.Printf("Warning: Failed to cleanup provider: %v", err)
		}
	}()

	// Run benchmark based on scenario
	var report benchmark.Report

	switch cfg.Scenario {
	case "single":
		report, err = runSingleDimensionTest(provider, cfg)
	case "multi":
		report, err = runMultiDimensionTest(provider, cfg)
	default:
		log.Fatalf("Unknown scenario: %s", cfg.Scenario)
	}

	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}

	// Print results
	printReport(report, cfg)
}

func parseFlags() Config {
	var cfg Config

	flag.StringVar(&cfg.Scenario, "scenario", "single", "Benchmark scenario (single, multi)")
	flag.IntVar(&cfg.Concurrency, "concurrency", 10, "Number of concurrent goroutines")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.Int64Var(&cfg.Capacity, "capacity", 1000, "Token bucket capacity")
	flag.Int64Var(&cfg.FillRate, "fill-rate", 100, "Token fill rate per second")
	flag.IntVar(&cfg.Dimensions, "dimensions", 10, "Number of dimensions for multi-dimension test")
	flag.StringVar(&cfg.BackendType, "backend-type", "memory", "Backend type (dynamodb, memory)")
	flag.StringVar(&cfg.ProviderType, "provider-type", "local", "Provider type (local, aws)")

	flag.Parse()

	return cfg
}

func runSingleDimensionTest(provider benchmarkstorage.Provider, cfg Config) (benchmark.Report, error) {
	dimension := "load-test-single"
	cfg.dimension = dimension

	backend, err := getBackend(cfg, provider)
	if err != nil {
		return benchmark.Report{}, err
	}

	bucket, err := provider.CreateBucket(cfg.Capacity, cfg.FillRate, dimension, backend)
	if err != nil {
		return benchmark.Report{}, fmt.Errorf("failed to create bucket: %w", err)
	}

	metrics := benchmark.NewMetrics()

	// Run load test
	return runLoadTest(bucket, metrics, cfg)
}

func runMultiDimensionTest(provider benchmarkstorage.Provider, cfg Config) (benchmark.Report, error) {
	// Create buckets for each dimension
	buckets := make([]*tokenbucket.Bucket, cfg.Dimensions)
	for i := 0; i < cfg.Dimensions; i++ {
		dimension := fmt.Sprintf("load-test-multi-%d", i)
		cfg.dimension = dimension
		backend, err := getBackend(cfg, provider)
		if err != nil {
			return benchmark.Report{}, err
		}
		bucket, err := provider.CreateBucket(cfg.Capacity, cfg.FillRate, dimension, backend)
		if err != nil {
			return benchmark.Report{}, fmt.Errorf("failed to create bucket %d: %w", i, err)
		}
		buckets[i] = bucket
	}

	metrics := benchmark.NewMetrics()

	// Run multi-dimensional load test
	return runMultiDimensionalLoadTest(buckets, metrics, cfg)
}

func runLoadTest(bucket *tokenbucket.Bucket, metrics *benchmark.Metrics, cfg Config) (benchmark.Report, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+1*time.Minute)
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
				log.Printf(
					"Snapshot: %7d ops, %8.2f ops/sec, Avg Latency: %7.2fms, Success: %7d, Failed: %7d",
					snapshot.TotalOperations, snapshot.OpsPerSecond, snapshot.AvgLatencyMs, snapshot.SuccessfulTakes, snapshot.FailedTakes,
				)
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
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+1*time.Minute)
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
				log.Printf(
					"Snapshot: %7d ops, %8.2f ops/sec, Avg Latency: %7.2fms, Success: %7d, Failed: %7d",
					snapshot.TotalOperations, snapshot.OpsPerSecond, snapshot.AvgLatencyMs, snapshot.SuccessfulTakes, snapshot.FailedTakes,
				)
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

func printReport(report benchmark.Report, cfg Config) {
	fmt.Printf("\n=== Benchmark Results ===\n")
	fmt.Printf("Scenario: %s\n", cfg.Scenario)
	fmt.Printf("Concurrency: %d\n", cfg.Concurrency)
	fmt.Printf("Duration: %v\n", cfg.Duration)
	fmt.Printf("Backend Type: %s\n", cfg.BackendType)

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

// getProvider returns the appropriate storage provider based on configuration
func getProvider(cfg Config) (benchmarkstorage.Provider, error) {
	switch cfg.ProviderType {
	case "local":
		return benchmarkstorage.GetLocalProvider(), nil
	case "aws":
		return benchmarkstorage.GetAWSProvider(), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.ProviderType)
	}
}

func getBackend(cfg Config, provider benchmarkstorage.Provider) (tokenbucket.TokenBucketStateRepository, error) {
	switch cfg.BackendType {
	case "custom":
		ddbCfg := provider.CreateBucketConfig(cfg.dimension)
		return ddbCfg.NewCustomBackend(context.Background(), cfg.dimension)
	case "memory":
		return memory.NewBackend(), nil
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.BackendType)
	}
}
