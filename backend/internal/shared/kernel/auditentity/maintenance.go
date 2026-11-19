// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package auditentity names operational audit subjects shared by writers and receipts.
package auditentity

// Maintenance subjects describe internal state or configuration, not CRM work.
const (
	KnowledgeDocument     = "knowledge_document"
	ModelRate             = "ai_model_rate"
	ForecastCaptureStatus = "forecast_capture_status"
	Tag                   = "tag"
	List                  = "list"
)

// MaintenanceTypes excludes only declared upkeep; unknown subjects remain visible
// to diagnostics rather than silently claiming the receipt is complete.
func MaintenanceTypes() []string {
	return []string{KnowledgeDocument, ModelRate, ForecastCaptureStatus, Tag, List}
}

// The after-image keys a list or tag audit row files a record link under.
const (
	ListMemberAdded   = "added"
	ListMemberRemoved = "removed"
	TagApplied        = "applied"
	TagRemoved        = "removed"
)

// LinkChange is a record link filed under a maintenance subject.
type LinkChange struct{ EntityType, Key string }

// LinkChanges are business work although their subject is upkeep: a record
// joining or leaving a list, a tag put on or taken off a record.
func LinkChanges() []LinkChange {
	return []LinkChange{
		{List, ListMemberAdded},
		{List, ListMemberRemoved},
		{Tag, TagApplied},
		{Tag, TagRemoved},
	}
}
