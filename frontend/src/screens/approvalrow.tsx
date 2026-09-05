import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useCallback, useState } from "react";
import { api } from "../api/client";
import {
  approvalDotTier,
  KIND_TO_VERB,
  useAgentTierMap,
} from "../app/autonomy";
import { ENTITY, isEntityKind } from "../app/entity";
import { navigate, type Route } from "../app/router";
import { Button, Card } from "../design-system/atoms";
import {
  DecisionCard,
  type DecisionCardLabels,
  DecisionStatusChip,
  type DecisionStatusLabels,
  DecisionToolChip,
} from "../design-system/decisioncard";
import { useToast } from "../design-system/toast";
import { AutonomyDot, confidenceLevel } from "../design-system/trust";
import { formatCountdown, useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import type { Locale, Translator } from "../i18n";
import { useLocale, useT } from "../i18n";
import {
  ApprovalDetailModal,
  DecideOutcome,
  editableSeed,
  editableStrings,
  StagedEditor,
} from "./approvaleditor";
import {
  approvalKindLabel,
  resolveDisplay,
  stagedDayFormatter,
} from "./approvalkind";
import type { Approval } from "./approvals.queries";
import {
  isAlreadyDecided,
  isVersionSkew,
  ProblemError,
  provenanceOf,
  throwProblem,
  useViewerId,
} from "./common";
import "./approvalrow.css";

// One staged proposal as a decidable row, and the screen-level sink that
// carries the one outcome the row cannot show itself.
//
// The row is the CANONICAL 🟡 affordance: per-row approve/reject plus the
// inline staged-draft editor. String fields of the proposed_change are editable
// and go up as edited_payload, which the server RE-ADMITS from scratch
// (re-tiered, re-RBAC'd, new diff_hash — ADR-0036); an edit can never silently
// escalate the effect. A 409 version-skew comes back as an honest "the world
// changed, re-stage" row error; a 409 already-decided drops the stale row
// instead of offering a re-stage retry.
//
// It lives beside the approvals queries rather than inside any one screen
// because several surfaces draw it: the workspace-wide decisions queue, Home,
// and the company record's decisions panel.

// Shared decision sink (AC-6, cross-surface): owns the screen-level state that
// must OUTLIVE the row that triggered it — a decide invalidates the pending
// list, which unmounts the row before it could render its own note. Every
// surface drawing an ApprovalRow consumes it so it shows the honest
// already-decided note; it may not live in ApprovalRow (it unmounts).
//
// The approve response still carries an approval token. It is not shown to
// anybody: an agent redeems its own staging by re-issuing the call under
// `X-Approval-Token`, and the runner's resume path loads the staged proposal
// server-side — so no human has ever needed to carry the string, while being
// handed an irrecoverable secret made every approval look like a key ceremony.
export function useDecisionSink(): {
  onAlreadyDecided: () => void;
  decidedNote: ReactNode;
} {
  const t = useT();
  const [alreadyDecided, setAlreadyDecided] = useState(false);
  const onAlreadyDecided = useCallback(() => setAlreadyDecided(true), []);
  const decidedNote = alreadyDecided ? (
    <Card
      as="div"
      inset
      style={{
        marginTop: "var(--space-3)",
        display: "flex",
        gap: "var(--space-2)",
        alignItems: "center",
      }}
    >
      <p className="t-caption" style={{ color: "var(--danger)", flex: 1 }}>
        {t("decision.alreadyDecided")}
      </p>
      <Button small onClick={() => setAlreadyDecided(false)}>
        {t("decision.dismiss")}
      </Button>
    </Card>
  ) : null;
  return { onAlreadyDecided, decidedNote };
}

// The words the decision card needs, in this surface's own vocabulary.
//
// Assembled once per render rather than inline, because the card takes them as
// one object and a caller that builds it in the JSX rebuilds it on every keystroke
// in the editor beside it.
//
// `skip` is deliberately absent: the queue is something somebody works to the end,
// and "later" on a surface whose whole purpose is to be emptied is a verb that
// only moves work sideways. The deck on Home is where later belongs.
//
// `showMore` / `showLess` are absent for a different reason and it is worth
// stating: the row clamps a long draft, and the way through to the whole of it on
// THIS surface is the detail dialog already on the row's own meta line. A second
// affordance for the same text, one line apart, is two answers to one question.
function rowLabels(t: Translator): DecisionCardLabels {
  return {
    accept: t("trust.accept"),
    edit: t("trust.edit"),
    reject: t("decision.reject"),
    expired: t("decision.expired"),
    draftSubject: t("decision.draftSubject"),
    draftBody: t("decision.draftBody"),
    noContent: t("common.empty"),
    loading: t("decision.detailLoading"),
  };
}

// The status chip's words. Spelled per surface like every other label bag here:
// what a countdown or a verdict is CALLED belongs to the screen showing it.
function statusLabels(t: Translator, locale: Locale): DecisionStatusLabels {
  return {
    expiresIn: (msRemaining) =>
      t("decision.expiresIn", {
        countdown: formatCountdown(msRemaining, t, locale),
      }),
    approved: t("decision.status.approved"),
    rejected: t("decision.status.rejected"),
    expired: t("decision.status.expired"),
  };
}

// The record page an approved change can be put back on, or undefined when
// there is none.
//
// isEntityKind is the same question the breadcrumb asks before it links, and it
// narrows the wire's free-form string to the five kinds with a record page. The
// history panel serves one more — `activity` — which has no page to route to,
// so a target of that kind is honestly not offered rather than linked into
// nothing.
export function recordRoute(
  entityType: string | null | undefined,
  entityID: string | null | undefined,
): Route | undefined {
  if (!entityID || !entityType || !isEntityKind(entityType)) {
    return undefined;
  }
  return ENTITY[entityType].route(entityID);
}

export function ApprovalRow({
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

  // Sticky, because this confirmation carries a verb and a reader reaching for
  // it must not lose it mid-reach. Only on approve: a rejection changed
  // nothing, so there is nothing to put back.
  const showUndoOffer = (verdict: "approve" | "reject") => {
    if (verdict !== "approve" || !undoTarget) {
      return;
    }
    // The verb goes through `action`: the region draws it on its own ground,
    // and withdraws the message once it has been pressed.
    toast.show(t("decision.applied"), {
      action: {
        label: t("decision.undoOnRecord"),
        onAct: () => navigate(undoTarget),
      },
    });
  };

  const decide = useMutation({
    mutationFn: async (input: {
      verdict: "approve" | "reject";
      editedPayload?: Record<string, unknown>;
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approvals/{id}/approve"
          : "/approvals/{id}/reject";
      const { data, error } = await api.POST(path, {
        params: { path: { id: approval.id } },
        ...(input.verdict === "approve" && input.editedPayload
          ? { body: { edited_payload: input.editedPayload } }
          : {}),
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_data, input) => {
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
      for (const queryKey of extraInvalidateKeys ?? []) {
        queryClient.invalidateQueries({ queryKey });
      }
      showUndoOffer(input.verdict);
    },
    onError: (error) => {
      const problem = error instanceof ProblemError ? error.problem : null;
      if (problem && isAlreadyDecided(problem)) {
        onAlreadyDecided?.();
        queryClient.invalidateQueries({ queryKey: ["approvals", "pending"] });
      }
    },
  });

  const change = approval.proposed_change ?? {};
  const strings = editableStrings(approval.kind, change);
  const display = resolveDisplay(
    approval.kind,
    change,
    t,
    stagedDayFormatter(locale, viewerZone()),
  );
  const level = confidenceLevel(approval.confidence);

  const problem =
    decide.error instanceof ProblemError ? decide.error.problem : null;
  const skew = problem ? isVersionSkew(problem) : false;
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

  const approveEdited = () => {
    decide.mutate({
      verdict: "approve",
      editedPayload: { ...change, ...draft },
    });
    setEditing(false);
  };

  const reRead = () => {
    queryClient.invalidateQueries({ queryKey: ["approvals", "pending"] });
    queryClient.invalidateQueries({ queryKey: ["approval", approval.id] });
    decide.reset();
  };

  return (
    <DecisionCard
      approval={approval}
      layout="row"
      className="approval-row"
      display={display}
      now={now}
      labels={rowLabels(t)}
      decided={decided}
      pending={decide.isPending}
      provenance={provenanceOf(approval.proposed_by, viewerId)}
      confidence={level ?? undefined}
      meta={
        <>
          {!decided && (
            <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
          )}
          {/* kind is meta, not the headline — the human reads the summary first */}
          <span className="t-small">{approvalKindLabel(approval.kind, t)}</span>
          <DecisionToolChip
            verb={KIND_TO_VERB[approval.kind]}
            label={(verb) => t("decision.viaTool", { verb })}
          />
          <DecisionStatusChip
            approval={approval}
            decided={!!decided}
            now={now}
            labels={statusLabels(t, locale)}
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
          <StagedEditor
            fields={strings}
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
      onAccept={() => decide.mutate({ verdict: "approve" })}
      // Offered only where the kind has something a reader may change. Disabled
      // with Reject while a verdict is out, and for a sharper reason than
      // symmetry: it opens an editor whose own Cancel is disabled while the
      // write is in flight, so a press here during it left the reader inside a
      // form they could not leave until the request came back.
      onEdit={strings.length > 0 ? startEdit : undefined}
      // One press, no reason form: a rejection discards a proposal and changes
      // no record, so it carries the same weight as Accept and asks nothing
      // extra. The contract's reason field stays for callers that have one.
      onReject={() => decide.mutate({ verdict: "reject" })}
      notice={
        <DecideOutcome
          decide={decide}
          skew={skew}
          alreadyDecided={alreadyDecided}
          onReRead={reRead}
        />
      }
    >
      <ApprovalDetailModal
        approvalId={approval.id}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
      />
    </DecisionCard>
  );
}
