// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTemplateCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			ctx := e.Admin()
			header := strings.Repeat("Letterhead line\n", 30) + "Final address line"
			fields := map[string]any{"name": "  Standard offer  ", "is_default": true, "layout": map[string]any{"header": header, "footer": "Registered company details"}}
			payload := map[string]any{"record_type": "offer_template", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createOfferTemplate", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "offer_template", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil || !slices.Contains(*preview.Effects, "set_template_locale_default") {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]crmcontracts.ApprovalPreviewValue{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value
			}
			if details["name"].Text != "Standard offer" || details["locale"].Text != "de-DE" || details["is_default"].Text != "true" || details["template_header"].Text != header || details["template_footer"].Text != "Registered company details" {
				t.Fatalf("template content differs: %+v", details)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM offer_template`); count != 0 {
				t.Fatalf("preview wrote %d templates", count)
			}
			input, err := templateCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			store := deals.NewStore(e.DB(), DealsInstallation())
			actual, err := store.CreateOfferTemplate(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Name != details["name"].Text || actual.Locale != details["locale"].Text || !actual.IsDefault || actual.Layout["header"] != header {
				t.Fatalf("writer differs: %+v", actual)
			}
			duplicate, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if duplicate.Preview == nil || duplicate.Preview.State != previewIncompleteState || duplicate.Preview.Details != nil {
				t.Fatalf("name collision remains ready: %+v", duplicate.Preview)
			}
			input.Name = "Another default"
			if _, err := store.PreviewTemplateCreation(ctx, input); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("default collision preview: %v", err)
			}
			if _, err := store.CreateOfferTemplate(ctx, input); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("default collision write: %v", err)
			}
			incumbent, err := store.GetOfferTemplate(ctx, ids.From[ids.OfferTemplateKind](ids.UUID(actual.Id)), storekit.LiveOnly)
			if err != nil || !incumbent.IsDefault {
				t.Fatalf("incumbent changed: %+v, %v", incumbent, err)
			}
			input.IsDefault = false
			input.Layout["unknown_content"] = map[string]any{"text": "Unreviewed"}
			if _, err := newApprovalPreviewReader(e.DB()).templateCreationDetails(ctx, input); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("unknown layout silently omitted: %v", err)
			}
		})
	}
}
