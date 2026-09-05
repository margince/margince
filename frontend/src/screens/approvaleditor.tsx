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
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import { Select } from "../design-system/select";
import { EvidenceChip } from "../design-system/trust";
import { isRealCalendarDay } from "../format/calendarday";
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
import type { Approval } from "./approvals.queries";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

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
 * AC-2: the row's "view everything" affordance — the proposal's own fields, the
 * evidence behind them and the timestamps the summary/evidence-chip row
 * necessarily elides.
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
        {t("decision.detail")}
      </h2>
      {open && (
        <QueryGate query={detail} pendingLabel={t("decision.detailLoading")}>
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
 * in named fields, then when it was asked, then the quoted evidence behind it.
 *
 * The summary leads and used to be absent entirely. It is the one field the
 * server documents as prose ("the sentence a human reads before deciding"), and
 * a dialog that opened on `deal_id` while omitting it was showing the reader
 * everything except the question.
 *
 * Only the raw payload stays behind the disclosure, and only for a kind that has
 * already said what its fields mean — there it is reference material for
 * somebody checking what the server actually staged. The target version and the
 * `agent:<id>` that proposed it are gone from this surface entirely: neither
 * names a person or describes a change, so a reader can do nothing with them,
 * and one click away rather than zero does not make them useful.
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
        <p className="approval-detail-lead t-body">{approval.summary}</p>
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
      {askedOn(approval, locale, zone, t).map(([label, value]) => (
        <FieldLine key={label} name={label} value={value} />
      ))}
      <EvidenceList evidence={approval.evidence} />
      {named.length > 0 && (
        <Disclosure summary={t("decision.detailTechnical")}>
          <div className="approval-detail">{rawPayload}</div>
        </Disclosure>
      )}
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

/**
 * When the question was raised, and when it was settled.
 *
 * These two used to sit behind the technical disclosure, labelled `created_at`
 * and `decided_at` in monospace beside a uuid. They are not technical: how long
 * a proposal has been waiting is one of the few things on this dialog a reader
 * actually weighs, and it was filed with the debugging material because it
 * arrived on the wire next to it.
 *
 * The rest of that block is gone from this surface rather than moved. A target
 * version and a `proposed_by` of `agent:<id>` name no person and describe no
 * change — there is nothing a reader can do with either, and putting them one
 * click away rather than zero does not make them useful.
 */
function askedOn(
  approval: Approval,
  locale: ReturnType<typeof useLocale>["locale"],
  zone: string,
  t: ReturnType<typeof useT>,
): [string, string][] {
  const asked: [string, string][] = [
    [
      t("decision.detailAsked"),
      formatDateTime(approval.created_at, locale, zone),
    ],
  ];
  if (approval.decided_at) {
    asked.push([
      t("decision.detailDecided"),
      formatDateTime(approval.decided_at, locale, zone),
    ]);
  }
  return asked;
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
      <p className={mono ? "t-mono" : "approval-detail-value t-body"}>
        {value}
      </p>
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
            {t("decision.versionSkew")}
          </p>
          <Button small onClick={onReRead}>
            {t("decision.reRead")}
          </Button>
        </div>
      )}
    </>
  );
}

/**
 * What the editor should START with for one field.
 *
 * This is the seed AND the display rule, deliberately in one function: a date
 * control silently discards a value it cannot parse, so if the two disagreed
 * the reader would see an empty box while the unparseable original still rode
 * out on approve — an editor showing one thing and submitting another.
 *
 * "Cannot parse" covers more than a wrong shape. `2026-02-30` is spelled
 * correctly and is not a day, and the element blanks it exactly as it blanks
 * `27/09/2026` — so the seed asks whether the date EXISTS, through
 * isRealCalendarDay, rather than only whether it looks like one.
 *
 * A non-string payload value seeds empty rather than being stringified. The
 * editor only ever offers string fields, so a number or object arriving here is
 * a payload that does not match its kind's declaration, and inviting somebody
 * to edit `[object Object]` is worse than an empty box.
 */
export function editableSeed(entry: EditableField, staged: unknown): string {
  if (typeof staged !== "string") {
    return "";
  }
  return entry.as === "date" && !isRealCalendarDay(staged) ? "" : staged;
}

// The seeded value narrowed to what the date control's own type accepts. The
// seed already dropped anything unshowable, so this is the type narrowing and
// not a second rule.
function isoOrBlank(staged: string | undefined): ISODate | "" {
  return staged && isISODate(staged) ? staged : "";
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
            ) : entry.as === "date" ? (
              <DateInput
                {...control}
                // A staged date the control cannot show is shown as empty
                // rather than passed through: the element would blank a
                // malformed value on its own, and an empty box a reader can
                // fill beats one that silently drops what they typed.
                value={isoOrBlank(draft[entry.field])}
                onChange={(event) => onChange(entry.field, event.target.value)}
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
          {t("decision.approveEdited")}
        </Button>
        <Button small disabled={pending} onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
      </div>
    </div>
  );
}
