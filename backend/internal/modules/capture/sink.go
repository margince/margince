// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Sink is the one connector.Sink implementation — the chokepoint every
// captured record passes on its way into the domain.
type Sink struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db             *database.DB
	stager         MergeStager
	ensurer        CounterpartyEnsurer
	channelEnsurer ChannelCounterpartyEnsurer
	transactional  *TransactionalList
	// files is the timeline module's attachment writer. Nil is a role that
	// keeps no files: messages still land, and their attachments do not.
	files FileKeeper
	// projectKeys resolves the subject-line rung of the project attribution
	// ladder. Nil is a role that attributes nothing — messages still land, and
	// none of them is filed under a project.
	projectKeys ProjectKeyMatcher
	// stampProject classifies the correspondence a project link qualifies. It
	// arrives with projectKeys in one ProjectAttribution value, so the two are
	// set together or neither is; attributeProject checks both anyway, because
	// a ladder that files without classifying is the one outcome this must
	// never reach.
	stampProject StampProjectCorrespondence
	// tracePayloads is the deployment's capture.trace_payloads posture: with it
	// on, the 24-hour trace keeps each message's sender and subject. Off is the
	// default and the only value a member can cause.
	tracePayloads bool
	// recomputeAudience derives a captured message's audience from every
	// mailbox that imported it. Nil derives nothing and leaves the audience the
	// capture was born with.
	recomputeAudience AudienceRecomputer
	// nameParticipants completes a resolved attendee's name from the name the
	// invitation gave them. Nil names nobody.
	nameParticipants ParticipantNamer
}

// fieldSourceSystem / fieldSourceID are the shared system_log detail keys for
// the natural key of the record a capture breadcrumb is about.
const (
	fieldSourceSystem = "source_system"
	fieldSourceID     = "source_id"
	fieldReason       = "reason"
	// fieldError carries the cause on a fault breadcrumb. Named once because
	// two post-commit steps record one, and an operator filtering system_log
	// must not have to know which of them spelled the key.
	fieldError = "error"
)

// MergeStager is the dedupe seam: a captured lead colliding with an
// existing record NEVER auto-merges — it stages a 🟡 merge_records
// proposal for the inbox. Compose injects the approvals engine.
type MergeStager interface {
	// note: the returned id is the staged approval's — it stays untyped
	// because the approvals engine behind this seam is the caller's, not
	// this module's, and the value is discarded here.
	StageMerge(ctx context.Context, in MergeProposal) (ids.UUID, error)
}

// MergeProposal names the collision: the surviving record and the
// captured fields that would fold into it.
type MergeProposal struct {
	// note: TargetType + TargetID are the polymorphic pair the approvals
	// merge target carries — this is a discriminated ref, not a single
	// entity's id, so it stays untyped (kernel Ref semantics).
	TargetType     string
	TargetID       ids.UUID
	ProposedChange json.RawMessage
	Summary        string
}

// MergeAddress reads the address a merge proposal collided on out of its
// payload, in the normalized form captureLead stores it.
//
// It lives here because this module owns what the payload is: the composition
// root stages the proposal and needs one field of it for the rejection memory's
// identity, and re-deriving that field there would be a second answer to "what
// shape is a dedupe payload" sitting in a package that never builds one.
//
// An unreadable payload answers the empty string rather than an error. The
// caller's use is an identity, and an identity that matches nothing costs a
// duplicate card a rep has seen before; failing the staging would cost the
// proposal entirely, and a collision nobody is told about is worse than one
// they are told about twice.
func MergeAddress(proposedChange json.RawMessage) string {
	var fields LeadFields
	if err := json.Unmarshal(proposedChange, &fields); err != nil {
		return ""
	}
	return fields.Email
}

// NewSink binds the capture sink to the pool its writes run through.
func NewSink(db *database.DB) *Sink {
	return &Sink{db: db}
}

// WithFileKeeper returns a copy that keeps the files a captured message
// carried. Without it the messages still land; their attachments do not.
func (s *Sink) WithFileKeeper(files FileKeeper) *Sink {
	c := *s
	c.files = files
	return &c
}

// WithStager returns a copy wired to the merge-staging path.
func (s *Sink) WithStager(stager MergeStager) *Sink {
	c := *s
	c.stager = stager
	return &c
}

var _ connector.Sink = (*Sink)(nil)

