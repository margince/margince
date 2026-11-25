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
package exttest

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

// Runtime is an extension.Runtime with nothing granted. Embed it by value.
// Caller answers the zero Caller, which is a job tick.
type Runtime struct{}

var _ extension.Runtime = Runtime{}

func (Runtime) Secrets() extension.Secrets { return Secrets{} }
func (Runtime) Caller() extension.Caller   { return extension.Caller{} }

func (Runtime) Tx(context.Context, func(context.Context, extension.Tx) error) error {
	return extension.ErrRuntimeExpired
}

func (Runtime) Ingest(context.Context, extension.UserID, extension.Record) (extension.Result, error) {
	return extension.Result{}, extension.ErrAttendedIngest
}

func (Runtime) IngestBatch(context.Context, extension.UserID, extension.SourceName, []extension.Record, extension.BatchPosition) ([]extension.Result, error) {
	return nil, extension.ErrAttendedIngest
}

func (Runtime) IngestPosition(context.Context, extension.SourceName) (extension.BatchPosition, error) {
	return "", extension.ErrNotDeclared
}

func (Runtime) SyncNow(context.Context, extension.JobName) error { return extension.ErrNoSuchJob }

func (Runtime) OAuthAccessToken(context.Context, extension.OAuthProvider) (string, error) {
	return "", extension.ErrNoSuchOAuthApp
}

func (Runtime) OAuthDisconnect(context.Context, extension.OAuthProvider) error {
	return extension.ErrNoSuchOAuthApp
}

func (Runtime) Deals() extension.Deals              { return deals{} }
func (Runtime) Authority() extension.Authority      { return authority{} }
func (Runtime) Directory() extension.Directory      { return directory{} }
func (Runtime) Activities() extension.Activities    { return activities{} }
func (Runtime) SetAside() extension.SetAside        { return setAside{} }
func (Runtime) Companies() extension.Companies      { return companies{} }
func (Runtime) MailContent() extension.MailContents { return mailContents{} }
func (Runtime) Models() extension.Models            { return models{} }
func (Runtime) Objects() extension.Objects          { return objects{} }

// Core is an extension.Core that hands back no door. Every getter answers nil,
// so a handler that files or reads through the core panics at the first call.
// Embed it and override the door under test.
// ponytail: the doors are nil, not refusing stubs; add refusing stubs when a
// suite needs the error.
type Core struct{}

var _ extension.Core = Core{}

func (Core) Activities() extension.ActivityRepo            { return nil }
func (Core) Deals() extension.DealRepo                     { return nil }
func (Core) Companies() extension.CompanyRepo              { return nil }
func (Core) Signals() extension.SignalRepo                 { return nil }
func (Core) CustomFields() extension.CustomFieldRepo       { return nil }
func (Core) Notices() extension.NoticeRepo                 { return nil }
func (Core) LinkSuggestions() extension.LinkSuggestionRepo { return nil }

// Secrets refuses every read and write: nothing is stored.
type Secrets struct{}

var _ extension.Secrets = Secrets{}

func (Secrets) Get(context.Context, string) ([]byte, error) { return nil, extension.ErrSecretNotFound }
func (Secrets) Put(context.Context, string, []byte) error   { return extension.ErrRuntimeExpired }
func (Secrets) Delete(context.Context, string) error        { return extension.ErrRuntimeExpired }

func (Secrets) GetUser(context.Context, extension.UserID, string) ([]byte, error) {
	return nil, extension.ErrSecretNotFound
}

func (Secrets) PutUser(context.Context, extension.UserID, string, []byte) error {
	return extension.ErrRuntimeExpired
}

func (Secrets) DeleteUser(context.Context, extension.UserID, string) error {
	return extension.ErrRuntimeExpired
}
