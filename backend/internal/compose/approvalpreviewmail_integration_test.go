// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailApprovalPreservesCompleteMessageAndFilingContext(t *testing.T) {
	for _, rest := range []bool{false, true} {
		name := "tool"
		if rest {
			name = "rest"
		}
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
			agreement, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{CompanyID: ids.From[ids.CompanyKind](company), Title: "Delivery agreement", ValueBasis: contracts.BasisTotal, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			fileBody := "Delivery terms accepted for review."
			file, err := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory()).UploadAttachment(e.Admin(), activities.AttachmentInput{
				EntityType: "company", EntityID: company, Filename: "terms.txt", ContentType: "text/plain", Content: strings.NewReader(fileBody),
			})
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Repeat("Complete proposal paragraph.\n", 80) + "Final commitment: delivery on Friday."
			payload := map[string]any{
				"to": []string{"buyer@example.test", "finance@example.test", "ADVISOR@example.test", "archive@example.test"},
				"cc": []string{"advisor@example.test"}, "bcc": []string{"archive@example.test"},
				"subject": "Delivery agreement", "body": body,
				"communication_context": "requested_followup", "consent_purpose": "transactional", "operator_reason": "Buyer requested the delivery plan.",
				"evidence":       map[string]string{"contract_id": agreement.Id.String()},
				"attachment_ids": []string{file.Id.String(), file.Id.String()},
				"links":          []map[string]any{{"entity_type": "company", "entity_id": company}},
				"scheduled_at":   "2027-01-12T09:00:00Z", "scheduled_tz": "Europe/Berlin",
			}
			if rest {
				payload = map[string]any{"operation": "SendCompanyEmail", "body": payload}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			canonical, hash, err := diffhash.Canonical(raw)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{
				Kind: "send_company_email", TargetType: "activity",
				ProposedChange: canonical, DiffHash: hash,
			})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(e.Admin(), id)
			if err != nil || proposal.Preview == nil || proposal.Preview.Details == nil {
				t.Fatalf("message preview: %+v, %v", proposal.Preview, err)
			}
			if proposal.Preview.Effects == nil || len(*proposal.Preview.Effects) != 1 || (*proposal.Preview.Effects)[0] != "authorize_message_send" {
				t.Fatalf("message decision does not explain authorization: %+v", proposal.Preview.Effects)
			}
			if proposal.Preview.Files == nil || len(*proposal.Preview.Files) != 1 {
				t.Fatalf("message attachments: %+v", proposal.Preview.Files)
			}
			shown := (*proposal.Preview.Files)[0]
			if shown.Id != file.Id || shown.Filename != file.Filename || shown.ByteSize != int64(len(fileBody)) || shown.ContentType != "text/plain" {
				t.Fatalf("attachment review differs from uploaded file: %+v", shown)
			}
			details := map[string]string{}
			for _, detail := range *proposal.Preview.Details {
				details[detail.Field] = detail.Value.Text
			}
			for field, want := range map[string]string{
				"to": "buyer@example.test\nfinance@example.test", "cc": "advisor@example.test",
				"bcc": "archive@example.test", "subject": "Delivery agreement", "body": body,
				"filed_under": "Harbor account", "scheduled_at": "2027-01-12T09:00:00Z",
				"scheduled_tz":          "Europe/Berlin",
				"communication_context": "requested_followup", "message_purpose": "transactional", "operator_reason": "Buyer requested the delivery plan.",
				"contract_id": "Delivery agreement",
			} {
				if details[field] != want {
					t.Errorf("%s: got %q, want %q", field, details[field], want)
				}
			}
		})
	}
}

func TestReplyApprovalInheritsTheOriginalConversationFiling(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
	contact := e.SeedContact(t, "Dana Buyer", &e.AdminUser)
	anchor, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new("Delivery question"), Body: new("When can you deliver?"),
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}, {EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical([]byte(`{"to":["buyer@example.test"],"subject":"Re: Delivery question","body":"We can deliver Friday."}`))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{
		Kind: "send_email", TargetType: "activity", TargetID: ids.UUID(anchor.Id), ProposedChange: canonical, DiffHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != previewReadyState || proposal.Preview.Details == nil {
		t.Fatalf("reply preview: %+v", proposal.Preview)
	}
	filed := map[ids.UUID]string{}
	for _, detail := range *proposal.Preview.Details {
		if detail.Field == "filed_under" && detail.Value.EntityId != nil {
			filed[ids.UUID(*detail.Value.EntityId)] = detail.Value.Text
		}
	}
	if len(filed) != 2 || filed[company] != "Harbor account" || filed[contact] != "Dana Buyer" {
		t.Fatalf("reply filing does not match its conversation: %+v", filed)
	}
}
