// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// coldStartBudget is PERF-6: the single binary answers ready within 2 s.
const coldStartBudget = 2 * time.Second

// coldStartSamples is how many boots the row is judged on. The first boot
// writes the installation's own rows, so it is run and never counted.
const coldStartSamples = 10

// coldStartPatience bounds one boot, well past the budget, so a binary that
// never becomes ready fails the run instead of hanging it.
const coldStartPatience = 30 * time.Second

// TestColdStartBudget times the built API binary from process start until
// /readyz answers 200, over a migrated database.
func TestColdStartBudget(t *testing.T) {
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	redisHost, redisDB := os.Getenv("MARGINCE_TEST_REDIS"), os.Getenv("MARGINCE_TEST_REDIS_DB")
	if ownerDSN == "" || appDSN == "" || redisHost == "" || redisDB == "" {
		t.Fatal("the test database and Redis are unset: run through make bench-coldstart")
	}
	boot := coldBoot{
		binary: buildAPI(t), root: integration.RepositoryRoot(t), work: bootWorkdir(t),
		appDSN: appDSN, ownerDSN: ownerDSN, redisAddr: redisHost + "/" + redisDB,
	}
	if _, err := boot.once(t); err != nil {
		t.Fatalf("the first boot, which writes the installation: %v", err)
	}
	runs := make([]time.Duration, 0, coldStartSamples)
	for range coldStartSamples {
		took, err := boot.once(t)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, took)
	}
	stats, err := search.MeasureQuery("cold_start", coldStartBudget, runs)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("perfbench: cold_start p50=%s p95=%s p99=%s budget=%s samples=%d", stats.P50, stats.P95, stats.P99, coldStartBudget, stats.Samples)
	row := integration.MeasurementFrom("PERF-6", "cold_start", stats.P50, stats.P95, stats.P99, coldStartBudget, stats.Samples)
	row.Note = "process start to /readyz 200 over a migrated database, with the example deployment config in the dev posture; the first boot is not counted"
	integration.WritePerfRecord(t, "bench-coldstart", postgresVersion(t, ownerDSN), []integration.BudgetMeasurement{row})
	if stats.P95 > coldStartBudget {
		t.Errorf("cold start p95 %s is over the %s budget", stats.P95, coldStartBudget)
	}
}

// buildAPI compiles this package, in the workspace the caller set, as the
// binary an operator runs.
func buildAPI(t *testing.T) string {
	t.Helper()
	// The build reads the composed extensions, and the boot reads the config.
	if err := gatekit.DeclareInputs(os.Getenv, "../../../extensions", "../../../build/composition", "../../../config/margince.example.yaml"); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "api")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building cmd/api: %v\n%s", err, out)
	}
	return binary
}

// bootWorkdir holds the admin password file the example deployment config
// names by a relative path.
func bootWorkdir(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "config", "margince-admin-password"), []byte("cold-start-bench-password"), 0o600); err != nil {
		t.Fatal(err)
	}
	return work
}

type coldBoot struct {
	binary, root, work          string
	appDSN, ownerDSN, redisAddr string
}

// once starts the binary, waits for /readyz to answer 200, and stops it. The
// clock starts before the process does, so process start-up is counted.
func (b coldBoot) once(t *testing.T) (time.Duration, error) {
	t.Helper()
	addr := freeAddr(t)
	var logs bytes.Buffer
	// The shipped deployment config, without the dev overlay. That overlay turns
	// the MCP connector on, whose boot fetch waits for a web tier this run lacks.
	api := exec.Command(b.binary, "--addr", addr, "--dsn", b.appDSN, "--redis", b.redisAddr,
		"--config", filepath.Join(b.root, "config", "margince.example.yaml"))
	api.Dir = b.work
	api.Stdout, api.Stderr = &logs, &logs
	api.Env = append(os.Environ(),
		"MARGINCE_SCHEMA_DSN="+b.ownerDSN,
		"MARGINCE_ENV=dev",
		"MARGINCE_AI_FAKE=true",
		"MARGINCE_KEYVAULT_ROOT_KEY=bWFyZ2luY2UtZGV2LW9ubHkta2V5dmF1bHQtcm9vdGs=",
		"MARGINCE_CONNECTOR_STATE_KEY=margince-dev-connector-state-key-0001",
		"MARGINCE_PUBLIC_BASE_URL=http://"+addr,
	)
	start := time.Now()
	if err := api.Start(); err != nil {
		return 0, fmt.Errorf("starting the binary: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- api.Wait() }()
	took, err := awaitReady("http://"+addr+"/readyz", start, exited)
	if stopErr := stop(api, exited); stopErr != nil && err == nil {
		err = stopErr
	}
	if err != nil {
		return 0, fmt.Errorf("%w\n%s", err, logs.String())
	}
	return took, nil
}

// awaitReady polls until the probe answers 200, the process exits, or the
// patience runs out.
func awaitReady(url string, start time.Time, exited <-chan error) (time.Duration, error) {
	client := http.Client{Timeout: time.Second}
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	patience := time.NewTimer(coldStartPatience - time.Since(start))
	defer patience.Stop()
	for {
		select {
		case err := <-exited:
			return 0, fmt.Errorf("the binary exited before it was ready: %w", err)
		case <-patience.C:
			return 0, fmt.Errorf("not ready after %s", coldStartPatience)
		case <-poll.C:
		}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		ready := resp.StatusCode == http.StatusOK
		if err := resp.Body.Close(); err != nil {
			return 0, err
		}
		if ready {
			return time.Since(start), nil
		}
	}
}

// stop ends the binary the way a supervisor does and waits for it to go.
func stop(api *exec.Cmd, exited <-chan error) error {
	if err := api.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-exited:
		return nil
	case <-time.After(coldStartPatience):
		if err := api.Process.Kill(); err != nil {
			return err
		}
		return errors.New("the binary did not stop on SIGTERM")
	}
}

// freeAddr is a loopback address nothing listens on yet.
func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("a TCP listener reported %T as its address", listener.Addr())
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
}

func postgresVersion(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to read the server version: %v", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing: %v", err)
		}
	}()
	return integration.PostgresVersion(func(sql string) (string, error) {
		var version string
		err := conn.QueryRow(ctx, sql).Scan(&version)
		return version, err
	})
}
