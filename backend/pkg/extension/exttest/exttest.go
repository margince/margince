// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package exttest is the base test kit for a unit's own suite.
//
// Runtime refuses every capability by default, with the sentinel the core
// answers a unit that holds nothing. Core hands out no door at all. A unit's
// fake embeds them and overrides only what its handlers use. A handler that
// starts using another capability then fails loudly. When Runtime grows a
// method, the kit grows it once and no unit's fake stops compiling.
//
//margince:extension-surface
//nolint:ireturn // each method must return the interface extension.Runtime declares.
package exttest

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

// Runtime is an extension.Runtime with nothing granted. Embed it by value.
// Caller answers the zero Caller, which is a job tick.
type Runtime struct{}

var _ extension.Runtime = Runtime{}

// Secrets answers as the core answers a unit that holds nothing.
func (Runtime) Secrets() extension.Secrets { return Secrets{} }

// Caller answers as the core answers a unit that holds nothing.
func (Runtime) Caller() extension.Caller { return extension.Caller{} }

// Tx answers as the core answers a unit that holds nothing.
func (Runtime) Tx(context.Context, func(context.Context, extension.Tx) error) error {
	return extension.ErrRuntimeExpired
}

// Ingest answers as the core answers a unit that holds nothing.
func (Runtime) Ingest(context.Context, extension.UserID, extension.Record) (extension.Result, error) {
	return extension.Result{}, extension.ErrAttendedIngest
}

// IngestBatch answers as the core answers a unit that holds nothing.
func (Runtime) IngestBatch(context.Context, extension.UserID, extension.SourceName, []extension.Record, extension.BatchPosition) ([]extension.Result, error) {
	return nil, extension.ErrAttendedIngest
}

// IngestPosition answers as the core answers a unit that holds nothing.
func (Runtime) IngestPosition(context.Context, extension.SourceName) (extension.BatchPosition, error) {
	return "", extension.ErrNotDeclared
}

// IngestMemberBatch answers as the core answers a unit that holds nothing.
func (Runtime) IngestMemberBatch(context.Context, extension.UserID, extension.SourceName, []extension.Record, extension.BatchPosition, extension.PositionVersion) (extension.MemberBatchResult, error) {
	return extension.MemberBatchResult{}, extension.ErrAttendedIngest
}

// MemberIngestPosition answers as the core answers a unit that holds nothing.
func (Runtime) MemberIngestPosition(context.Context, extension.UserID, extension.SourceName) (extension.MemberPosition, error) {
	return extension.MemberPosition{}, extension.ErrNotDeclared
}

// SyncNow answers as the core answers a unit that holds nothing.
func (Runtime) SyncNow(context.Context, extension.JobName) error { return extension.ErrNoSuchJob }

// OAuthAccessToken answers as the core answers a unit that holds nothing.
func (Runtime) OAuthAccessToken(context.Context, extension.OAuthProvider) (string, error) {
	return "", extension.ErrNoSuchOAuthApp
}

// OAuthDisconnect answers as the core answers a unit that holds nothing.
func (Runtime) OAuthDisconnect(context.Context, extension.OAuthProvider) error {
	return extension.ErrNoSuchOAuthApp
}

// Deals answers as the core answers a unit that holds nothing.
func (Runtime) Deals() extension.Deals { return deals{} }

// Authority answers as the core answers a unit that holds nothing.
func (Runtime) Authority() extension.Authority { return authority{} }

// Directory answers as the core answers a unit that holds nothing.
func (Runtime) Directory() extension.Directory { return directory{} }

// Activities answers as the core answers a unit that holds nothing.
func (Runtime) Activities() extension.Activities { return activities{} }

// SetAside answers as the core answers a unit that holds nothing.
func (Runtime) SetAside() extension.SetAside { return setAside{} }

// Companies answers as the core answers a unit that holds nothing.
func (Runtime) Companies() extension.Companies { return companies{} }

// MailContent answers as the core answers a unit that holds nothing.
func (Runtime) MailContent() extension.MailContents { return mailContents{} }

// Models answers as the core answers a unit that holds nothing.
func (Runtime) Models() extension.Models { return models{} }

// Objects answers as the core answers a unit that holds nothing.
func (Runtime) Objects() extension.Objects { return objects{} }

// Core is an extension.Core that hands back no door. Every getter answers nil,
// so a handler that files or reads through the core panics at the first call.
// Embed it and override the door under test.
// ponytail: the doors are nil, not refusing stubs; add refusing stubs when a
// suite needs the error.
type Core struct{}

var _ extension.Core = Core{}

// Activities answers nil: this kit hands back no door.
func (Core) Activities() extension.ActivityRepo { return nil }

// Deals answers nil: this kit hands back no door.
func (Core) Deals() extension.DealRepo { return nil }

// Companies answers nil: this kit hands back no door.
func (Core) Companies() extension.CompanyRepo { return nil }

// Signals answers nil: this kit hands back no door.
func (Core) Signals() extension.SignalRepo { return nil }

// CustomFields answers nil: this kit hands back no door.
func (Core) CustomFields() extension.CustomFieldRepo { return nil }

// Notices answers nil: this kit hands back no door.
func (Core) Notices() extension.NoticeRepo { return nil }

// LinkSuggestions answers nil: this kit hands back no door.
func (Core) LinkSuggestions() extension.LinkSuggestionRepo { return nil }

// Secrets refuses every read and write: nothing is stored.
type Secrets struct{}

var _ extension.Secrets = Secrets{}

// Get answers as an empty store answers.
func (Secrets) Get(context.Context, string) ([]byte, error) { return nil, extension.ErrSecretNotFound }

// Put answers as an empty store answers.
func (Secrets) Put(context.Context, string, []byte) error { return extension.ErrRuntimeExpired }

// Delete answers as an empty store answers.
func (Secrets) Delete(context.Context, string) error { return extension.ErrRuntimeExpired }

// GetUser answers as an empty store answers.
func (Secrets) GetUser(context.Context, extension.UserID, string) ([]byte, error) {
	return nil, extension.ErrSecretNotFound
}

// PutUser answers as an empty store answers.
func (Secrets) PutUser(context.Context, extension.UserID, string, []byte) error {
	return extension.ErrRuntimeExpired
}

// DeleteUser answers as an empty store answers.
func (Secrets) DeleteUser(context.Context, extension.UserID, string) error {
	return extension.ErrRuntimeExpired
}
