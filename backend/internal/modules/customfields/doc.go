// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package customfields is the governed add-field engine. It is the only code
// allowed to run a runtime ALTER TABLE.
//
// It validates a field definition against the closed type and object sets and
// derives the namespaced column name. It builds the DDL from the validated spec
// and never from raw user text. It also detects a structural request, so a
// caller can refuse it.
//
// engine.go is the pure core, with no database handle and no HTTP, so its DDL
// can be tested without a database. create.go runs it in a schema-pool
// transaction: the ALTER TABLE, the custom_field catalog insert and one
// audit_log row commit together.
//
// Tables owned: docs/reference/entity-model/customfields.md.
package customfields
