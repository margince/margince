// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package migration is the shared importer engine
// (import-export-migration, IEM-FORM-1): one mapping/classification
// step, one dry-run, one checkpointed resumable run loop — sources plug
// in as connectors behind the Source seam, and native rows land through
// the Writers seam so this module never imports the record modules it
// feeds (compose injects both). CSV is the connector it serves today
// (UC-E11-03); Salesforce plugs into the same engine in its own ticket.
package migration
