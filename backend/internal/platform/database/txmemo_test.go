// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// memoTx stands in for a transaction only as a key; no method is called.
type memoTx struct{ pgx.Tx }

func countingRead(reads *int, value string) func() (string, error) {
	return func() (string, error) {
		*reads++
		return value, nil
	}
}

func TestATransactionReadsARememberedValueOnce(t *testing.T) {
	tx := &memoTx{}
	defer openTxMemo(tx)()
	reads := 0
	for range 3 {
		if got, err := Remember(tx, "list", countingRead(&reads, "a")); err != nil || got != "a" {
			t.Fatalf("Remember = %q, %v", got, err)
		}
	}
	if reads != 1 {
		t.Fatalf("read ran %d times, want once", reads)
	}
}

func TestForgetMakesTheNextAskReadAgain(t *testing.T) {
	tx := &memoTx{}
	defer openTxMemo(tx)()
	reads := 0
	if _, err := Remember(tx, "owner:1", countingRead(&reads, "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := Remember(tx, "other", countingRead(&reads, "b")); err != nil {
		t.Fatal(err)
	}
	Forget(tx, "owner:")
	if _, err := Remember(tx, "owner:1", countingRead(&reads, "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := Remember(tx, "other", countingRead(&reads, "b")); err != nil {
		t.Fatal(err)
	}
	if reads != 3 {
		t.Fatalf("read ran %d times, want 3: the forgotten key again, the other key once", reads)
	}
}

func TestAFailedReadIsNotRemembered(t *testing.T) {
	tx := &memoTx{}
	defer openTxMemo(tx)()
	failed := errors.New("statement timeout")
	if _, err := Remember(tx, "list", func() (string, error) { return "", failed }); !errors.Is(err, failed) {
		t.Fatalf("err = %v, want the read's", err)
	}
	reads := 0
	if got, _ := Remember(tx, "list", countingRead(&reads, "a")); got != "a" || reads != 1 {
		t.Fatalf("after a failure got %q with %d reads, want a fresh read", got, reads)
	}
}

func TestATransactionWithoutAMemoReadsEveryTime(t *testing.T) {
	tx := &memoTx{}
	end := openTxMemo(tx)
	end()
	reads := 0
	for range 2 {
		if _, err := Remember(tx, "list", countingRead(&reads, "a")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Remember(&memoTx{}, "list", countingRead(&reads, "a")); err != nil {
		t.Fatal(err)
	}
	if reads != 3 {
		t.Fatalf("read ran %d times, want every time once the memo ended and for a transaction that never had one", reads)
	}
}
