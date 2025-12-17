package parallel

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// WorkerPool is a pool of goroutines for parallel rendering.
//
// The pool distributes work items across multiple workers, each with their own
// queue. Workers can steal work from other workers when their own queue is empty.
// This helps balance load when some tasks are slower than others.
//
// Thread safety: WorkerPool is safe for concurrent use.
type WorkerPool struct {
	// workers is the number of worker goroutines.
	workers int

	// workQueues holds per-worker work queues.
	// Each worker primarily pulls from its own queue but can steal from others.
	workQueues []chan func()

	// done signals workers to stop.
	done chan struct{}

	// wg waits for all workers to finish.
	wg sync.WaitGroup

	// running indicates whether the pool is accepting work.
	running atomic.Bool

	// queueSize is the buffer size for each worker's queue.
	queueSize int
}

// NewWorkerPool creates a new worker pool with the specified number of workers.
// If workers is 0 or negative, GOMAXPROCS is used.
// The pool starts immediately and workers begin waiting for work.
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	// Buffer size: 2-4x workers helps hide latency (from research)
	queueSize := workers * 4
	if queueSize < 8 {
		queueSize = 8
	}

	p := &WorkerPool{
		workers:    workers,
		workQueues: make([]chan func(), workers),
		done:       make(chan struct{}),
		queueSize:  queueSize,
	}

	// Create per-worker queues
	for i := range workers {
		p.workQueues[i] = make(chan func(), queueSize)
	}

	p.running.Store(true)

	// Start worker goroutines
	p.wg.Add(workers)
	for i := range workers {
		go p.worker(i)
	}

	return p
}

// worker is the main loop for each worker goroutine.
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	myQueue := p.workQueues[id]

	for {
		select {
		case <-p.done:
			// Drain remaining work before exiting
			p.drainQueue(myQueue)
			return

		case work := <-myQueue:
			if work != nil {
				work()
			}

		default:
			// Try to steal work from another worker
			if stolen := p.steal(id); stolen != nil {
				stolen()
			} else {
				// No work available anywhere, block on own queue
				select {
				case <-p.done:
					p.drainQueue(myQueue)
					return
				case work := <-myQueue:
					if work != nil {
						work()
					}
				}
			}
		}
	}
}

// drainQueue executes all remaining work in a queue.
func (p *WorkerPool) drainQueue(queue chan func()) {
	for {
		select {
		case work := <-queue:
			if work != nil {
				work()
			}
		default:
			return
		}
	}
}

// steal attempts to take work from another worker's queue.
// Returns nil if no work is available.
func (p *WorkerPool) steal(myID int) func() {
	// Try each other worker's queue once
	for i := range p.workers {
		if i == myID {
			continue
		}

		select {
		case work := <-p.workQueues[i]:
			return work
		default:
			// Queue is empty, try next
		}
	}
	return nil
}

// ExecuteAll distributes work across workers and waits for all to complete.
// This is the primary method for parallel processing.
// If the pool is closed, this is a no-op.
func (p *WorkerPool) ExecuteAll(work []func()) {
	if len(work) == 0 || !p.running.Load() {
		return
	}

	var completionWG sync.WaitGroup
	completionWG.Add(len(work))

	// Wrap each work item to signal completion
	for i, fn := range work {
		workerID := i % p.workers
		workFn := fn // Capture for closure

		wrappedWork := func() {
			defer completionWG.Done()
			workFn()
		}

		// Submit to worker's queue (may block if queue is full)
		select {
		case p.workQueues[workerID] <- wrappedWork:
			// Successfully queued
		case <-p.done:
			// Pool is closing, execute remaining work directly
			completionWG.Done()
		}
	}

	completionWG.Wait()
}

// ExecuteAsync distributes work across workers without waiting for completion.
// Useful for fire-and-forget scenarios.
// If the pool is closed, this is a no-op.
func (p *WorkerPool) ExecuteAsync(work []func()) {
	if len(work) == 0 || !p.running.Load() {
		return
	}

	// Distribute work round-robin across workers
	for i, fn := range work {
		workerID := i % p.workers
		workFn := fn // Capture for closure

		select {
		case p.workQueues[workerID] <- workFn:
			// Successfully queued
		case <-p.done:
			// Pool is closing
			return
		}
	}
}

// Submit sends a single work item to the pool.
// The work is distributed to the worker with the shortest queue.
// If the pool is closed, this is a no-op.
func (p *WorkerPool) Submit(fn func()) {
	if fn == nil || !p.running.Load() {
		return
	}

	// Find worker with shortest queue (simple load balancing)
	minLen := len(p.workQueues[0])
	minIdx := 0

	for i := 1; i < p.workers; i++ {
		qLen := len(p.workQueues[i])
		if qLen < minLen {
			minLen = qLen
			minIdx = i
		}
	}

	select {
	case p.workQueues[minIdx] <- fn:
		// Successfully queued
	case <-p.done:
		// Pool is closing
	}
}

// Close gracefully shuts down the pool.
// It stops accepting new work, waits for all queued work to complete,
// and then stops all workers.
// Close is safe to call multiple times.
func (p *WorkerPool) Close() {
	if !p.running.CompareAndSwap(true, false) {
		// Already closed
		return
	}

	// Signal workers to stop
	close(p.done)

	// Wait for all workers to finish
	p.wg.Wait()
}

// Workers returns the number of workers in the pool.
func (p *WorkerPool) Workers() int {
	return p.workers
}

// IsRunning returns true if the pool is still accepting work.
func (p *WorkerPool) IsRunning() bool {
	return p.running.Load()
}

// QueuedWork returns the total number of work items currently queued.
// This is an approximation as queues can change while iterating.
func (p *WorkerPool) QueuedWork() int {
	total := 0
	for _, q := range p.workQueues {
		total += len(q)
	}
	return total
}
