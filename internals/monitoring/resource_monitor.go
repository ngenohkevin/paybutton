package monitoring

import (
	"context"
	"log"
	"runtime"
	"sync"
	"time"
)

// ResourceMonitor tracks and limits resource usage
type ResourceMonitor struct {
	mu               sync.RWMutex
	activeGoroutines int
	maxGoroutines    int
	maxMemoryMB      int
	cleanupInterval  time.Duration
	stopChan         chan struct{}
}

var (
	monitor     *ResourceMonitor
	monitorOnce sync.Once
)

// InitResourceMonitor initializes the resource monitor
func InitResourceMonitor(maxGoroutines, maxMemoryMB int, cleanupInterval time.Duration) *ResourceMonitor {
	monitorOnce.Do(func() {
		monitor = &ResourceMonitor{
			maxGoroutines:   maxGoroutines,
			maxMemoryMB:     maxMemoryMB,
			cleanupInterval: cleanupInterval,
			stopChan:        make(chan struct{}),
		}
		go monitor.runCleanup()
	})
	return monitor
}

// GetResourceMonitor returns the singleton resource monitor
func GetResourceMonitor() *ResourceMonitor {
	if monitor == nil {
		// Initialize with defaults if not already initialized
		return InitResourceMonitor(50, 400, 5*time.Minute)
	}
	return monitor
}

// CanStartGoroutine checks if we can start a new goroutine
func (rm *ResourceMonitor) CanStartGoroutine() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.activeGoroutines < rm.maxGoroutines
}

// StartGoroutine increments the goroutine counter if allowed
func (rm *ResourceMonitor) StartGoroutine() bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.activeGoroutines >= rm.maxGoroutines {
		log.Printf("Warning: Maximum goroutines limit reached (%d/%d)", rm.activeGoroutines, rm.maxGoroutines)
		return false
	}

	rm.activeGoroutines++
	return true
}

// EndGoroutine decrements the goroutine counter
func (rm *ResourceMonitor) EndGoroutine() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.activeGoroutines > 0 {
		rm.activeGoroutines--
	}
}

// RunWithLimit runs a function in a goroutine if resource limits allow
func (rm *ResourceMonitor) RunWithLimit(fn func()) bool {
	if !rm.StartGoroutine() {
		return false
	}

	go func() {
		defer rm.EndGoroutine()
		fn()
	}()

	return true
}

// runCleanup periodically runs garbage collection and checks memory usage
func (rm *ResourceMonitor) runCleanup() {
	ticker := time.NewTicker(rm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.performCleanup()
		case <-rm.stopChan:
			return
		}
	}
}

// performCleanup runs garbage collection and logs memory stats
func (rm *ResourceMonitor) performCleanup() {
	// Force garbage collection
	runtime.GC()
	runtime.Gosched()

	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := m.Alloc / 1024 / 1024
	sysMB := m.Sys / 1024 / 1024

	log.Printf("Resource Monitor - Memory: Alloc=%dMB, Sys=%dMB, Goroutines=%d/%d",
		allocMB, sysMB, runtime.NumGoroutine(), rm.maxGoroutines)

	// If memory usage is too high, force more aggressive GC
	if allocMB > uint64(rm.maxMemoryMB) {
		log.Printf("Warning: Memory usage exceeds limit (%dMB > %dMB), forcing aggressive GC", allocMB, rm.maxMemoryMB)
		runtime.GC()
		runtime.GC() // Run twice for more aggressive collection
	}
}

// GetStats returns current resource statistics
func (rm *ResourceMonitor) GetStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"active_goroutines": rm.activeGoroutines,
		"max_goroutines":    rm.maxGoroutines,
		"system_goroutines": runtime.NumGoroutine(),
		"memory_alloc_mb":   m.Alloc / 1024 / 1024,
		"memory_sys_mb":     m.Sys / 1024 / 1024,
		"memory_limit_mb":   rm.maxMemoryMB,
		"gc_runs":           m.NumGC,
	}
}

// Shutdown stops the resource monitor
func (rm *ResourceMonitor) Shutdown() {
	close(rm.stopChan)
}

// WithTimeout runs a function with a timeout
func WithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
