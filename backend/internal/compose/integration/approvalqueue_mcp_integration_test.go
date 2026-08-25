// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The 🟡 loop closed from ONE conversation, against real Postgres.
//
// Until the queue tools existed the loop had a hole where its other half should
// be: a refused call staged a proposal, told the agent to wait for a person, and
// then neither of them could reach the queue without opening the web app. The
// agent could not even see that its own proposal was already waiting.
//
// What is proven here is the whole round trip on one credential — stage, see,
// read, answer, redeem — plus the two bounds that make it safe to admit at all:
// the answer is recorded as the PERSON's, not the credential's, and a passport
// its human lent for reading cannot answer anything.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/internal/shared/ports/workflow"
)

// queueEnv is one app plus the governed registry the api role composes.
type queueEnv struct {
	*apptest.AppEnv
	registry *agents.Registry
	auth     *identity.Service
}

func setupQueue(t *testing.T) *queueEnv {
	t.Helper()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	return &queueEnv{AppEnv: e, registry: compose.NewRegistry(e.Pool, compose.SendPath{}), auth: identity.NewService(e.Pool)}
}

// invoker re-authenticates the passport per call, exactly as every transport
// does: a credential revoked between two calls stops at the second one.
func (q *queueEnv) invoker(t *testing.T, token string) func(tool, args string) (string, error) {
	t.Helper()
	return func(tool, args string) (string, error) {
		ws, err := q.auth.InstallationWorkspace(context.Background())
		if err != nil {
			t.Fatalf("resolving the installation workspace: %v", err)
		}
		ctx := principal.WithWorkspaceID(context.Background(), ws.UUID)
		agent, err := q.auth.AuthenticateAgent(ctx, token)
		if err != nil {
			t.Fatalf("authenticating the passport: %v", err)
		}
		ctx = principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7())
		out, invokeErr := q.registry.Invoke(ctx, tool, json.RawMessage(args))
		return string(out), invokeErr
	}
}

func (q *queueEnv) mintPassport(t *testing.T, label string, scopes ...string) string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := q.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		"label": label, "scopes": scopes,
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issuing passport %q → %d", label, status)
	}
	return minted.Token
}

// stageAConfirmFirstCall refuses one 🟡 call and answers the proposal it left
// behind.
//
// It stages an `enrich`, not an archive. A passport does what its holder could
// do unaided, so archiving no longer asks a second time — while enrich stays
// confirm-first for a reason that is not about authority at all: the MODEL
// names the URL the server fetches. What the queue tests need is any verb that
// still puts a call in front of a human, and this is it.
func (q *queueEnv) stageAConfirmFirstCall(t *testing.T, invoke func(tool, args string) (string, error), name string) (orgID string, approvalID ids.UUID) {
	t.Helper()
	var org struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/organizations", apptest.AnyMap{"display_name": name}, nil, &org); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}
	_, err := invoke("enrich", `{"organization_id":"`+org.ID+`"}`)
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("enrich → %v, want a staged approval", err)
	}
	return org.ID, staged.ApprovalID.UUID
}

type queueItem struct {
	StagedActionID string          `json:"staged_action_id"`
	Kind           string          `json:"kind"`
	Status         string          `json:"status"`
	Summary        string          `json:"summary"`
	TargetType     string          `json:"target_type"`
	TargetID       string          `json:"target_id"`
	DecidedBy      *string         `json:"decided_by"`
	BundleID       *string         `json:"bundle_id"`
	ProposedChange json.RawMessage `json:"proposed_change"`
}

// answered unwraps the surface's result envelope, which every tool answer
// carries: the payload a caller reads is under `data`.
func answered[T any](t *testing.T, out string) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decoding a tool answer: %v", err)
	}
	return envelope.Data
}

