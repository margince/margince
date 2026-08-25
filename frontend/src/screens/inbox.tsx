import {
  type QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useId, useState } from "react";
import { api } from "../api/client";
import {
  approvalDotTier,
  KIND_TO_VERB,
  useAgentTierMap,
} from "../app/autonomy";
import {
  Badge,
  Button,
  Card,
  Disclosure,
  Field,
  Modal,
  SegmentedControl,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  DecisionCard,
  type DecisionCardLabels,
  DecisionStatusChip,
  type DecisionStatusLabels,
  DecisionToolChip,
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgencyTone,
} from "../design-system/decisioncard";
import { Select } from "../design-system/select";
import {
  AutonomyDot,
  confidenceLevel,
  EvidenceChip,
  ProvenanceTag,
} from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { formatCountdown, type Translator, useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  approvalKindLabel,
  EDITABLE_FIELDS,
  type EditableField,
  humanizeKind,
} from "./approvalkind";
import {
  isAlreadyDecided,
  isVersionSkew,
  ProblemError,
  problemMessageOf,
  provenanceOf,
  QueryGate,
  throwProblem,
  useViewerId,
} from "./common";
import {
  type Approval,
  type BundleDecision,
  type BundleOutcome,
  useDecidedApprovals,
  usePendingApprovals,
} from "./inbox.queries";
import "./inbox.css";

// Re-exported so existing consumers (home.tsx) keep importing from "./inbox".
export { useDecidedApprovals, usePendingApprovals } from "./inbox.queries";

// The approval inbox (B-EP09.12a) — the CANONICAL 🟡 surface. Per-row
// approve/reject plus the inline staged-draft editor: string fields of the
// proposed_change are editable and go up as edited_payload, which the server
// RE-ADMITS from scratch (re-tiered, re-RBAC'd, new diff_hash — ADR-0036);
// an edit can never silently escalate the effect. A 409 version-skew comes
// back as an honest "the world changed, re-stage" row error; a 409
// already-decided drops the stale row instead of offering a re-stage retry
// (Task 10, AC-1..7).

// What the inline editor offers for one proposal.
//
// The default is every string field as a text box, which is the right shape
// for a payload whose values are prose. A kind that declares an EDITABLE_FIELDS
// policy gets exactly the fields it named, in the shape it named them —
// identifiers and enums are not prose, and offering them as free text asks a
// reader to type their way into a refusal.
function editableStrings(
  kind: string,
  change: Record<string, unknown>,
): EditableField[] {
  // Own-property only: `kind` is a wire string, and a value named `constructor`
  // would otherwise find a function on Object's prototype, pass the truthy
  // check, and crash the inbox on .filter instead of falling back to the
  // generic editor.
  const declared = Object.hasOwn(EDITABLE_FIELDS, kind)
    ? EDITABLE_FIELDS[kind]
    : undefined;
  if (declared) {
    // A declared field the payload does not carry is skipped rather than
    // rendered empty: an editor offering a field that is not in the change
    // would ADD it on approve, and the server reads an added path as a
    // retargeted edit.
    return declared.filter((entry) => typeof change[entry.field] === "string");
  }
  return Object.entries(change)
    .filter((entry): entry is [string, string] => typeof entry[1] === "string")
    .map(([field]) => ({ field, as: "text" }) as const);
}

// The per-claim evidence chips, shared by the row and the detail modal (was
// duplicated verbatim in both). A snippet-less evidence item is dropped.
//
// `source_lines` rides along with the snippet rather than being dropped: a
// proposal read out of a transcript is only checkable if the reader can find
// the exchange it came from, and a quoted sentence with no address is a claim
// they have to take on trust.
function EvidenceList({
  evidence,
}: Readonly<{ evidence: Approval["evidence"] }>) {
  return (
    <>
      {evidence?.map((item) =>
        item.evidence_snippet ? (
          <EvidenceChip
            key={`${item.source_id}-${item.evidence_snippet.slice(0, 12)}`}
            evidence={{
              snippet: item.evidence_snippet,
              source: item.source_type ?? "",
              lines: item.source_lines,
            }}
          />
        ) : null,
      )}
    </>
  );
}

