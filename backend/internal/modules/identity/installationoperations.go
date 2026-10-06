// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The worker's operating values on the installation-settings surface: how
// often each background pass runs, how far ahead a mailbox subscription is
// renewed, and how fast one mailbox sends. All whole numbers, so they are read
// and patched as (setting, value) pairs rather than one block per field.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// OperationSettings holds the operating values as they stand. Its fields are the
// contract's OperationSettings, in the same order, so the handler converts
// one to the other and the compiler holds the two equal.
type OperationSettings struct {
	AgentRunnerIntervalSeconds       int
	CloseDateSweepIntervalSeconds    int
	FollowUpReconcileIntervalSeconds int
	GeocodeBackfillIntervalSeconds   int
	GmailWatchRenewWithinHours       int
	GmailWatchScanIntervalSeconds    int
	GraphWatchRenewWithinHours       int
	GraphWatchScanIntervalSeconds    int
	RetentionSweepIntervalSeconds    int
	SendMaxAgeHours                  int
	SendRateLimit                    int
	SendRateWindowSeconds            int
	TechnicalBackfillIntervalSeconds int
	TimeScanIntervalSeconds          int
	WebhookRetryIntervalSeconds      int
}

// OperationPatch is a sparse write of those values: a nil field is left as it is.
type OperationPatch struct {
	AgentRunnerIntervalSeconds       *int
	CloseDateSweepIntervalSeconds    *int
	FollowUpReconcileIntervalSeconds *int
	GeocodeBackfillIntervalSeconds   *int
	GmailWatchRenewWithinHours       *int
	GmailWatchScanIntervalSeconds    *int
	GraphWatchRenewWithinHours       *int
	GraphWatchScanIntervalSeconds    *int
	RetentionSweepIntervalSeconds    *int
	SendMaxAgeHours                  *int
	SendRateLimit                    *int
	SendRateWindowSeconds            *int
	TechnicalBackfillIntervalSeconds *int
	TimeScanIntervalSeconds          *int
	WebhookRetryIntervalSeconds      *int
}

type operationField struct {
	entry *settings.Entry[int]
	into  *int
}

// fields pairs every value with its setting. installationoperations_test.go
// holds this list and writes() to the structs above, so a value added there
// cannot be left unread or unsaved.
func (o *OperationSettings) fields() []operationField {
	return []operationField{
		{AgentRunnerIntervalSeconds, &o.AgentRunnerIntervalSeconds},
		{CloseDateSweepIntervalSeconds, &o.CloseDateSweepIntervalSeconds},
		{FollowUpReconcileIntervalSeconds, &o.FollowUpReconcileIntervalSeconds},
		{GeocodeBackfillIntervalSeconds, &o.GeocodeBackfillIntervalSeconds},
		{GmailWatchRenewWithinHours, &o.GmailWatchRenewWithinHours},
		{GmailWatchScanIntervalSeconds, &o.GmailWatchScanIntervalSeconds},
		{GraphWatchRenewWithinHours, &o.GraphWatchRenewWithinHours},
		{GraphWatchScanIntervalSeconds, &o.GraphWatchScanIntervalSeconds},
		{RetentionSweepIntervalSeconds, &o.RetentionSweepIntervalSeconds},
		{SendMaxAgeHours, &o.SendMaxAgeHours},
		{SendRateLimit, &o.SendRateLimit},
		{SendRateWindowSeconds, &o.SendRateWindowSeconds},
		{TechnicalBackfillIntervalSeconds, &o.TechnicalBackfillIntervalSeconds},
		{TimeScanIntervalSeconds, &o.TimeScanIntervalSeconds},
		{WebhookRetryIntervalSeconds, &o.WebhookRetryIntervalSeconds},
	}
}

func (p OperationPatch) writes() ([]pendingWrite, error) {
	values := []struct {
		entry *settings.Entry[int]
		value *int
	}{
		{AgentRunnerIntervalSeconds, p.AgentRunnerIntervalSeconds},
		{CloseDateSweepIntervalSeconds, p.CloseDateSweepIntervalSeconds},
		{FollowUpReconcileIntervalSeconds, p.FollowUpReconcileIntervalSeconds},
		{GeocodeBackfillIntervalSeconds, p.GeocodeBackfillIntervalSeconds},
		{GmailWatchRenewWithinHours, p.GmailWatchRenewWithinHours},
		{GmailWatchScanIntervalSeconds, p.GmailWatchScanIntervalSeconds},
		{GraphWatchRenewWithinHours, p.GraphWatchRenewWithinHours},
		{GraphWatchScanIntervalSeconds, p.GraphWatchScanIntervalSeconds},
		{RetentionSweepIntervalSeconds, p.RetentionSweepIntervalSeconds},
		{SendMaxAgeHours, p.SendMaxAgeHours},
		{SendRateLimit, p.SendRateLimit},
		{SendRateWindowSeconds, p.SendRateWindowSeconds},
		{TechnicalBackfillIntervalSeconds, p.TechnicalBackfillIntervalSeconds},
		{TimeScanIntervalSeconds, p.TimeScanIntervalSeconds},
		{WebhookRetryIntervalSeconds, p.WebhookRetryIntervalSeconds},
	}
	out := make([]pendingWrite, 0, len(values))
	for _, v := range values {
		write, err := encodePatchField(v.entry, v.value)
		if err != nil {
			return nil, err
		}
		out = append(out, write)
	}
	return out, nil
}

// readOperations reads every value in one transaction, as one view rather than
// fifteen.
func (s *InstallationSettingsStore) readOperations(ctx context.Context) (OperationSettings, error) {
	var out OperationSettings
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		for _, field := range out.fields() {
			value, err := settings.GetTx(ctx, tx, field.entry)
			if err != nil {
				return err
			}
			*field.into = value
		}
		return nil
	})
	if err != nil {
		return OperationSettings{}, fmt.Errorf("identity: reading the operating values: %w", err)
	}
	return out, nil
}