// The whole round trip, on the credentials it really takes: the one that
// proposed the action sees it and reads it, a SECOND one answers it, and the
// first redeems what was released. The split is the rule, not the harness — a
// credential does not confirm its own proposal.
func TestAStagedCallIsSeenAndAnsweredFromTheConversationThatStagedIt(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "proposing agent", "read", "write", "enrich"))
	orgID, approvalID := q.stageAConfirmFirstCall(t, invoke, "Queue Subject")

	// SEE IT. The proposal the agent could not perform is in the queue it can
	// read, named the way it was staged.
	out, err := invoke("list_approvals", `{}`)
	if err != nil {
		t.Fatalf("list_approvals → %v", err)
	}
	listing := answered[struct {
		Approvals []queueItem `json:"approvals"`
	}](t, out)
	var found bool
	for _, item := range listing.Approvals {
		if item.StagedActionID != approvalID.String() {
			continue
		}
		found = true
		if item.Kind != "enrich" || item.Status != "pending" {
			t.Errorf("the staged item reads %s/%s, want enrich/pending", item.Kind, item.Status)
		}
		if item.TargetType != "organization" || item.TargetID != orgID {
			t.Errorf("the item points at %s/%s, want organization/%s", item.TargetType, item.TargetID, orgID)
		}
		if item.Summary == "" {
			t.Error("the item carries no sentence a person could answer from")
		}
		// The listing is scanned to choose; the staged document is what
		// read_approval is for. A queue that carried every payload would spend a
		// run's window on documents nobody asked to see.
		if len(item.ProposedChange) != 0 {
			t.Errorf("the listing carries the staged change (%s); that is read_approval's answer", item.ProposedChange)
		}
		// A member with nothing to say is ABSENT, never a zero uuid: this
		// proposal is waiting, and a decided_by printed as 00000000-0000-…
		// tells a caller that somebody has already answered it.
		if item.DecidedBy != nil {
			t.Errorf("a pending item names %v as having decided it", *item.DecidedBy)
		}
		if item.BundleID != nil {
			t.Errorf("an item staged alone claims to belong to act %v", *item.BundleID)
		}
	}
	if !found {
		t.Fatalf("the proposal this agent staged (%s) is not in the queue it can read", approvalID)
	}

	// READ IT. What approving would actually do.
	readArgs := `{"staged_action_id":"` + approvalID.String() + `"}`
	if out, err = invoke("read_approval", readArgs); err != nil {
		t.Fatalf("read_approval %s → %v", readArgs, err)
	}
	one := answered[queueItem](t, out)
	if len(one.ProposedChange) == 0 {
		t.Error("read_approval answered without the change it proposes — there is nothing to decide from")
	}

	// THE PROPOSER DOES NOT ANSWER IT. Approving the row it staged would be the
	// confirm-first act performed on itself.
	answer := `{"staged_action_id":"` + approvalID.String() + `","decision":"approve"}`
	if _, err = invoke("decide_approval", answer); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the proposer approving its own proposal → %v, want ErrPermissionDenied", err)
	}

	// ANSWER IT on another of the same human's credentials, and the answer is
	// the PERSON's: decided_by is the human who lent it, never the credential.
	decider := q.invoker(t, q.mintPassport(t, "deciding agent", "read", "write", "enrich"))
	if _, err = decider("decide_approval", answer); err != nil {
		t.Fatalf("decide_approval → %v", err)
	}
	var status string
	var decidedIsTheLender bool
	if err := q.Owner.QueryRow(t.Context(), `
		SELECT a.status, a.decided_by = p.on_behalf_of
		  FROM approval a JOIN passport p ON p.id = a.passport_id
		 WHERE a.id = $1`, approvalID).Scan(&status, &decidedIsTheLender); err != nil {
		t.Fatalf("reading the decided approval: %v", err)
	}
	if status != "approved" {
		t.Fatalf("the approval reads %q after its decision", status)
	}
	if !decidedIsTheLender {
		t.Error("the decision was recorded against somebody other than the person who lent the passport")
	}

	// REDEEM IT, on the credential that staged it: approving does not perform an
	// agent's staged call, the agent re-issues it, and the approval fits only
	// the caller it was staged by.
	// The retry gets PAST the gate on the approval the human granted, which is
	// what closes this loop. Where enrich lands after that is the crawler's
	// business — this composition binds no model path — so the distinction is
	// "refused by the gate" versus "released and now the tool's own answer".
	_, released := invoke("enrich",
		`{"organization_id":"`+orgID+`","approval_id":"`+approvalID.String()+`"}`)
	if errors.Is(released, apperrors.ErrRequiresApproval) || errors.Is(released, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("the released retry → %v — the approval did not release the call", released)
	}
}

