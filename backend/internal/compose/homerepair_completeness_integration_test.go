// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The classification repair reads a request by the routine classifier's rule.
// It reads only a mail, and our last mail before it, that were read in full.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repairOperator is an admin who may preview and run a data task.
func repairOperator(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	e.GrantRole(t, e.AdminUser, "admin")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key='admin'`)
	return e.As(e.AdminUser, nil, permissions)
}

// previewVerdictRepair runs the repair preview through its HTTP handler and
// answers the planned item for the request.
func previewVerdictRepair(operator context.Context, t *testing.T, o *owedEnv, brain *owedBrainStub, request ids.UUID) homeRepairTaskRef {
	t.Helper()
	server := &Server{dataTaskHandlers: dataTaskHandlers{pool: o.e.Pool, clock: func() time.Time { return o.now }}, log: slog.Default()}
	WithHomeRepairClassifier(brain)(server, o.e.Pool)
	response := httptest.NewRecorder()
	server.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview",
		bytes.NewBufferString(`{"source":"verdict","limit":25}`)).WithContext(operator))
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
	var wire crmcontracts.HomeRepairReview
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	review, err := homeRepairReviewInput(wire)
	if err != nil {
		t.Fatal(err)
	}
	var scope homeRepairScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		t.Fatal(err)
	}
	for _, item := range scope.Tasks {
		if item.ID == request {
			return item
		}
	}
	t.Fatalf("the preview planned nothing for the request: %+v", scope)
	return homeRepairTaskRef{}
}

func TestTheClassificationRepairReadsOnlyMailReadInFull(t *testing.T) {
	for _, tc := range []struct {
		name, ask, prior string
		reclassified     bool
	}{
		{"the request read in part", brokenAfter("Could you send the contract?"), "", false},
		{"our last mail read in part", readInFull("Could you send the contract?"), brokenAfter("Here is the pricing."), false},
		{"everything read in full", readInFull("Could you send the contract?"), readInFull("Here is the pricing."), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			operator := repairOperator(t, o.e)
			inReplyTo := ""
			if tc.prior != "" {
				o.weWrite(t, owedMail{
					to: "pat@customer.example", subject: "Pricing", body: tc.prior, encoding: "base64",
					messageID: "repair-prior@ws.example", at: o.now.Add(-2 * time.Hour),
				})
				inReplyTo = "repair-prior@ws.example"
			}
			request := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "Re: Pricing", inReplyTo: inReplyTo,
				messageID: "repair-ask@customer.example", body: tc.ask, encoding: "base64", authenticated: true,
				at: o.now.Add(-time.Hour),
			})
			_, readAt, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "old-rules", o.now, 1, 400, 400)
			if err != nil {
				t.Fatal(err)
			}
			if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs, "old-rules", readAt); err != nil || !applied {
				t.Fatalf("the native verdict: applied=%v err=%v", applied, err)
			}
			brain := &owedBrainStub{verdict: activities.OwedVerdictInformsUs, confidence: 0.95}
			item := previewVerdictRepair(operator, t, o, brain, request)
			if got := item.Action == repairReclassifyMessage; got != tc.reclassified {
				t.Fatalf("planned %q (%s), want reclassified = %v", item.Action, item.Reason, tc.reclassified)
			}
			if !tc.reclassified {
				if item.Reason != "classification_evidence_incomplete" {
					t.Errorf("reason = %q, want the evidence named as incomplete", item.Reason)
				}
				for _, prompt := range brain.prompts {
					if strings.Contains(prompt.Messages[0].Content, request.String()) {
						t.Error("a mail not read in full was sent to the classifier")
					}
				}
			}
		})
	}
}

// Applying a correction rechecks the same rule, so a plan cannot write a
// verdict over a mail that was not read in full.
func TestApplyingAClassificationCorrectionRefusesMailNotReadInFull(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	operator := repairOperator(t, o.e)
	request := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "NDA", messageID: "apply-ask@customer.example",
		body: brokenAfter("Could you send the NDA?"), encoding: "base64", authenticated: true, at: o.now.Add(-time.Hour),
	})
	_, readAt, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "old-rules", o.now, 1, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs, "old-rules", readAt); err != nil || !applied {
		t.Fatalf("the native verdict: applied=%v err=%v", applied, err)
	}
	err = o.e.DB().Tx(operator, func(tx pgx.Tx) error {
		expected, err := activities.OwedReviewTx(operator, tx, request, owedBodyLimit, owedPriorBodyLimit)
		if err != nil {
			return err
		}
		if expected.EvidenceComplete {
			t.Fatal("the fixture's mail reads as complete")
		}
		_, err = activities.ApplyReviewedOwedVerdictTx(operator, tx, expected, activities.OwedVerdictInformsUs,
			"current-rules", "classification_changed", ids.NewV7(), owedBodyLimit, owedPriorBodyLimit)
		return err
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a correction over a mail not read in full: err = %v, want a conflict", err)
	}
	if *verdictOf(t, o.e, request) != activities.OwedVerdictAsksUs {
		t.Error("the verdict changed")
	}
}
