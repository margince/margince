// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
)

const (
	followUpMinDays = 1
	followUpMaxDays = 30
	// settingsAuditVerb is the audit action every activities settings change records.
	settingsAuditVerb = "update"
	// settingsObject is the RBAC object that guards every activities setting.
	settingsObject = "installation_settings"
)

// FollowUpAfterDays is how long a message the reader sent may go unanswered
// before their worklist reminds them to follow up. An admin sets it for the
// whole workspace; two days is where a sales follow-up is due.
var FollowUpAfterDays = config.DefineStored(config.Spec[int]{
	Key:     "activities.follow_up_after_days",
	Env:     "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS",
	Type:    config.Int,
	Default: config.Some(2),
	Min:     config.Some(followUpMinDays),
	Max:     config.Some(followUpMaxDays),
	Doc:     "Days a sent message may go without a reply before its sender is asked to follow up.",
}, config.Store{
	Cacheable:  true,
	RBACObject: settingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the worklist applies it to rows the reader is separately gated for
})

// PrivateMailRequests lets the AI read mail that is private for a routine reason
// (privateReasons): the owed-reply check, and the signal pass for a quotation
// request. Mail held for any other reason never
// goes to a model, whatever this says. On by default: with mail sharing off, every new
// mail is private, and without it no request in it would ever be found.
var PrivateMailRequests = config.DefineStored(config.Spec[bool]{
	Key:     "activities.private_mail_requests",
	Env:     "MARGINCE_ACTIVITIES_PRIVATE_MAIL_REQUESTS",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let the AI read private mail for requests: what a customer asks of its reader, and a request for a quote. Only mail private because of a setting is read; mail held for any other reason never is. Sharing a quote request with the workspace is a separate setting.",
}, config.Store{
	RBACObject: settingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the owed-reply pass reads it as the system, for every seat's mail
})

// SharePrivateQuoteRequests lets a quote request in mail private only by a
// mailbox setting become visible to the whole workspace, on the deal it opens.
// It needs PrivateMailRequests on as well. Mail held for any
// other reason is never shared. On by default, as the product decided.
var SharePrivateQuoteRequests = config.DefineStored(config.Spec[bool]{
	Key:     "activities.share_private_quote_requests",
	Env:     "MARGINCE_ACTIVITIES_SHARE_PRIVATE_QUOTE_REQUESTS",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Share a request for a quote found in private mail with the whole workspace: what the customer asked for becomes visible on the deal it opens. Only mail private because of a mailbox setting counts; mail held for any other reason never does.",
}, config.Store{
	RBACObject: settingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the signal pass and the deal opener read it as the system
})

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{FollowUpAfterDays, PrivateMailRequests, SharePrivateQuoteRequests}
}

// sharePrivateQuoteRequests reads SharePrivateQuoteRequests in the caller's
// transaction.
func sharePrivateQuoteRequests(ctx context.Context, tx pgx.Tx) (bool, error) {
	on, err := database.Remember(tx, settings.ValueMemoKey(SharePrivateQuoteRequests.Key()), func() (bool, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(), SharePrivateQuoteRequests)
	})
	if err != nil {
		return false, fmt.Errorf("read %s: %w", SharePrivateQuoteRequests.Key(), err)
	}
	return on, nil
}

// followUpAfterDays reads the setting inside the caller's transaction, under
// the boot value the program bound.
func followUpAfterDays(ctx context.Context, tx pgx.Tx) (int, error) {
	days, err := database.Remember(tx, settings.ValueMemoKey(FollowUpAfterDays.Key()), func() (int, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(ctx), FollowUpAfterDays)
	})
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", FollowUpAfterDays.Key(), err)
	}
	return days, nil
}

// privateMailRequests reads PrivateMailRequests inside the caller's
// transaction, so one pass judges every row by one answer.
func privateMailRequests(ctx context.Context, tx pgx.Tx) (bool, error) {
	on, err := database.Remember(tx, settings.ValueMemoKey(PrivateMailRequests.Key()), func() (bool, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(), PrivateMailRequests)
	})
	if err != nil {
		return false, fmt.Errorf("read %s: %w", PrivateMailRequests.Key(), err)
	}
	return on, nil
}
