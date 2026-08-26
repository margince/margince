import { useQuery } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import {
  Button,
  Disclosure,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Select } from "../design-system/select";
import { EvidenceChip } from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  EDITABLE_FIELDS,
  type EditableField,
  humanizeKind,
  resolveDisplay,
  stagedDayFormatter,
} from "./approvalkind";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import type { Approval } from "./inbox.queries";

// The two slots an ApprovalRow hands to `DecisionCard`: the "view everything"
// dialog behind its meta line, and the inline staged-draft editor. They sit
// apart from the row because neither is about deciding — one reads the whole
// proposal, the other rewrites the part of it this installation lets a person
// change — and the row is already the longest thing on the surface.

/**
 * What the inline editor offers for one proposal.
 *
 * The default is every string field as a text box, which is the right shape
 * for a payload whose values are prose. A kind that declares an EDITABLE_FIELDS
 * policy gets exactly the fields it named, in the shape it named them —
 * identifiers and enums are not prose, and offering them as free text asks a
 * reader to type their way into a refusal.
 */
export function editableStrings(
  kind: string,
  change: Record<string, unknown>,
): EditableField[] {
  // Own-property only: `kind` is a wire string, and a value named `constructor`
  // would otherwise find a function on Object's prototype, pass the truthy
  // check, and crash the queue on .filter instead of falling back to the
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

/**
 * AC-2: the row's "view everything" affordance — the full proposed_change
 * (key→value), evidence, target_version, proposed_by/on_behalf_of and
 * timestamps the summary/evidence-chip row necessarily elides.
 */
export function ApprovalDetailModal({
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
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("inbox.detail")}
      </h2>
      {open && (
        <QueryGate query={detail}>
          {(approval) => (
            <ApprovalDetailBody
              approval={approval}
              locale={locale}
              zone={zone}
            />
          )}
        </QueryGate>
      )}
    </Modal>
  );
}

/**
 * What "view everything" shows.
 *
 * Ordered the way a person reads a decision rather than the way the row is
 * stored: the sentence saying what is being asked, then what the proposal says
 * in named fields, then the quoted evidence behind it, and only then — behind a
 * disclosure — the identifiers and versions.
 *
 * The summary leads and used to be absent entirely. It is the one field the
 * server documents as prose ("the sentence a human reads before deciding"), and
 * a dialog that opened on `deal_id` while omitting it was showing the reader
 * everything except the question.
 *
 * The technical block is kept rather than dropped. Somebody debugging a stuck
 * proposal needs the target version and the proposing actor, and a support
 * conversation needs the id — but none of that is why a person is being asked
 * to agree to something, so it does not open the dialog.
 */
function ApprovalDetailBody({
  approval,
  locale,
  zone,
}: Readonly<{
  approval: Approval;
  locale: ReturnType<typeof useLocale>["locale"];
  zone: string;
}>) {
  const t = useT();
  const change = (approval.proposed_change ?? {}) as Record<string, unknown>;
  const display = resolveDisplay(
    approval.kind,
    change,
    t,
    stagedDayFormatter(locale, zone),
  );
  const named = display.filter((entry) => entry.value !== null);
  // The raw payload is shown ONCE, and where it goes says what it is. A kind
  // that named its fields has already told the reader what the proposal says,
  // so the keys behind that are reference material; a kind that named none has
  // nothing else to show, so they are the content.
  const rawPayload = <RawPayload change={change} />;
  return (
    <div className="approval-detail">
      {approval.summary && (
        <p className="approval-detail-lead">{approval.summary}</p>
      )}
      {named.length > 0
        ? named.map((entry) => (
            <FieldLine
              key={entry.field}
              name={entry.label}
              value={entry.value ?? ""}
            />
          ))
        : rawPayload}
      <EvidenceList evidence={approval.evidence} />
      <Disclosure summary={t("inbox.detailTechnical")}>
        <div className="approval-detail">
          {named.length > 0 && rawPayload}
          {detailMeta(approval, locale, zone).map(([key, value]) => (
            <FieldLine key={key} name={key} mono value={value} />
          ))}
        </div>
      </Disclosure>
    </div>
  );
}

// The payload as the wire spells it. A nested document prints as its JSON
// rather than as "[object Object]".
function RawPayload({
  change,
}: Readonly<{ change: Readonly<Record<string, unknown>> }>) {
  return (
    <>
      {Object.entries(change).map(([key, value]) => (
        <FieldLine
          key={key}
          name={key}
          mono
          value={typeof value === "string" ? value : JSON.stringify(value)}
        />
      ))}
    </>
  );
}

// Wire field identifiers (contract shape), not translatable prose — rendered
// raw, behind the technical disclosure where a payload path is what the reader
// actually came for.
function detailMeta(
  approval: Approval,
  locale: ReturnType<typeof useLocale>["locale"],
  zone: string,
): [string, string][] {
  const meta: [string, string][] = [
    ["target_version", String(approval.target_version ?? "—")],
    ["proposed_by", approval.proposed_by],
  ];
  if (approval.on_behalf_of) {
    meta.push(["on_behalf_of", approval.on_behalf_of]);
  }
  meta.push(["created_at", formatDateTime(approval.created_at, locale, zone)]);
  if (approval.decided_at) {
    meta.push([
      "decided_at",
      formatDateTime(approval.decided_at, locale, zone),
    ]);
  }
  return meta;
}

// `mono` marks a value the WIRE spells — a uuid, a version, a payload path.
// A named field carries a person's own words and a date they recognise, and
// setting those in mono would dress a business fact as machine output.
function FieldLine({
  name,
  value,
  mono,
}: Readonly<{ name: string; value: string; mono?: boolean }>) {
  return (
    <div className="field">
      <span className="t-label">{name}</span>
      <p className={mono ? "t-mono" : "approval-detail-value"}>{value}</p>
    </div>
  );
}

/**
 * The row-local decide outcomes that KEEP the row mounted: a generic error and
 * the version-skew re-stage state. The already-decided note is deliberately NOT
 * here — it fires a pending invalidation that unmounts the row, so it is
 * surfaced at screen level by `useDecisionSink`, where it survives the refetch.
 */
export function DecideOutcome({
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
          style={{ color: "var(--danger)", marginTop: "var(--space-2)" }}
        >
          {problemMessageOf(decide.error, t)}
        </p>
      )}
      {skew && (
        <div style={{ marginTop: "var(--space-2)" }}>
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

/**
 * The inline staged-draft editor: the string fields of the proposed_change, in
 * the shape the kind declared for them, going up as `edited_payload`.
 *
 * It is handed to the card as its `editor` slot rather than living inside the
 * primitive, and that boundary is the point: an edit re-enters the admission
 * gate from scratch on the server (re-tiered, re-RBAC'd, new diff_hash —
 * ADR-0036), so what it may offer is a question about THIS kind and this
 * contract, which is screen knowledge. The card knows how to draw a decision;
 * it does not know what this installation lets a person change.
 */
export function StagedEditor({
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
