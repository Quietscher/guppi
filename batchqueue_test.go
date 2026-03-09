package main

import "testing"

func TestNewBatchQueue(t *testing.T) {
	paths := []string{"/a", "/b", "/c", "/d", "/e"}
	q := newBatchQueue(paths, 3)

	if len(q.pending) != 5 {
		t.Errorf("expected 5 pending, got %d", len(q.pending))
	}
	if q.maxWorkers != 3 {
		t.Errorf("expected maxWorkers 3, got %d", q.maxWorkers)
	}
}

func TestStartReturnsUpToMaxWorkers(t *testing.T) {
	paths := []string{"/a", "/b", "/c", "/d", "/e"}
	q := newBatchQueue(paths, 3)

	initial := q.Start()
	if len(initial) != 3 {
		t.Errorf("expected 3 initial paths, got %d", len(initial))
	}
	if initial[0] != "/a" || initial[1] != "/b" || initial[2] != "/c" {
		t.Errorf("unexpected initial paths: %v", initial)
	}
	if q.active != 3 {
		t.Errorf("expected 3 active, got %d", q.active)
	}
}

func TestStartWithFewerThanMaxWorkers(t *testing.T) {
	paths := []string{"/a", "/b"}
	q := newBatchQueue(paths, 5)

	initial := q.Start()
	if len(initial) != 2 {
		t.Errorf("expected 2 initial paths, got %d", len(initial))
	}
	if q.active != 2 {
		t.Errorf("expected 2 active, got %d", q.active)
	}
}

func TestNextReturnsNextPath(t *testing.T) {
	paths := []string{"/a", "/b", "/c", "/d", "/e"}
	q := newBatchQueue(paths, 2)
	q.Start() // starts /a, /b

	next, ok := q.Next()
	if !ok {
		t.Error("expected next to be available")
	}
	if next != "/c" {
		t.Errorf("expected /c, got %s", next)
	}
	if q.active != 2 {
		t.Errorf("expected 2 active (one finished, one started), got %d", q.active)
	}
}

func TestNextReturnsFalseWhenQueueEmpty(t *testing.T) {
	paths := []string{"/a", "/b"}
	q := newBatchQueue(paths, 2)
	q.Start() // starts both

	next, ok := q.Next()
	if ok {
		t.Errorf("expected no next, got %s", next)
	}
	if q.active != 1 {
		t.Errorf("expected 1 active after completion, got %d", q.active)
	}
}

func TestDrainQueue(t *testing.T) {
	paths := []string{"/a", "/b", "/c", "/d", "/e"}
	q := newBatchQueue(paths, 2)
	q.Start() // /a, /b active

	// Complete /a, get /c
	next, ok := q.Next()
	if !ok || next != "/c" {
		t.Errorf("expected /c, got %s (ok=%v)", next, ok)
	}

	// Complete /b, get /d
	next, ok = q.Next()
	if !ok || next != "/d" {
		t.Errorf("expected /d, got %s (ok=%v)", next, ok)
	}

	// Complete /c, get /e
	next, ok = q.Next()
	if !ok || next != "/e" {
		t.Errorf("expected /e, got %s (ok=%v)", next, ok)
	}

	// Complete /d, queue empty
	_, ok = q.Next()
	if ok {
		t.Error("expected no next after queue drained")
	}

	// Complete /e
	_, ok = q.Next()
	if ok {
		t.Error("expected no next")
	}

	if !q.Done() {
		t.Error("expected queue to be done")
	}
}

func TestDoneWhenAllComplete(t *testing.T) {
	paths := []string{"/a"}
	q := newBatchQueue(paths, 5)
	q.Start()

	if q.Done() {
		t.Error("should not be done while active")
	}

	q.Next() // complete /a
	if !q.Done() {
		t.Error("should be done after all complete")
	}
}

func TestEmptyQueue(t *testing.T) {
	q := newBatchQueue(nil, 5)
	initial := q.Start()
	if len(initial) != 0 {
		t.Errorf("expected 0 initial, got %d", len(initial))
	}
	if !q.Done() {
		t.Error("empty queue should be done immediately")
	}
}
