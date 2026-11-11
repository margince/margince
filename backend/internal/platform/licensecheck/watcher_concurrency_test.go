// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// heldTokens is a token source the test holds open: once hold is set, the
// next read reports itself on entered and waits for release.
type heldTokens struct {
	mu      sync.Mutex
	token   string
	reads   int
	hold    bool
	entered chan struct{}
	release chan struct{}
}

func newHeldTokens() *heldTokens {
	return &heldTokens{entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *heldTokens) source(context.Context) (string, error) {
	h.mu.Lock()
	h.reads++
	token, hold := h.token, h.hold
	h.hold = false
	h.mu.Unlock()
	if hold {
		h.entered <- struct{}{}
		<-h.release
	}
	return token, nil
}

func (h *heldTokens) set(token string, hold bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token, h.hold = token, hold
}

func (h *heldTokens) readCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reads
}

// lockedLog is a log sink two goroutines may write while the test reads it.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) contains(s string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Contains(l.buf.String(), s)
}

// heldWatcher boots a watcher on an absent token, logging at debug so a test
// can see a caller join a re-check.
func heldWatcher(t *testing.T) (*Watcher, *heldTokens, *lockedLog) {
	t.Helper()
	source, log := newHeldTokens(), &lockedLog{}
	w, err := NewWatcher(context.Background(), source.source, fixedClock(checkedAt),
		slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug})), runtimeenv.Production)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	return w, source, log
}

// recheckAsync runs Recheck on its own goroutine and hands back its answer.
func recheckAsync(w *Watcher) <-chan error {
	answer := make(chan error, 1)
	go func() { answer <- w.Recheck(context.Background()) }()
	return answer
}

// waitForJoin waits until a second caller has joined the re-check in flight.
func waitForJoin(t *testing.T, log *lockedLog) {
	t.Helper()
	giveUp := time.After(5 * time.Second)
	for !log.contains("joining the license re-check already running") {
		select {
		case <-giveUp:
			t.Fatal("the second caller never joined the re-check in flight")
		default:
			runtime.Gosched()
		}
	}
}

// Two presses of "Check now" at once run the module once and share its answer.
func TestTwoRechecksAtOnceRunTheModuleOnce(t *testing.T) {
	t.Parallel()
	w, source, log := heldWatcher(t)
	source.set("a-replaced-token-the-module-refuses", true)

	first := recheckAsync(w)
	<-source.entered
	second := recheckAsync(w)
	waitForJoin(t, log)
	close(source.release)

	for _, answer := range []<-chan error{first, second} {
		if err := <-answer; err != nil {
			t.Errorf("Recheck: %v", err)
		}
	}
	if reads := source.readCount(); reads != 2 {
		t.Errorf("the token was read %d times, want 2: the boot and one shared re-check", reads)
	}
	if got := w.Posture().State; got != StateRejected {
		t.Errorf("posture = %q, want rejected", got)
	}
}

// A press during the daily re-check joins it. The next re-check after it reads
// the token again, and its newer answer is the one that stands.
func TestAPressDuringTheDailyRecheckJoinsItAndTheNextReadWins(t *testing.T) {
	t.Parallel()
	w, source, log := heldWatcher(t)
	source.set("", true)

	daily := recheckAsync(w)
	<-source.entered
	press := recheckAsync(w)
	waitForJoin(t, log)
	source.set("a-replaced-token-the-module-refuses", false)
	close(source.release)

	for _, answer := range []<-chan error{daily, press} {
		if err := <-answer; err != nil {
			t.Errorf("Recheck: %v", err)
		}
	}
	if got := w.Posture().State; got != StateAbsent {
		t.Fatalf("posture after the shared run = %q, want the absent token it read", got)
	}
	if reads := source.readCount(); reads != 2 {
		t.Fatalf("the token was read %d times, want 2: the boot and one shared re-check", reads)
	}

	if err := w.Recheck(context.Background()); err != nil {
		t.Fatalf("Recheck: %v", err)
	}
	if got := w.Posture().State; got != StateRejected {
		t.Errorf("posture after the next re-check = %q, want the replaced token's rejected", got)
	}
}