// AC-2: the row's "view everything" affordance — the full proposed_change
// (key→value), evidence, target_version, proposed_by/on_behalf_of and
// timestamps the summary/evidence-chip row necessarily elides.
function ApprovalDetailModal({
  approvalId,
  open,
  onClose,
}: Readonly<{ approvalId: string; open: boolean; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  // An approval is a thing the reader must act on, so its timestamps belong on
  // the reader's own clock rather than the record's.
  const zone = viewerZone();
  const headingId = useId();
  const detail = useQuery({
    queryKey: ["approval", approvalId],
    enabled: open,
    queryFn: async () => {
      const { data, error } = await api.GET("/approvals/{id}", {
        params: { path: { id: approvalId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
        {t("inbox.detail")}
      </h2>
      {open && (
        <QueryGate query={detail}>
          {(approval) => {
            const change = (approval.proposed_change ?? {}) as Record<
              string,
              unknown
            >;
            // Wire field identifiers (contract shape), not translatable prose
            // — rendered raw, exactly like the proposed_change keys below.
            const meta: [string, string][] = [
              ["target_version", String(approval.target_version ?? "—")],
              ["proposed_by", approval.proposed_by],
            ];
            if (approval.on_behalf_of) {
              meta.push(["on_behalf_of", approval.on_behalf_of]);
            }
            meta.push([
              "created_at",
              formatDateTime(approval.created_at, locale, zone),
            ]);
            if (approval.decided_at) {
              meta.push([
                "decided_at",
                formatDateTime(approval.decided_at, locale, zone),
              ]);
            }
            return (
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {Object.entries(change).map(([key, value]) => (
                  <div className="field" key={key}>
                    <span className="t-label">{key}</span>
                    <p className="t-mono">
                      {typeof value === "string"
                        ? value
                        : JSON.stringify(value)}
                    </p>
                  </div>
                ))}
                <EvidenceList evidence={approval.evidence} />
                {meta.map(([key, value]) => (
                  <div className="field" key={key}>
                    <span className="t-label">{key}</span>
                    <p className="t-mono">{value}</p>
                  </div>
                ))}
              </div>
            );
          }}
        </QueryGate>
      )}
    </Modal>
  );
}

// The row-local decide outcomes that KEEP the row mounted: a generic error
// and the version-skew re-stage state. The success token (AC-4) and the
// already-decided note (AC-6) are deliberately NOT here — both fire a pending
// invalidation that unmounts this row, so they are surfaced at screen level
// (InboxScreen) where they survive the refetch.
function DecideOutcome({
  decide,
  skew,
  alreadyDecided,
  onReRead,
}: Readonly<{
  decide: { isError: boolean; error: unknown };
  skew: boolean;
  alreadyDecided: boolean;
  onReRead: () => void;
}>) {
  const t = useT();
  const generic = decide.isError && !skew && !alreadyDecided;
  return (
    <>
      {generic && (
        <p
          className="t-caption"
          style={{ color: "var(--danger)", marginTop: 8 }}
        >
          {problemMessageOf(decide.error, t)}
        </p>
      )}
      {skew && (
        <div style={{ marginTop: 8 }}>
          <p className="t-caption" style={{ color: "var(--danger)" }}>
            {t("inbox.versionSkew")}
          </p>
          <Button small onClick={onReRead}>
            {t("inbox.reRead")}
          </Button>
        </div>
      )}
    </>
  );
}

// The screen-level "shown once" approval-token surface (AC-4). Rendered by
// InboxScreen/HomeScreen — NOT the row — so the pending invalidation that
// unmounts the just-approved row cannot take the token with it. This is the
// most consequential irrecoverable state on the surface, so it leads with a
// strong heading + a warn-tinted banner, not a small gray caption.
function TokenOnceModal({
  token,
  onClose,
}: Readonly<{ token: string | null; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const [copied, setCopied] = useState(false);
  // A fresh token clears the previous "copied" acknowledgement (referencing
  // `token` in the body keeps it a genuine effect dependency).
  useEffect(() => {
    if (token != null) {
      setCopied(false);
    }
  }, [token]);
  const handleCopy = () => {
    if (!token) {
      return;
    }
    const clip = navigator.clipboard;
    if (!clip) {
      setCopied(false);
      return;
    }
    clip.writeText(token).then(
      () => setCopied(true),
      () => setCopied(false),
    );
  };
  return (
    <Modal open={token != null} onClose={onClose} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ color: "var(--textPrimary)", marginBottom: 10 }}
      >
        {t("inbox.tokenTitle")}
      </h2>
      <div
        style={{
          display: "flex",
          gap: 8,
          alignItems: "center",
          background: "var(--warnBg)",
          border: "1px solid var(--warnBorder)",
          borderRadius: "var(--r-sm)",
          padding: "8px 10px",
          marginBottom: 10,
        }}
      >
        <TriangleAlert size={16} color="var(--warn)" aria-hidden />
        <span className="t-caption" style={{ color: "var(--warn)" }}>
          {t("inbox.tokenOnce")}
        </span>
      </div>
      <p className="t-mono" style={{ wordBreak: "break-all" }}>
        {token}
      </p>
      <div className="actions">
        <Button small onClick={handleCopy}>
          {copied ? t("inbox.copied") : t("inbox.copy")}
        </Button>
        <Button small variant="primary" onClick={onClose}>
          {t("inbox.tokenDone")}
        </Button>
      </div>
    </Modal>
  );
}

// Shared decision sink (AC-4/AC-6, cross-surface): owns the screen-level state
// that must OUTLIVE the row that triggered it (a decide invalidates the
// pending list, unmounting the row) — the once-shown approval token and the
// "already decided by someone else" note. BOTH InboxScreen and HomeScreen
// consume it so either surface catches the minted token AND shows the honest
// already-decided note; neither may live in ApprovalRow (it unmounts).
export function useApprovalTokenSink(): {
  onApproved: (approvalId: string, token: string) => void;
  onAlreadyDecided: () => void;
  tokenModal: ReactNode;
  decidedNote: ReactNode;
} {
  const t = useT();
  const [token, setToken] = useState<string | null>(null);
  const [alreadyDecided, setAlreadyDecided] = useState(false);
  const onApproved = useCallback(
    (_approvalId: string, minted: string) => setToken(minted),
    [],
  );
  const onAlreadyDecided = useCallback(() => setAlreadyDecided(true), []);
  const tokenModal = (
    <TokenOnceModal token={token} onClose={() => setToken(null)} />
  );
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
        {t("inbox.alreadyDecided")}
      </p>
      <Button small onClick={() => setAlreadyDecided(false)}>
        {t("inbox.dismiss")}
      </Button>
    </Card>
  ) : null;
  return { onApproved, onAlreadyDecided, tokenModal, decidedNote };
}

// The words the decision card needs, in this surface's own vocabulary.
//
// Assembled once per render rather than inline, because the card takes them as
// one object and a caller that builds it in the JSX rebuilds it on every keystroke
// in the editor beside it.
//
// `skip` is deliberately absent: the inbox is a queue somebody works to the end,
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
    reject: t("inbox.reject"),
    expired: t("inbox.expired"),
    draftSubject: t("inbox.draftSubject"),
    draftBody: t("inbox.draftBody"),
    noContent: t("common.empty"),
  };
}

// The status chip's words. Spelled per surface like every other label bag here:
// what a countdown or a verdict is CALLED belongs to the screen showing it.
function statusLabels(t: Translator): DecisionStatusLabels {
  return {
    expiresIn: (msRemaining) =>
      t("inbox.expiresIn", { countdown: formatCountdown(msRemaining, t) }),
    approved: t("inbox.status.approved"),
    rejected: t("inbox.status.rejected"),
    expired: t("inbox.status.expired"),
  };
}

// The inline staged-draft editor: the string fields of the proposed_change, in
// the shape the kind declared for them, going up as `edited_payload`.
//
// It is handed to the card as its `editor` slot rather than living inside the
// primitive, and that boundary is the point: an edit re-enters the admission gate
// from scratch on the server (re-tiered, re-RBAC'd, new diff_hash — ADR-0036), so
// what it may offer is a question about THIS kind and this contract, which is
// screen knowledge. The card knows how to draw a decision; it does not know what
// this installation lets a person change.
function StagedEditor({
  fields,
  draft,
  onChange,
  pending,
  onApprove,
  onCancel,
}: Readonly<{
  fields: readonly EditableField[];
  draft: Record<string, string>;
  onChange: (field: string, value: string) => void;
  pending: boolean;
  onApprove: () => void;
  onCancel: () => void;
}>) {
  const t = useT();
  return (
    <div className="approval-editor">
      {fields.map((entry) => (
        <Field
          key={entry.field}
          label={entry.label ? t(entry.label) : entry.field}
        >
          {(control) =>
            entry.as === "choice" ? (
              <Select
                {...control}
                options={entry.options.map((option) => {
                  // The VALUE stays the wire enum — it is what gets submitted.
                  // Only what the reader sees is translated, and an option the
                  // kind declared no label for degrades to its own words rather
                  // than to an identifier.
                  const label = entry.optionLabels?.[option];
                  return {
                    value: option,
                    label: label ? t(label) : humanizeKind(option),
                  };
                })}
                value={draft[entry.field] ?? ""}
                onChange={(value) => onChange(entry.field, value)}
              />
            ) : entry.as === "textarea" ? (
              <Textarea
                {...control}
                rows={12}
                value={draft[entry.field] ?? ""}
                onChange={(event) => onChange(entry.field, event.target.value)}
              />
            ) : (
              <TextInput
                {...control}
                value={draft[entry.field] ?? ""}
                onChange={(event) => onChange(entry.field, event.target.value)}
              />
            )
          }
        </Field>
      ))}
      <div className="approval-gate">
        {/* The edited approve is the same write as the plain one and was the one
            path with no gate at all, so a second press sent a second verdict. */}
        <Button variant="primary" small pending={pending} onClick={onApprove}>
          {t("inbox.approveEdited")}
        </Button>
        <Button small disabled={pending} onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
      </div>
    </div>
  );
}

export function ApprovalRow({
  approval,
  decided,
  onApproved,
  onAlreadyDecided,
  extraInvalidateKeys,
}: Readonly<{
  approval: Approval;
  decided?: boolean;
  // Lift the just-minted token / the already-decided signal to a surface that
  // survives this row's unmount (the pending invalidation drops it). Optional
  // so HomeScreen can reuse the row without a screen-level surface.
  onApproved?: (approvalId: string, token: string) => void;
  onAlreadyDecided?: () => void;
  // Reads outside the approvals list that a decision also changes. A record
  // page carrying its own count of what is waiting has to re-read it, and only
  // the caller knows which record that is.
  extraInvalidateKeys?: readonly QueryKey[];
}>) {
  const t = useT();
  const viewerId = useViewerId();
  const queryClient = useQueryClient();
  const tierMap = useAgentTierMap();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const [detailOpen, setDetailOpen] = useState(false);
  // Only a live pending row with an expiry needs the per-second clock; a
  // read-only decided row (or one without expires_at) never shows a countdown,
  // so its interval is disabled — no needless per-second re-renders on long
  // Decided lists (interval 0 ⇒ useNow does not tick).
  const needsCountdown = !decided && approval.expires_at != null;
  const now = useNow(needsCountdown ? 1000 : 0);

  const decide = useMutation({
    mutationFn: async (input: {
      verdict: "approve" | "reject";
      editedPayload?: Record<string, unknown>;
      reason?: string;
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
        ...(input.verdict === "reject"
          ? { body: { reason: input.reason ?? "" } }
          : {}),
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      // Lift the token FIRST — the parent state is set before the invalidation
      // below unmounts this row, so the screen-level surface always receives it.
      if (data?.approval_token) {
        onApproved?.(approval.id, data.approval_token);
      }
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
      for (const queryKey of extraInvalidateKeys ?? []) {
        queryClient.invalidateQueries({ queryKey });
      }
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
  const level = confidenceLevel(approval.confidence);

  const problem =
    decide.error instanceof ProblemError ? decide.error.problem : null;
  const skew = problem ? isVersionSkew(problem) : false;
  const alreadyDecided = problem ? isAlreadyDecided(problem) : false;

  const startEdit = () => {
    setDraft(
      Object.fromEntries(
        strings.map((entry) => [
          entry.field,
          String(change[entry.field] ?? ""),
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

  const confirmReject = () => {
    decide.mutate({ verdict: "reject", reason });
    setRejecting(false);
    setReason("");
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
            label={(verb) => t("inbox.viaTool", { verb })}
          />
          <DecisionStatusChip
            approval={approval}
            decided={!!decided}
            now={now}
            labels={statusLabels(t)}
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
          {t("inbox.detail")}
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
      onReject={() => setRejecting(true)}
      notice={
        <DecideOutcome
          decide={decide}
          skew={skew}
          alreadyDecided={alreadyDecided}
          onReRead={reRead}
        />
      }
    >
      <ConfirmModal
        open={rejecting}
        onClose={() => setRejecting(false)}
        title={t("inbox.reject")}
        confirmLabel={t("inbox.reject")}
        confirmVariant="danger"
        pending={decide.isPending}
        onConfirm={confirmReject}
      >
        <Field
          label={t("inbox.rejectReason")}
          hint={t("inbox.rejectReasonHint")}
        >
          {(control) => (
            <Textarea
              {...control}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
      <ApprovalDetailModal
        approvalId={approval.id}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
      />
    </DecisionCard>
  );
}

// One act's proposals, or one proposal on its own — what the pending queue is
// actually a list of.
//
// The server stamps every proposal ONE act staged with a shared `bundle_id` (a
// site read publishes the company's facts plus a lead per person it found), so a
// read of a ten-person team page arrives as eleven rows. Rendered flat that is
// eleven questions for one act, which is the whole defect: the reader cannot see
// that answering them is one decision.
//
// A bundle of ONE is deliberately not a group. A group holding a single child
// hides the very question it exists to present, and the reader gains nothing for
// the click — so it reads as the plain row it is.
type InboxItem =
  | { readonly kind: "single"; readonly approval: Approval }
  | {
      readonly kind: "bundle";
      readonly bundleId: string;
      readonly members: readonly Approval[];
    };

// Groups the queue by act WITHOUT reordering it: each bundle is emitted at the
// position of its first member, so the order the caller sorted (oldest staged
// first) still decides where a group sits.
function groupByBundle(approvals: readonly Approval[]): InboxItem[] {
  const byBundle = new Map<string, Approval[]>();
  for (const approval of approvals) {
    if (approval.bundle_id) {
      const held = byBundle.get(approval.bundle_id);
      if (held) {
        held.push(approval);
      } else {
        byBundle.set(approval.bundle_id, [approval]);
      }
    }
  }
  const items: InboxItem[] = [];
  const emitted = new Set<string>();
  for (const approval of approvals) {
    const bundleId = approval.bundle_id;
    const members = bundleId ? byBundle.get(bundleId) : undefined;
    if (!bundleId || !members || members.length < 2) {
      items.push({ kind: "single", approval });
    } else if (!emitted.has(bundleId)) {
      emitted.add(bundleId);
      items.push({ kind: "bundle", bundleId, members });
    }
  }
  return items;
}

// What the group HOLDS, which is the only thing that makes it one thing: "4
// proposals" is a count nobody can answer, while "Read the company site · Add a
// person found on the site" says what saying yes would do. Kinds in the order
// the act staged them, each named once — how many there are is the line under
// it, and which ones they are is the list inside.
function bundleSubject(members: readonly Approval[], t: Translator): string {
  return [...new Set(members.map((member) => member.kind))]
    .map((kind) => approvalKindLabel(kind, t))
    .join(" · ");
}

// Who staged the act, shown only where every member agrees. One act has one
// author, so a disagreement means the grouping is not what it claims — and
// naming one member's proposer for all of them would be the wrong half of an
// honest answer.
function sharedProposer(members: readonly Approval[]): string | null {
  const first = members[0]?.proposed_by;
  return first && members.every((member) => member.proposed_by === first)
    ? first
    : null;
}

// The group's countdown: the SOONEST live member expiry, because that is when
// this act starts losing proposals. The copy says "first" — a bare countdown in
// a group header reads as one deadline shared by every member, and members keep
// their own expiries (the bundle is a grouping, never a second authority).
//
// `live` is the members that have not lapsed, so an empty list means this act
// has run out of time rather than that it never had a member.
function BundleExpiryChip({
  live,
  now,
}: Readonly<{ live: readonly Approval[]; now: number }>) {
  const t = useT();
  if (live.length === 0) {
    return <Badge tone="danger">{t("inbox.expired")}</Badge>;
  }
  const expiries = live
    .map(decisionExpiryMs)
    .filter((expiresAtMs): expiresAtMs is number => expiresAtMs != null);
  if (expiries.length === 0) {
    return null;
  }
  const remaining = Math.min(...expiries) - now;
  return (
    <Badge tone={decisionUrgencyTone(remaining)}>
      {t("inbox.bundle.expiresIn", {
        countdown: formatCountdown(remaining, t),
      })}
    </Badge>
  );
}

type BundleVerdict = "approve" | "reject";

// One act, as one question — with every member still reachable underneath it.
//
// The two bundle routes decide every still-pending member in one call and answer
// per member, so the collapsed card can carry the whole decision. What it must
// NOT carry is an edit: an edit re-hashes one diff and re-pins one entity, so
// there is no such thing as one edit spanning several proposals (the request
// body has no arm for it). That is why the members open as full rows rather than
// as a read-only manifest — a reader who wants to change one goes to it, decides
// it there, and answers the rest here.
function BundleCard({
  bundleId,
  members,
  onApproved,
  onAlreadyDecided,
  onDecided,
}: Readonly<{
  bundleId: string;
  members: readonly Approval[];
  onApproved?: (approvalId: string, token: string) => void;
  onAlreadyDecided?: () => void;
  // Lifts the per-member report to a surface that survives this card's unmount:
  // the decision invalidates the pending list, which takes the card with it.
  onDecided: (verdict: BundleVerdict, decision: BundleDecision) => void;
}>) {
  const t = useT();
  const viewerId = useViewerId();
  const queryClient = useQueryClient();
  const tierMap = useAgentTierMap();
  const [confirming, setConfirming] = useState<BundleVerdict | null>(null);
  const [reason, setReason] = useState("");
  // Only a group holding an expiry needs the per-second clock (interval 0 ⇒
  // useNow does not tick).
  const needsCountdown = members.some((member) => member.expires_at != null);
  const now = useNow(needsCountdown ? 1000 : 0);

  const decide = useMutation({
    mutationFn: async (input: {
      bundleId: string;
      verdict: BundleVerdict;
      reason: string;
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approval-bundles/{bundle_id}/approve"
          : "/approval-bundles/{bundle_id}/reject";
      const { data, error } = await api.POST(path, {
        params: { path: { bundle_id: input.bundleId } },
        // One stated reason for one decision, recorded on every member it
        // decides. An unstated one is omitted rather than sent empty: the
        // request body is closed, and a blank string would be recorded as
        // though somebody had written it.
        ...(input.reason ? { body: { reason: input.reason } } : {}),
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data, input) => {
      // Report FIRST — the invalidation below unmounts this card, so the
      // screen-level surface has to have the outcomes before it goes.
      onDecided(input.verdict, data);
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
    },
    onError: (error) => {
      const problem = error instanceof ProblemError ? error.problem : null;
      if (problem && isAlreadyDecided(problem)) {
        onAlreadyDecided?.();
        queryClient.invalidateQueries({ queryKey: ["approvals", "pending"] });
      }
    },
  });

  const subject = bundleSubject(members, t);
  const live = members.filter((member) => !decisionLapsed(member, now));
  const proposedBy = sharedProposer(members);
  // The autonomy dot every row carries, raised to the group only where the
  // members agree on it: one dot over a mix would claim a tier half of them do
  // not have, and the rows inside say it for themselves either way.
  const tiers = new Set(
    members.map((member) => approvalDotTier(member.kind, tierMap)),
  );
  const sharedTier = tiers.size === 1 ? [...tiers][0] : null;

  const send = (verdict: BundleVerdict, stated: string) => {
    decide.mutate({ bundleId, verdict, reason: stated });
    setConfirming(null);
    setReason("");
  };

  return (
    <Card as="article" className="approval-bundle" ariaLabel={subject}>
      <div className="approval-bundle-meta">
        {sharedTier && <AutonomyDot tier={sharedTier} />}
        {proposedBy && (
          <ProvenanceTag provenance={provenanceOf(proposedBy, viewerId)} />
        )}
        <BundleExpiryChip live={live} now={now} />
      </div>
      <p className="t-h2">{subject}</p>
      <p className="t-small approval-bundle-why">
        {t("inbox.bundle.why", { count: members.length })}
      </p>
      {live.length > 0 && (
        <div className="approval-gate">
          {/* Approve-all is what STARTED the write, so it is the control that
              goes busy; Reject-all is merely unavailable while a verdict is out
              and stays `disabled`. Both count the LIVE members: a lapsed one is
              not something this press can decide. */}
          <Button
            variant="primary"
            small
            pending={decide.isPending}
            onClick={() => setConfirming("approve")}
          >
            {t("inbox.bundle.approveAll", { count: live.length })}
          </Button>
          <Button
            small
            disabled={decide.isPending}
            onClick={() => setConfirming("reject")}
          >
            {t("inbox.bundle.rejectAll", { count: live.length })}
          </Button>
        </div>
      )}
      {decide.isError && (
        <p className="t-caption approval-bundle-error">
          {problemMessageOf(decide.error, t)}
        </p>
      )}
      <Disclosure
        className="approval-bundle-open"
        summary={t("inbox.bundle.members", { count: members.length })}
      >
        {/* A list, not an indent: the count and the boundaries of the group have
            to reach a reader who is hearing this page rather than seeing it. */}
        <ul className="approval-bundle-members">
          {members.map((member) => (
            <li key={member.id}>
              <ApprovalRow
                approval={member}
                onApproved={onApproved}
                onAlreadyDecided={onAlreadyDecided}
              />
            </li>
          ))}
        </ul>
      </Disclosure>
      <ConfirmModal
        open={confirming === "approve"}
        onClose={() => setConfirming(null)}
        title={t("inbox.bundle.approveAll", { count: live.length })}
        confirmLabel={t("trust.accept")}
        pending={decide.isPending}
        onConfirm={() => send("approve", "")}
      >
        <p className="t-small">{t("inbox.bundle.approveAllConfirm")}</p>
      </ConfirmModal>
      <ConfirmModal
        open={confirming === "reject"}
        onClose={() => setConfirming(null)}
        title={t("inbox.bundle.rejectAll", { count: live.length })}
        confirmLabel={t("inbox.reject")}
        confirmVariant="danger"
        pending={decide.isPending}
        onConfirm={() => send("reject", reason)}
      >
        <Field
          label={t("inbox.rejectReason")}
          hint={t("inbox.bundle.rejectReasonHint")}
        >
          {(control) => (
            <Textarea
              {...control}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </Card>
  );
}

type BundleResult = {
  readonly verdict: BundleVerdict;
  readonly decision: BundleDecision;
};

// The outcomes that are NOT a verdict this call recorded, each in the two number
// forms its sentence needs. Keyed off the wire enum minus `decided`, so an
// outcome the contract adds fails the type here rather than dropping silently
// out of a report whose whole job is to be complete.
const OUTCOME_KEYS: Readonly<
  Record<
    Exclude<BundleOutcome, "decided">,
    { readonly one: MessageKey; readonly other: MessageKey }
  >
> = {
  already_decided: {
    one: "inbox.bundle.result.alreadyDecided.one",
    other: "inbox.bundle.result.alreadyDecided.other",
  },
  expired: {
    one: "inbox.bundle.result.expired.one",
    other: "inbox.bundle.result.expired.other",
  },
  effect_failed: {
    one: "inbox.bundle.result.effectFailed.one",
    other: "inbox.bundle.result.effectFailed.other",
  },
};

// The order the report reads in: what somebody else settled, what ran out of
// time, and last the one that needs somebody to look at a record.
const REPORTED_OUTCOMES = [
  "already_decided",
  "expired",
  "effect_failed",
] as const satisfies readonly (keyof typeof OUTCOME_KEYS)[];

// How many members came back in each outcome.
function outcomeCounts(decision: BundleDecision): Map<BundleOutcome, number> {
  const counts = new Map<BundleOutcome, number>();
  for (const member of decision.data) {
    counts.set(member.outcome, (counts.get(member.outcome) ?? 0) + 1);
  }
  return counts;
}

// One sentence per outcome the response reported, verdict first.
//
// Deciding a bundle is not all-or-nothing, so "approved" on its own would be a
// claim the response does not make: a member that had lapsed, one somebody else
// had already answered, and one whose follow-on change failed to land each came
// back saying so, and each is a different thing for the reader to do next.
function outcomeLines(result: BundleResult, t: Translator): string[] {
  const counts = outcomeCounts(result.decision);
  const lines: string[] = [];
  const decided = counts.get("decided") ?? 0;
  if (decided > 0) {
    lines.push(
      t(
        result.verdict === "approve"
          ? "inbox.bundle.result.approved"
          : "inbox.bundle.result.rejected",
        { count: decided },
      ),
    );
  }
  for (const outcome of REPORTED_OUTCOMES) {
    const count = counts.get(outcome) ?? 0;
    if (count > 0) {
      const keys = OUTCOME_KEYS[outcome];
      lines.push(t(count === 1 ? keys.one : keys.other, { count }));
    }
  }
  return lines;
}

// The worst thing that happened, which is what the notice's tone has to say: an
// approved member whose change did not land leaves a record that did NOT change
// while its approval stands, and that outranks anything else in the report.
function outcomeTone(decision: BundleDecision): "danger" | "warn" | "success" {
  const outcomes = new Set(decision.data.map((member) => member.outcome));
  if (outcomes.has("effect_failed")) {
    return "danger";
  }
  return outcomes.has("already_decided") || outcomes.has("expired")
    ? "warn"
    : "success";
}

// What the decision actually did, member by member.
//
// Screen-level, like the minted token, because the decision invalidates the
// pending list and unmounts the card that made it.
function BundleOutcomeNote({
  result,
  onDismiss,
}: Readonly<{ result: BundleResult; onDismiss: () => void }>) {
  const t = useT();
  const lines = outcomeLines(result, t);
  // A report with nothing in it says nothing: a call that decided no member
  // answers 404, so there is no honest sentence to put in an empty box here.
  if (lines.length === 0) {
    return null;
  }
  const tone = outcomeTone(result.decision);
  return (
    <Callout
      tone={tone}
      live="status"
      className="approval-bundle-result"
      actions={
        <Button small onClick={onDismiss}>
          {t("inbox.dismiss")}
        </Button>
      }
    >
      {lines.map((line) => (
        <p key={line}>{line}</p>
      ))}
    </Callout>
  );
}

// The pending queue: one card per act, one row per proposal staged alone.
function PendingList({
  approvals,
  onApproved,
  onAlreadyDecided,
  onBundleDecided,
}: Readonly<{
  approvals: readonly Approval[];
  onApproved: (approvalId: string, token: string) => void;
  onAlreadyDecided: () => void;
  onBundleDecided: (verdict: BundleVerdict, decision: BundleDecision) => void;
}>) {
  return (
    <>
      {groupByBundle(approvals).map((item) =>
        item.kind === "bundle" ? (
          <BundleCard
            key={item.bundleId}
            bundleId={item.bundleId}
            members={item.members}
            onApproved={onApproved}
            onAlreadyDecided={onAlreadyDecided}
            onDecided={onBundleDecided}
          />
        ) : (
          <ApprovalRow
            key={item.approval.id}
            approval={item.approval}
            onApproved={onApproved}
            onAlreadyDecided={onAlreadyDecided}
          />
        ),
      )}
    </>
  );
}

export function InboxScreen() {
  const t = useT();
  const [tab, setTab] = useState<"pending" | "decided">("pending");
  // Screen-level surfaces that must outlive the row that triggered them (a
  // decide invalidates the pending list, unmounting the row): the once-shown
  // approval token (AC-4, via the shared sink) and the "already decided by
  // someone else" note (AC-6).
  const { onApproved, onAlreadyDecided, tokenModal, decidedNote } =
    useApprovalTokenSink();
  // The per-member report of a bundle decision, held here for the same reason:
  // the decision unmounts the card that made it.
  const [bundleResult, setBundleResult] = useState<BundleResult | null>(null);
  const pendingQuery = usePendingApprovals();
  const decidedQuery = useDecidedApprovals(tab === "decided");
  const query = tab === "pending" ? pendingQuery : decidedQuery;
  return (
    <div className="wrap">
      {/* .filter-tabs: the gap below the tabs holds for every query state —
          empty and loading bodies clear the tabs like a populated list does. */}
      <div className="filter-tabs">
        <SegmentedControl
          options={["pending", "decided"] as const}
          value={tab}
          onChange={setTab}
          labels={{
            pending: t("inbox.tab.pending"),
            decided: t("inbox.tab.decided"),
          }}
        />
      </div>
      {decidedNote}
      {bundleResult && (
        <BundleOutcomeNote
          result={bundleResult}
          onDismiss={() => setBundleResult(null)}
        />
      )}
      <QueryGate query={query} empty={(page) => page.data.length === 0}>
        {(page) =>
          tab === "pending" ? (
            <div className="approval-queue arrive-stack">
              <PendingList
                approvals={page.data}
                onApproved={onApproved}
                onAlreadyDecided={onAlreadyDecided}
                onBundleDecided={(verdict, decision) =>
                  setBundleResult({ verdict, decision })
                }
              />
            </div>
          ) : (
            // Decided stays ungrouped on purpose: it is history in the order the
            // verdicts were recorded, and each member carries its OWN verdict —
            // a group header over it would have to state a status the group does
            // not have (three approved, one rejected, one lapsed).
            <div className="approval-queue arrive-stack">
              {page.data.map((approval) => (
                <ApprovalRow
                  key={approval.id}
                  approval={approval}
                  decided
                  onApproved={onApproved}
                  onAlreadyDecided={onAlreadyDecided}
                />
              ))}
            </div>
          )
        }
      </QueryGate>
      {tokenModal}
    </div>
  );
}
