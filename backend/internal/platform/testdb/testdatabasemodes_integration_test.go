// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"os"
	"testing"
	"time"
)

// A session on the template blocks every copy. The copy waits it out instead
// of failing the test that happened to ask while a lane migrated the template.
//
//testdb:serial holds a session on the template, which every parallel test copies
func TestACopyWaitsForASessionOnTheTemplateToEnd(t *testing.T) {
	ownerDSN := os.Getenv(ownerDSNEnv)
	onTemplate, err := withDatabase(ownerDSN, os.Getenv(TemplateEnv))
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, onTemplate)
	released := make(chan struct{})
	go func() {
		defer close(released)
		<-time.After(300 * time.Millisecond)
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("closing the session on the template: %v", err)
		}
	}()
	db, err := copyTemplate(t.Context())
	<-released
	if err != nil {
		t.Fatalf("a copy refused while a session held the template: %v", err)
	}
	if err := release(context.Background(), "", db); err != nil {
		t.Fatal(err)
	}
}

// The bench targets keep their corpus: in place, New answers the lane's own
// database, migrated, and does not drop it when the test ends.
//
//testdb:serial sets InPlaceEnv, which every New and For in the process reads
func TestInPlaceAnswersTheLanesDatabaseAndKeepsIt(t *testing.T) {
	t.Setenv(InPlaceEnv, "1")
	lane, err := databaseName(os.Getenv(ownerDSNEnv))
	if err != nil {
		t.Fatal(err)
	}
	var name string
	t.Run("bench", func(t *testing.T) {
		db := New(t)
		name = db.Name
		if isCopy(db.Name) {
			t.Errorf("in place, New made a copy %s", db.Name)
		}
		recorded, err := recordedMigrations(t.Context(), connect(t, db.OwnerDSN), "schema_migrations_core")
		if err != nil {
			t.Fatal(err)
		}
		if reason := namespaceAtHead(coreNamespace(t), recorded); reason != "" {
			t.Errorf("in place, the lane's database is not at head: %s", reason)
		}
	})
	if name != lane {
		t.Fatalf("in place, New answered %s, want the lane's database %s", name, lane)
	}
	var exists bool
	if err := connect(t, os.Getenv(ownerDSNEnv)).QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, lane).Scan(&exists); err != nil || !exists {
		t.Errorf("the lane's database is gone after an in-place test (err=%v)", err)
	}
}
