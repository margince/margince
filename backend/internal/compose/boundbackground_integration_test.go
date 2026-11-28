// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// boundBackground is the context the worker middleware hands a job: a
// background context with the installation's workspace bound by the same
// bindInstallation the client runs. A test that calls Work directly goes round
// the middleware, so it binds the way the middleware would. It reads the
// workspace the test's environment has made live when it is called.
func boundBackground(t *testing.T) context.Context {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set; run `make db-up` and try again")
	}
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, err := bindInstallation(context.Background(), pool)
	if err != nil {
		t.Fatalf("binding the installation's workspace: %v", err)
	}
	return ctx
}