// Upsert lands one normalized record: raw original + domain row +
// audit + captured event, one transaction, idempotent on the natural
// key. Replays return the existing row and write NOTHING new — an
// at-least-once sync loop costs no duplicate audit entries.
func (s *Sink) Upsert(ctx context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalConnector {
		return datasource.EntityRef{}, errors.New("capture: sink requires a connector principal — the registry builds it, nothing else may")
	}
	if rec.NaturalKey.SourceSystem == "" || rec.NaturalKey.SourceID == "" {
		return datasource.EntityRef{}, errors.New("capture: a natural key is required — unkeyed capture cannot be idempotent")
	}
	if rec.CapturedBy != actor.ID {
		// Provenance comes from the authenticated principal; a connector
		// cannot claim to be another one.
		return datasource.EntityRef{}, fmt.Errorf("capture: captured_by %q does not match the acting connector %q", rec.CapturedBy, actor.ID)
	}
	if err := admitCounterpartyShape(counterpartyShapeOf(rec.Counterparty)); err != nil {
		return datasource.EntityRef{}, err
	}
	if err := admitCounterpartyKeys(rec.Counterparty); err != nil {
		return datasource.EntityRef{}, err
	}

	var ref datasource.EntityRef
	var dedupeHit *ids.LeadID
	var dedupeFields json.RawMessage
	var activityCreated bool
	var decision counterpartyDecision
	// internalOnly is set INSIDE the transaction and read after it commits.
	// The skip has to commit — its breadcrumb is the proof that a message
	// produced no rows, and returning ErrSkip from inside the callback would
	// roll that proof back along with everything else (ADR-0082 §1).
	var internalOnly bool
	// dropped says why, when a gate above the raw store kept the message out;
	// it is the skip's sentence to the connector, so it names the rule, never
	// an address.
	var dropped string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A channel record's account id IS personal data, and THIS transaction is
		// the one that makes it durable — so the erasure is excluded here, under
		// the account's own lock, and not only at the ingress edge that admitted
		// the update. sinkchannel.go states what landing inside an erasure costs.
		if err := s.refuseErasedChannelAccount(ctx, tx, rec.Counterparty); err != nil {
			return err
		}

		// The pre-store gates run BEFORE the raw store, which is the whole
		// point: raw capture is append-once evidence, so a message that gets
		// that far has been kept whatever happens next.
		drop, err := s.dropBeforeStoreTx(ctx, tx, rec)
		if err != nil {
			return err
		}
		if drop != "" {
			internalOnly = true
			dropped = drop
			return nil
		}

		if err := storeRawCapture(ctx, tx, rec); err != nil {
			return err
		}

		switch fields := rec.Fields.(type) {
		case ActivityFields:
			// BEFORE the activity is captured, so a message that completes the
			// corroboration is judged under the claim it just proved rather
			// than being the last one read as mail from a stranger.
			if err := noteAliasSightingTx(ctx, tx, actor.UserID, rec.DeliveredTo, rec.Source); err != nil {
				return err
			}
			var err error
			ref, activityCreated, decision, err = s.captureActivity(ctx, tx, rec, fields)
			return err
		case LeadFields:
			var err error
			ref, dedupeHit, dedupeFields, err = s.captureLead(ctx, tx, rec, fields)
			return err
		default:
			return fmt.Errorf("capture: unmapped Fields type %T for %s", rec.Fields, rec.EntityType)
		}
	})
	if err != nil {
		s.traceInvisibleIncumbent(ctx, rec, err)
		return datasource.EntityRef{}, err
	}
	if internalOnly {
		// The breadcrumb is committed; only now does the connector learn the
		// message was dropped. ErrSkip is counted by every sync loop and fails
		// none of them, so the watermark advances past the message exactly as it
		// does for any other skip — which is what makes the classification
		// irreversible, and why the own-domain set is admin-visible (ADR-0082 §4).
		return datasource.EntityRef{}, fmt.Errorf("%w: %s", connector.ErrSkip, dropped)
	}
	if activityCreated {
		// The tier ladder already decided, and recorded its decision, inside
		// the transaction above. Creation runs AFTER that commit, in its own
		// transaction: the timeline row is never lost to a resolver fault, and
		// a fault here is logged for the link_reconcile sweep rather than
		// surfaced as a capture failure (the 60s p95 already delivered).
		s.ensureCounterparty(ctx, rec, ref, decision)
		// The project ladder runs on the same terms and for the same reasons:
		// after the commit, in its own transaction, never failing the capture.
		// It is independent of the counterparty decision — a message from a
		// sender no record was created for still belongs to the project its
		// subject names.
		s.attributeProject(ctx, rec, ref)
	}
	if dedupeHit != nil && s.stager != nil {
		// Staged OUTSIDE the capture transaction on purpose: the capture
		// itself wrote nothing (the collision blocked it), and the
		// proposal must survive independently for the inbox.
		if _, err := s.stager.StageMerge(ctx, MergeProposal{
			TargetType:     "lead",
			TargetID:       dedupeHit.UUID,
			ProposedChange: dedupeFields,
			Summary:        fmt.Sprintf("Captured %s/%s duplicates an existing lead", rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID),
		}); err != nil {
			return datasource.EntityRef{}, fmt.Errorf("capture: staging the dedupe merge: %w", err)
		}
	}
	return ref, nil
}

// skipInvisibleIncumbent refuses a record whose incumbent row — the lead an
// address collides with, the activity a replayed natural key already landed
// as — lies outside the granting human's authority over it. Resolving it is not
// the connector's to do: returning the ref would disclose a row the caller
// cannot read, folding the capture onto a row they hold only a `read` share of
// would let the connector make a change they could not make themselves, and
// writing a second row anyway would fork the record across scopes.
// The natural key names the skip, never the captured address or the
// incumbent's id — a skip must re-store neither PII nor an existence proof.
func skipInvisibleIncumbent(rec connector.NormalizedRecord, object string) error {
	return fmt.Errorf("capture: %s/%s resolves onto a %s: %w: %w",
		rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID, object, errInvisibleIncumbent, connector.ErrSkip)
}

// errInvisibleIncumbent distinguishes this skip from the other one the capture
// transaction can raise. Both wrap connector.ErrSkip, and the caller must tell
// them apart: this one is traced, because from the member's side a message they
// can see in their own mailbox simply never arrives and they deserve to know
// why. The erased-channel skip is NOT traced -- writing one would re-store what
// the erasure just removed.
var errInvisibleIncumbent = errors.New("the incumbent row is outside the granting human's authority over it")
