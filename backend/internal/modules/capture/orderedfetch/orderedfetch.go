// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package orderedfetch walks a provider page with a few fetches in flight. It
// hands each result to its consumer in the page's own order. Every walk in the
// process shares one memory budget.
package orderedfetch

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Walk is how a provider walks its page. It sets how many fetches run at once
// and how far they may run ahead of the consumer. It also sets how much memory
// one fetch reserves until its result has been consumed.
type Walk[T any] struct {
	Workers   int
	Lookahead int
	Budget    *semaphore.Weighted
	Reserve   int64
	// Retained is what a fetched result still holds; the rest of the
	// reservation goes back to the budget as soon as the fetch ends.
	Retained func(T) int64
	// Halts names a fetch error that ends the page, such as a rate limit. It
	// stops further fetches at once instead of when the consumer reaches it,
	// so a throttled provider is not sent the rest of the look-ahead. Nil
	// never halts.
	Halts func(error) bool
}

type ordered[T any] struct {
	v           T
	err         error
	reservation *Reservation
}

type queued struct {
	index       int
	reservation *Reservation
}

// InOrder fetches n items and consumes each in index order. A nil Budget skips
// the memory accounting. The first consume error stops the walk and is returned.
func (w Walk[T]) InOrder(
	ctx context.Context, n int,
	fetch func(ctx context.Context, i int) (T, error),
	consume func(i int, v T, err error) error,
) error {
	if n == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	slots := make([]chan ordered[T], n)
	for i := range slots {
		slots[i] = make(chan ordered[T], 1)
	}
	tokens := make(chan struct{}, w.Lookahead)
	next := make(chan queued)
	halt := &halting{stopped: make(chan struct{})}
	queuedAll := make(chan int, 1)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
		for _, slot := range slots {
			select {
			case remaining := <-slot:
				remaining.reservation.Release()
			default:
			}
		}
	}()
	wg.Go(func() { queuedAll <- w.queue(ctx, n, tokens, next, halt) })
	for range min(w.Workers, n) {
		wg.Go(func() { w.work(ctx, next, slots, halt, fetch) })
	}
	queued := n
	for i := range n {
		result, err := await(ctx, i, slots[i], queuedAll, &queued, halt)
		if err != nil {
			return err
		}
		err = consumeOne(i, result, consume)
		<-tokens
		if err != nil {
			return err
		}
	}
	return nil
}

// work fetches what the queue hands it into each item's slot. An item handed
// over as the walk halted is answered with the halting error rather than sent
// to the provider.
func (w Walk[T]) work(ctx context.Context, next <-chan queued, slots []chan ordered[T], halt *halting, fetch func(context.Context, int) (T, error)) {
	for item := range next {
		if cause := halt.cause(); cause != nil {
			slots[item.index] <- ordered[T]{err: cause, reservation: item.reservation}
			continue
		}
		v, err := fetch(item.reservation.Context(ctx), item.index)
		if err != nil && w.Halts != nil && w.Halts(err) {
			halt.stop(err)
		}
		if w.Retained != nil {
			item.reservation.Shrink(w.Retained(v))
		}
		slots[item.index] <- ordered[T]{v: v, err: err, reservation: item.reservation}
	}
}

// await answers item i's result, or why the walk stopped before it. Either the
// queue halted short of i, after the consumer let the halting error pass, or
// the walk ended.
func await[T any](ctx context.Context, i int, slot <-chan ordered[T], queuedAll <-chan int, queued *int, halt *halting) (ordered[T], error) {
	for {
		if i >= *queued {
			if cause := halt.cause(); cause != nil {
				return ordered[T]{}, cause
			}
			return ordered[T]{}, ctx.Err()
		}
		select {
		case result := <-slot:
			return result, nil
		case *queued = <-queuedAll:
			queuedAll = nil
		case <-ctx.Done():
			return ordered[T]{}, ctx.Err()
		}
	}
}

// halting is the first fetch error that ended the page.
type halting struct {
	once    sync.Once
	err     error
	stopped chan struct{}
}

func (h *halting) stop(err error) {
	h.once.Do(func() {
		h.err = err
		close(h.stopped)
	})
}

func (h *halting) cause() error {
	select {
	case <-h.stopped:
		return h.err
	default:
		return nil
	}
}

// queue hands items to the workers in index order and answers how many it
// handed over: n, unless the walk halted or ended first.
func (w Walk[T]) queue(ctx context.Context, n int, tokens chan<- struct{}, next chan<- queued, halt *halting) int {
	defer close(next)
	for i := range n {
		select {
		case tokens <- struct{}{}:
		case <-halt.stopped:
			return i
		case <-ctx.Done():
			return i
		}
		// Reserve in listing order, before a later result can occupy the budget.
		reservation, err := Reserve(ctx, w.Budget, w.Reserve)
		if err != nil {
			return i
		}
		select {
		case next <- queued{index: i, reservation: reservation}:
		case <-halt.stopped:
			reservation.Release()
			return i
		case <-ctx.Done():
			reservation.Release()
			return i
		}
	}
	return n
}

func consumeOne[T any](index int, result ordered[T], consume func(int, T, error) error) error {
	defer result.reservation.Release()
	return consume(index, result.v, result.err)
}
