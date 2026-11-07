// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sync/semaphore"
)

func TestRawReservationsStayHeldUntilConsumptionEnds(t *testing.T) {
	budget := semaphore.NewWeighted(rawFetchReservation)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	consuming := make(chan struct{})
	finish := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- inOrder(ctx, 3, func(ctx context.Context, i int) (Message, error) {
			reserveResponse(ctx, 128)
			return Message{RFC822: []byte{byte(i)}}, nil
		}, func(i int, msg Message, err error) error {
			if err != nil || len(msg.RFC822) != 1 || int(msg.RFC822[0]) != i {
				return errors.New("message order changed")
			}
			if i == 0 {
				close(consuming)
				<-finish
			}
			return nil
		}, budget, rawFetchReservation)
	}()
	<-consuming
	if budget.TryAcquire(rawFetchReservation) {
		budget.Release(rawFetchReservation)
		t.Error("consuming message released its reservation too soon")
	}
	close(finish)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	assertRawBudgetReturned(t, budget)
}

func TestStoppedWalksReturnAllRawReservations(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "sink failure", true: "cancelled"}[cancelled], func(t *testing.T) {
			budget := semaphore.NewWeighted(rawFetchReservation)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sinkError := errors.New("sink failed")
			err := inOrder(ctx, 8, func(ctx context.Context, _ int) (Message, error) {
				reserveResponse(ctx, 128)
				return Message{RFC822: []byte("mail")}, nil
			}, func(_ int, _ Message, _ error) error {
				if cancelled {
					cancel()
				}
				return sinkError
			}, budget, rawFetchReservation)
			if !errors.Is(err, sinkError) {
				t.Fatalf("walk error=%v want sink error", err)
			}
			assertRawBudgetReturned(t, budget)
		})
	}
}

func assertRawBudgetReturned(t *testing.T, budget *semaphore.Weighted) {
	t.Helper()
	if !budget.TryAcquire(rawFetchReservation) {
		t.Fatal("walk leaked a raw reservation")
	}
	budget.Release(rawFetchReservation)
}

func TestAPanickingConsumerReturnsItsReservation(t *testing.T) {
	budget := semaphore.NewWeighted(rawFetchReservation)
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		if err := inOrder(t.Context(), 1, func(context.Context, int) (Message, error) {
			return Message{RFC822: []byte("mail")}, nil
		}, func(int, Message, error) error { panic("consumer failed") }, budget, rawFetchReservation); err != nil {
			t.Fatal(err)
		}
	}()
	if !panicked {
		t.Fatal("consumer panic was hidden")
	}
	assertRawBudgetReturned(t, budget)
}
