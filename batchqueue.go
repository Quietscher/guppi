package main

const maxConcurrentOps = 10

// batchQueue manages concurrent execution of operations with a limit.
// It hands out paths in batches up to maxWorkers, and as each completes,
// the next path is returned to keep concurrency at the limit.
type batchQueue struct {
	pending    []string
	maxWorkers int
	active     int
}

func newBatchQueue(paths []string, maxWorkers int) batchQueue {
	p := make([]string, len(paths))
	copy(p, paths)
	return batchQueue{
		pending:    p,
		maxWorkers: maxWorkers,
	}
}

// Start returns the initial batch of paths (up to maxWorkers).
func (q *batchQueue) Start() []string {
	count := q.maxWorkers
	if count > len(q.pending) {
		count = len(q.pending)
	}
	batch := q.pending[:count]
	q.pending = q.pending[count:]
	q.active = count
	return batch
}

// Next is called when one operation completes. It returns the next path
// to process, or false if the queue is empty.
func (q *batchQueue) Next() (string, bool) {
	q.active--
	if len(q.pending) == 0 {
		return "", false
	}
	path := q.pending[0]
	q.pending = q.pending[1:]
	q.active++
	return path, true
}

// Done returns true when all operations have completed.
func (q *batchQueue) Done() bool {
	return len(q.pending) == 0 && q.active == 0
}
