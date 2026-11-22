import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { useLawfulBasisGate } from "../../components/lawfulbasiswarning";
import { SendPermission } from "../../components/sendpermission";
import {
  DecisionCard,
  DecisionStatusChip,
  type DecisionStatusLabels,
} from "../../design-system/decisioncard";
import { provenanceOf } from "../../design-system/provenanceof";
import { useToast } from "../../design-system/toast";
import { AutonomyDot } from "../../design-system/trust";
import { formatCountdown, useNow } from "../../format/now";
import type { Locale, Translator } from "../../i18n";
import { useLocale, usePlural, useT } from "../../i18n";
import { editedCommercialPreview } from "../../lib/approvalpreview";
import {
  type Approval,
  invalidateApprovalViews,
} from "../../lib/approvals.queries";
import { approvalDotTier, useAgentTierMap } from "../../lib/autonomy";
import { recordRoute } from "../../lib/entity";
import { hideHomeRows } from "../../lib/homerows";
import {
  isAlreadyDecided,
  isVersionSkew,
  ProblemError,
  unwrap,
} from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import { useUnsavedGuard } from "../../lib/unsavedclaims";
import { useSendPermission } from "../../lib/usesendpermission";
import { ApprovalDetailModal } from "./approvaldetail";
import {
  cardRecords,
  DecideOutcome,
  editableSeed,
  StagedEditDialog,
} from "./approvaleditor";
import { ApprovalEvidence } from "./approvalevidence";
import {
  approvalAcceptLabel,
  approvalExpiryLabel,
  approvalLabel,
  decisionSkewKey,
  editableStrings,
  requiredMailFields,
} from "./approvalkind";
import {
  announceApproval,
  editedAwayFromProposal,
  MailboxBlocked,
} from "./approvalmail";
import { rowLabels, useApprovalDisplay } from "./approvalproposal";
import { approvalAcceptable, mailCannotBeSent } from "./approvalresponsibility";
import { stagedSendOf } from "./approvalsend";
import { ApprovalDecider, ApprovalTarget } from "./approvaltarget";
import {
  heldRejectionBar,
  refusalToast,
  useHeldRejection,
} from "./heldrejection";
import { LegalReviewRequestRow } from "./legalreviewrequestrow";
import { LinkedInMatchChoice, useLinkedInChoice } from "./linkedinmatchchoice";
import "./approvalrow.css";

// One staged proposal as a decidable row, and the screen-level sink that
// carries the one outcome the row cannot show itself.
//
// The row is the CANONICAL 🟡 affordance: per-row approve/reject plus the
// staged-draft editor, in a dialog. String fields of the proposed_change are editable
// and go up as edited_payload, which the server RE-ADMITS from scratch
// (re-tiered, re-RBAC'd, new diff_hash — ADR-0036); an edit can never silently
// escalate the effect. A 409 version-skew comes back as an honest "the world
// changed, re-stage" row error; a 409 already-decided drops the stale row
// instead of offering a re-stage retry.
//
// It lives beside the approvals queries rather than inside any one screen
// because several surfaces draw it: the workspace-wide decisions queue, Brief,
// and the company record's decisions panel.

// The status chip's words. Spelled per surface like every other label bag here:
// what a countdown or a verdict is CALLED belongs to the screen showing it.
function statusLabels(
  t: Translator,
  locale: Locale,
  kind: string,
): DecisionStatusLabels {
  return {
    expiresIn: (msRemaining) =>
      approvalExpiryLabel(kind, formatCountdown(msRemaining, t, locale), t),
    approved: t("decision.status.approved"),
    rejected: t("decision.status.rejected"),
    expired: t("decision.status.expired"),
  };
}

function findEditOpener(cardTestId: string, editLabel: string) {
  const card = `[data-testid="${cardTestId}"]`;
  return (
    document.querySelector<HTMLElement>(
      `${card} [aria-label="${editLabel}"]`,
    ) ?? document.querySelector<HTMLElement>(`${card} button`)
  );
}