// A credential lent for reading answers nothing. Acting as somebody is not the
// same as holding everything they hold: the caps are the half of a passport its
// human chose, and a decision spends write.
func TestAReadOnlyPassportSeesTheQueueAndCannotAnswerIt(t *testing.T) {
	q := setupQueue(t)
	_, approvalID := q.stageAConfirmFirstCall(t, q.invoker(t, q.mintPassport(t, "staging agent", "read", "write", "enrich")), "Read Only Subject")

	reader := q.invoker(t, q.mintPassport(t, "reading agent", "read"))
	if _, err := reader("list_approvals", `{}`); err != nil {
		t.Fatalf("a read passport cannot see the queue: %v", err)
	}
	// Refused at the admission gate, on the cap alone: the door does not need to
	// know which proposal this is to know this credential answers none.
	_, err := reader("decide_approval", `{"staged_action_id":"`+approvalID.String()+`","decision":"approve"}`)
	if !errors.Is(err, apperrors.ErrScopeExceeded) {
		t.Fatalf("a read passport deciding → %v, want the scope refusal", err)
	}
	var status string
	if err := q.Owner.QueryRow(t.Context(), `SELECT status FROM approval WHERE id = $1`, approvalID).Scan(&status); err != nil {
		t.Fatalf("reading the approval back: %v", err)
	}
	if status != "pending" {
		t.Errorf("the proposal reads %q after a refused decision — something was written anyway", status)
	}
}

// A passport does what its holder could do unaided. Relinking an activity onto
// a person, a company or a deal is an association a member undoes in the app
// with no ceremony, so it must not cost a human decision here either — while a
// PROJECT destination stays confirm-first, because filing under a project
// classifies the activity as commercial correspondence, which is write-once.
//
// Both halves are the point. relinkActivityTier has always said so; what it
// could not do was make the auto-execute half reachable. auth/admit.go raises a
// dynamic tier that resolves to auto-execute without naming the version it was
// resolved from, and relink_activity answered no version at all — so EVERY
// agent relink was raised, whatever its destination.
//
// Driven from claude.ai on 2026-08-25 that showed up as three approvals for one
// logged meeting: the model attached the person, the company and the deal after
// the fact, and each attach staged a card the app would never have asked for.
func TestAnAgentRelinksToAPersonWithoutAskingAndStillStagesAProject(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "relinking agent", "read", "write"))

	var person struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/people", apptest.AnyMap{"full_name": "Relink Subject"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}
	var org struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/organizations", apptest.AnyMap{"display_name": "Relink Account"}, nil, &org); status != http.StatusCreated {
		t.Fatalf("create organization → %d", status)
	}
	var project struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/projects", apptest.AnyMap{
		"name": "Relink Engagement", "organization_id": org.ID,
	}, nil, &project); status != http.StatusCreated {
		t.Fatalf("create project → %d", status)
	}

	// An activity linked to nothing, which is the shape the live failure left
	// behind: logged first, attached afterwards.
	logged, err := invoke("log_activity", `{"kind":"note","body":"relink me"}`)
	if err != nil {
		t.Fatalf("log_activity: %v", err)
	}
	// The tool answers the governed envelope, not a bare record.
	var loggedEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(logged), &loggedEnvelope); err != nil {
		t.Fatalf("reading the logged activity: %v", err)
	}
	activity := loggedEnvelope.Data
	if activity.ID == "" {
		t.Fatalf("log_activity named no activity id: %s", logged)
	}

	for _, destination := range []struct {
		entityType string
		entityID   string
	}{
		{"person", person.ID},
		{"organization", org.ID},
	} {
		t.Run("a "+destination.entityType+" is relinked with no approval", func(t *testing.T) {
			if _, err := invoke("relink_activity", `{"activity_id":"`+activity.ID+
				`","entity_type":"`+destination.entityType+`","entity_id":"`+destination.entityID+`"}`); err != nil {
				var staged *workflow.StagedApprovalError
				if errors.As(err, &staged) {
					t.Fatalf("relinking to a %s staged approval %s; a member does this in the app unasked",
						destination.entityType, staged.ApprovalID)
				}
				t.Fatalf("relink to %s: %v", destination.entityType, err)
			}
		})
	}

	t.Run("a project still stages", func(t *testing.T) {
		_, err := invoke("relink_activity", `{"activity_id":"`+activity.ID+
			`","entity_type":"project","entity_id":"`+project.ID+`"}`)
		var staged *workflow.StagedApprovalError
		if !errors.As(err, &staged) {
			t.Fatalf("relinking to a project → %v, want a staged approval — filing under a project is write-once", err)
		}
	})
}
