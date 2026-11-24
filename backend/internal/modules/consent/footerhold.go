// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// FooterHeldAction is the system_log action of one send held because its
// footer owed a company detail nobody stated. FooterGapsTx counts these rows.
const FooterHeldAction = "send.footer_held"

// RecordFooterHoldTx writes one row for each sending entity this decision set
// holds for ReasonFooterIncomplete. background says no user pressed send.
// intent is the held message, zero when none is held; FooterGapsTx counts one
// held message once however often it was pressed. A set held for another
// reason writes nothing.
func RecordFooterHoldTx(ctx context.Context, tx pgx.Tx, set commsauthz.DecisionSet, intent ids.UUID, background bool) error {
	kindsByEntity := map[ids.UUID][]string{}
	var entities []ids.UUID
	for _, d := range set.Decisions {
		if d.ReasonCode != commsauthz.ReasonFooterIncomplete {
			continue
		}
		if _, seen := kindsByEntity[d.SendingEntity]; !seen {
			entities = append(entities, d.SendingEntity)
			kindsByEntity[d.SendingEntity] = []string{}
		}
		kindsByEntity[d.SendingEntity] = appendDetailKinds(kindsByEntity[d.SendingEntity], d.Evidence.FooterFindings)
	}
	for _, entity := range entities {
		// A zero entity is the sender's own, which is the entity the
		// disclosures of a zero entity were resolved for.
		named := entity
		if named.IsZero() {
			var err error
			if named, err = sendingEntityOf(ctx, tx); err != nil {
				return err
			}
		}
		detail := map[string]any{"legal_entity_id": named.String(), "kinds": kindsByEntity[entity], "background": background}
		if !intent.IsZero() {
			detail["intent_id"] = intent.String()
		}
		if _, err := storekit.LogSystem(ctx, tx, FooterHeldAction, detail); err != nil {
			return err
		}
	}
	return nil
}

// appendDetailKinds keeps the findings that name a company detail. A missing
// stop link is a footer finding too, but no detail an admin fills in.
func appendDetailKinds(kinds []string, findings []commsauthz.FooterFinding) []string {
	for _, finding := range findings {
		if messagingrules.DisclosureKind(finding.Kind).Validate() == nil && !slices.Contains(kinds, finding.Kind) {
			kinds = append(kinds, finding.Kind)
		}
	}
	return kinds
}