function DecidableApprovalRow({
  approval,
  decided,
  onAlreadyDecided,
  extraInvalidateKeys,
}: Readonly<{
  approval: Approval;
  decided?: boolean;
  // Lift the already-decided signal to a surface that survives this row's
  // unmount (the pending invalidation drops it). Optional so a surface can
  // reuse the row without a screen-level surface.
  onAlreadyDecided?: () => void;
  // Reads outside the approvals list that a decision also changes. A record
  // page carrying its own count of what is waiting has to re-read it, and only
  // the caller knows which record that is.
  extraInvalidateKeys?: readonly QueryKey[];
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const queryClient = useQueryClient();
  const tierMap = useAgentTierMap();
  // No ownership question left to answer. The offer outlives this row —
  // approving invalidates ["approvals"], which unmounts the row that showed it
  // — and the region it renders into belongs to the application rather than to
  // whichever surface happened to be on screen, so the message survives on its
  // own. The prop that used to hand a controller down from the queue, and the
  // conditional region under it, went with that.
  const toast = useToast();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [detailOpen, setDetailOpen] = useState(false);
  // Only a live pending row with an expiry needs the per-second clock; a
  // read-only decided row (or one without expires_at) never shows a countdown,
  // so its interval is disabled — no needless per-second re-renders on long
  // Decided lists (interval 0 ⇒ useNow does not tick).
  const needsCountdown = !decided && approval.expires_at != null;
  const now = useNow(needsCountdown ? 1000 : 0);
  // Whether the engine would let the staged mail go, asked before the approver
  // releases it. An approver is deciding on somebody else's behalf, and until
  // now learned of a refusal the way the author would have: by pressing Accept
  // and reading the effect's failure. Only a live row asks — a decided one has
  // nothing left to release — and only a kind whose payload describes a mail.
  const staged = stagedSendOf(approval);
  const permission = useSendPermission({
    recipients: staged?.recipients ?? [],
    anchorActivityId: staged?.anchorActivityId,
    links: staged?.links,
    context: staged?.context,
    marketingPurpose: staged?.marketingPurpose,
    consentPurpose: staged?.consentPurpose,
    evidence: staged?.evidence,
    draft: staged?.draft,
    enabled: !decided && staged !== undefined,
  });

  // The server marks a mail its sender cannot send. approvalAcceptable then
  // withholds the verbs here and on every other surface that decides it.
  const mailboxBlocked = mailCannotBeSent(approval, decided);

  // An agent-drafted mail to a recipient with no lawful basis on record waits
  // for the approver to confirm the warning, as the composer does.
  const lawfulBasis = useLawfulBasisGate(permission.preview);

  // Where an approved change can be put back, and where it cannot.
  //
  // The undo is the RECORD's, not the approval's: an effect that changes a
  // field writes an ordinary `update` audit row on the record it changed, and
  // the history panel already reverses one of those (compose/undoability.go
  // judges it, and refuses with a named reason where it cannot). So this offers
  // the way there rather than a second reversal engine.
  //
  // Only for an approval naming a record type that panel serves — recordRoute
  // above decides which. A kind that sends mail, creates a record, or names no
  // target at all has nothing to put back, and a button leading to a page with
  // no entry for it would be an offer the product cannot keep.
  const undoTarget = recordRoute(
    approval.target_entity_type,
    approval.target_entity_id,
  );

  const decide = useMutation({
    mutationFn: async (input: {
      verdict: "approve" | "reject";
      editedPayload?: Record<string, unknown>;
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approvals/{id}/approve"
          : "/approvals/{id}/reject";
      return unwrap(
        await api.POST(path, {
          params: { path: { id: approval.id } },
          // A held rejection released on pagehide has to outlive the page.
          keepalive: input.verdict === "reject",
          ...(input.verdict === "approve" && input.editedPayload
            ? { body: { edited_payload: input.editedPayload } }
            : {}),
        }),
      );
    },
    onMutate: () =>
      hideHomeRows(queryClient, [{ source: "approval", id: approval.id }]),
    onSettled: (_data, _error, _input, settle) => settle?.(),
    onSuccess: (data, input) => {
      void invalidateApprovalViews(queryClient);
      for (const queryKey of extraInvalidateKeys ?? []) {
        queryClient.invalidateQueries({ queryKey });
      }
      if (input.verdict === "approve")
        announceApproval(toast, t, data, undoTarget);
    },
    onError: (error) => {
      const problem = error instanceof ProblemError ? error.problem : null;
      const alreadyDecided = problem && isAlreadyDecided(problem);
      if (alreadyDecided) onAlreadyDecided?.();
      if (alreadyDecided || approval.kind === "commitment_task") {
        void invalidateApprovalViews(queryClient);
      }
    },
  });

  // A deadline that lapses inside the window still sends: the server judges
  // expiry. A refusal shows on the card, or as a toast once the card is gone.
  const held = useHeldRejection({
    send: () => decide.mutateAsync({ verdict: "reject" }),
    decided: !!decided,
    onRefusedAfterLeaving: refusalToast(toast, t),
  });

  const change = approval.proposed_change ?? {};
  const linkedIn = useLinkedInChoice(approval);
  const strings = editableStrings(approval.kind, change);
  const display = useApprovalDisplay(
    approval,
    editedCommercialPreview(approval.preview, draft, editing),
  );

  const problem =
    decide.error instanceof ProblemError ? decide.error.problem : null;
  const skewKey =
    problem && isVersionSkew(problem) ? decisionSkewKey(approval.kind) : null;
  const alreadyDecided = problem ? isAlreadyDecided(problem) : false;

  // The draft is seeded with what the CONTROL will show, not with what the
  // payload holds. A date control silently discards a value it cannot parse, so
  // seeding the raw string would leave the reader looking at an empty box while
  // the original value still rode out on approve — the editor showing one thing
  // and submitting another.
  const startEdit = () => {
    setDraft(
      Object.fromEntries(
        strings.map((entry) => [
          entry.field,
          editableSeed(entry, change[entry.field]),
        ]),
      ),
    );
    setEditing(true);
  };

  const acceptable = approvalAcceptable(approval);
  // A mail must say something: the fields it cannot send without.
  const required = requiredMailFields(staged !== undefined, strings);

  // The editor closes only once the server took the decision, so a refusal
  // leaves the typed text where it was.
  // Typed text the decision panel's Escape, X and backdrop must ask about.
  useUnsavedGuard(
    editedAwayFromProposal(editing, strings, change, draft),
    `approval-edit:${approval.id}`,
  );

  const approveEdited = () => {
    if (!acceptable) return;
    lawfulBasis.gate(() => {
      held.announce();
      decide.mutate(
        {
          verdict: "approve",
          editedPayload: { ...change, ...draft },
        },
        { onSuccess: () => setEditing(false) },
      );
    });
  };

  const reRead = () => {
    void invalidateApprovalViews(queryClient);
    decide.reset();
  };

  const outcomeShown = (shown: boolean) => (
    <DecideOutcome
      shown={shown}
      decide={decide}
      skewKey={skewKey}
      alreadyDecided={alreadyDecided}
      onReRead={reRead}
    />
  );
  const cardTestId = `approval-card-${approval.id}`;
  // The card's verbs are not mounted while the editor is open, so the dialog
  // names the Edit control it will find once they are back.
  const editOpener = () => findEditOpener(cardTestId, t("trust.edit"));

  return (
    <DecisionCard
      approval={approval}
      layout="row"
      testId={cardTestId}
      className="approval-row"
      display={display}
      detail={
        <>
          <ApprovalTarget approval={approval} />
          <ApprovalDecider approval={approval} />
          <ApprovalEvidence approval={approval} />
        </>
      }
      now={now}
      labels={{
        ...rowLabels(t),
        accept: approvalAcceptLabel(
          approval.kind,
          plural,
          t(
            ["send_email", "held_draft", "scheduled_send_held"].includes(
              approval.kind,
            )
              ? "brief.approval.email"
              : "brief.approval.approve",
          ),
          locale,
          1,
          typeof approval.proposed_change?.existing_task_id === "string",
        ),
      }}
      decided={decided}
      pending={decide.isPending}
      provenance={provenanceOf(approval.proposed_by, viewerId)}
      meta={
        <>
          {!decided && (
            <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
          )}
          {/* kind is meta, not the headline — the human reads the summary first */}
          <span className="t-caption">{approvalLabel(approval, t)}</span>
          <DecisionStatusChip
            approval={approval}
            decided={!!decided}
            now={now}
            labels={statusLabels(t, locale, approval.kind)}
          />
        </>
      }
      aside={
        /* lighter, secondary affordance — must not compete with Accept/Reject */
        <button
          type="button"
          className="link-button"
          onClick={() => setDetailOpen(true)}
        >
          {t("decision.detail")}
        </button>
      }
      editor={
        editing ? (
          <StagedEditDialog
            outcome={outcomeShown(true)}
            returnFocusTo={editOpener}
            unsaved={editedAwayFromProposal(editing, strings, change, draft)}
            disabled={!acceptable}
            required={required}
            acceptLabel={approvalAcceptLabel(
              approval.kind,
              plural,
              t("decision.approveEdited"),
              locale,
              1,
              typeof approval.proposed_change?.existing_task_id === "string",
            )}
            fields={strings}
            records={cardRecords(approval.proposed_change)}
            draft={draft}
            onChange={(field, value) =>
              setDraft((current) => ({ ...current, [field]: value }))
            }
            pending={decide.isPending}
            onApprove={approveEdited}
            onCancel={() => setEditing(false)}
          />
        ) : undefined
      }
      onAccept={
        acceptable
          ? () =>
              lawfulBasis.gate(() => {
                held.announce();
                decide.mutate({
                  verdict: "approve",
                  editedPayload: linkedIn?.edited,
                });
              })
          : undefined
      }
      // Offered only where the kind has something a reader may change. Disabled
      // with Reject while a verdict is out, and for a sharper reason than
      // symmetry: it opens an editor whose own Cancel is disabled while the
      // write is in flight, so a press here during it left the reader inside a
      // form they could not leave until the request came back.
      onEdit={strings.length > 0 ? startEdit : undefined}
      // No reason form: a rejection discards a proposal and changes no record.
      // The undo window is the only step between the press and the send.
      onReject={held.hold}
      heldRejection={heldRejectionBar(t, held)}
      notice={
        <>
          {/* Read-only: the approver may release or refuse the message, and an
              override of the engine belongs to whoever is writing to the
              contact, recorded against their own name. */}
          <LinkedInMatchChoice
            offer={linkedIn}
            decided={decided}
            disabled={decide.isPending}
          />
          <MailboxBlocked blocked={mailboxBlocked} />
          {!decided && staged !== undefined && (
            <SendPermission
              preview={permission.preview}
              asking={permission.asking}
              unanswered={permission.unanswered}
            />
          )}
          {outcomeShown(!editing)}
        </>
      }
    >
      <ApprovalDetailModal
        approvalId={approval.id}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
      />
      {lawfulBasis.dialog}
    </DecisionCard>
  );
}

/** One staged proposal as a row. A legal review request has its own presentation. */
export function ApprovalRow(props: Parameters<typeof DecidableApprovalRow>[0]) {
  if (props.approval.kind === "legal_review_request") {
    return (
      <LegalReviewRequestRow
        approval={props.approval}
        decided={props.decided}
        extraInvalidateKeys={props.extraInvalidateKeys}
      />
    );
  }
  return <DecidableApprovalRow {...props} />;
}
