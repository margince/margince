import {
  AlertTriangle,
  ChevronRight,
  Circle,
  FileQuestion,
} from "lucide-react";
import type { ChangeEvent } from "react";
import { useEffect, useRef, useState } from "react";
import { Badge, Field } from "../../../design-system/atoms";
import {
  ConfidenceMeter,
  EvidenceChip,
  ProvenanceTag,
} from "../../../design-system/trust";
import { formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import type { CompanyFieldName } from "../onboarding";
import {
  type ReviewRow,
  type RowState,
  STATE_WORD,
} from "./company-review-state";
import { FINDING_EXPAND_EVENT } from "./entries";

function isBand(state: RowState): state is "high" | "med" | "low" {
  return state === "high" || state === "med" || state === "low";
}

// isWork's set split in two. The banded half (low, med) reads as a worded
// mark through ConfidenceMeter; this is the rest, the states with no value to
// grade. Required outranks empty in the order the nav's blocking cluster
// uses, so a row's mark and the nav agree on which gap is urgent.
function isOutstandingUnbanded(state: RowState): state is "required" | "empty" {
  return state === "required" || state === "empty";
}

/** The settled half of the same split: `typed`, `stored` and `quoted` all
 * have a value and no confidence to grade, so `isBand` skips them too. */
function isSettledUnbanded(
  state: RowState,
): state is "typed" | "stored" | "quoted" {
  return state === "typed" || state === "stored" || state === "quoted";
}

// The mark of a row with no gradable value. Its icon is shaped, not
// coloured, to tell the states apart (a solid warning glyph when blocking, a
// hollow ring when empty). It also carries the words a screen reader gets.
function OutstandingMark({
  state,
  t,
}: Readonly<{
  state: "required" | "empty";
  t: ReturnType<typeof useT>;
}>) {
  return (
    <span className="ob-triage-row-outstanding" data-state={state}>
      {state === "required" ? (
        <AlertTriangle aria-hidden />
      ) : (
        <Circle aria-hidden />
      )}
      <span className="sr-only">{t(STATE_WORD[state])}</span>
    </span>
  );
}

// isWork's complement, the settled half. ProvenanceTag's vocabulary words
// `typed` ("typed by you"), but has no entry for `stored` or `quoted`. Typed,
// untouched profile value and legal-notice quote are three different truths.
// "Typed by you" over the last two would be wrong, so each gets its own label.
// The default tone, not `ai`: none is a machine's claim.
const PROVENANCE_WORD: Readonly<Record<"stored" | "quoted", MessageKey>> = {
  stored: "ob.conv.triage.stateStoredBadge",
  quoted: "ob.conv.triage.stateQuotedBadge",
};

function ProvenanceMark({
  state,
  t,
}: Readonly<{
  state: "typed" | "stored" | "quoted";
  t: ReturnType<typeof useT>;
}>) {
  if (state === "typed") {
    return <ProvenanceTag provenance={{ kind: "human", self: true }} />;
  }
  return <Badge>{t(PROVENANCE_WORD[state])}</Badge>;
}

// A collapsed value shows one short line, cut on a word boundary. The rest
// stays out of the DOM until the row expands, the rule the collapsed
// EvidenceChip keeps for its own snippet.
const PROSE_PREVIEW_CHARS = 140;

function prosePreview(value: string): string {
  if (value.length <= PROSE_PREVIEW_CHARS) {
    return value;
  }
  const cut = value.lastIndexOf(" ", PROSE_PREVIEW_CHARS);
  return `${value.slice(0, cut > 0 ? cut : PROSE_PREVIEW_CHARS).trimEnd()}…`;
}

/**
 * The omission notice for a field the read looked for and came back without.
 * The backend drops a field that fails the evidence assertion (omission is the
 * safe failure), so the wire carries silence. This surface names the omission.
 *
 * The notice shows only where the read accounts for the field itself
 * (`row.omissionReasonKey`, the legal trio's gap). Elsewhere "not found on
 * your site" would be a fabrication, so those rows read as empty and fillable.
 * The read's `warnings` are crawl-wide and tied to no field; the CoverageCard
 * shows them.
 */
function OmissionNotice({
  label,
  reasonKey,
  t,
}: Readonly<{
  label: string;
  reasonKey: MessageKey;
  t: ReturnType<typeof useT>;
}>) {
  return (
    <div className="ob-triage-omitted">
      <FileQuestion aria-hidden />
      <div className="ob-triage-omitted-body">
        <p className="ob-triage-omitted-label">
          {t("ob.conv.triage.omittedLabel")}
        </p>
        <p>
          {t("ob.conv.triage.omittedField", {
            field: label,
            reason: t(reasonKey),
          })}
        </p>
      </div>
    </div>
  );
}

/**
 * One reviewable field, in one of two weights, so 16 fields stay readable:
 * - Collapsed: a 1-line summary that is itself the open button, so the
 *   evidence chip's button never nests in another.
 * - Expanded: the value editable in place, the state in words, the verbatim
 *   evidence one toggle away.
 * The work states (required, weak, empty) open by default; they are the work.
 */
export function FieldRow({
  row,
  setField,
  defaultExpanded,
}: Readonly<{
  row: ReviewRow;
  setField: (field: CompanyFieldName, value: string) => void;
  defaultExpanded: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [expanded, setExpanded] = useState(defaultExpanded);
  const onChange = (
    event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => setField(row.field, event.target.value);

  // Opts into jumpToFindings' "open it, then focus the real control" contract
  // (entries.tsx). A collapsed row has nothing typeable to land on, so a jump
  // here means "open this". An expanded row has nothing more to do.
  const li = useRef<HTMLLIElement>(null);
  useEffect(() => {
    const node = li.current;
    if (node === null) {
      return;
    }
    const onExpandRequest = () => setExpanded(true);
    node.addEventListener(FINDING_EXPAND_EVENT, onExpandRequest);
    return () =>
      node.removeEventListener(FINDING_EXPAND_EVENT, onExpandRequest);
  }, []);

  if (!expanded) {
    return (
      <li
        ref={li}
        id={rowDomId(row.field)}
        tabIndex={-1}
        data-finding-id={row.field}
        data-state={row.state}
        className="ob-triage-row"
      >
        <button
          type="button"
          className="ob-triage-row-summary"
          aria-expanded={false}
          onClick={() => setExpanded(true)}
        >
          <ChevronRight aria-hidden className="ob-triage-row-caret" />
          <span className="t-label">{row.label}</span>
          <span className="ob-conv-field-value">
            {row.value.trim() === ""
              ? t(row.emptyHintKey)
              : prosePreview(row.value)}
          </span>
          <span className="ob-triage-row-meta">
            {row.confidence !== null && (
              <span className="ob-triage-score t-caption">
                {formatNumber(Math.round(row.confidence * 100), locale)}
                {/* How many sources back the score: at most one, since a
                    proposal field carries a single evidence pair, but said as
                    a count rather than assumed. */}
                {row.evidence !== null && (
                  <span className="t-caption">
                    {/* A row carries at most one evidence record, so the
                        count is the literal one: a single digit no locale
                        groups or punctuates differently. */}
                    {t("ob.conv.triage.sourceCount", { count: "1" })}
                  </span>
                )}
              </span>
            )}
            {isBand(row.state) && <ConfidenceMeter level={row.state} />}
            {/* The band states carry a worded mark (ConfidenceMeter). The
                rest of isWork's set gets an icon that survives greyscale
                (shape, not colour) plus the words a screen reader gets. */}
            {isOutstandingUnbanded(row.state) && (
              <OutstandingMark state={row.state} t={t} />
            )}
            {isSettledUnbanded(row.state) && (
              <ProvenanceMark state={row.state} t={t} />
            )}
          </span>
        </button>
      </li>
    );
  }

  return (
    <li
      ref={li}
      id={rowDomId(row.field)}
      tabIndex={-1}
      data-finding-id={row.field}
      data-state={row.state}
      className="ob-triage-row ob-triage-row-open ob-conv-confirm-missing-row"
    >
      <Field
        label={
          <>
            {row.label}
            <em className="t-caption">{t(STATE_WORD[row.state])}</em>
          </>
        }
        labelEnd={
          <button
            type="button"
            className="ob-conv-field-expand"
            aria-expanded
            onClick={() => setExpanded(false)}
          >
            {t("ob.conv.review.showLess")}
          </button>
        }
      >
        {(control) => (
          <>
            {/* Above the control, not below it: the reader learns why the box
                is empty before deciding what to type in it. */}
            {row.omissionReasonKey !== null && (
              <OmissionNotice
                label={row.label}
                reasonKey={row.omissionReasonKey}
                t={t}
              />
            )}
            {row.multiline ? (
              <textarea {...control} value={row.value} onChange={onChange} />
            ) : (
              <input {...control} value={row.value} onChange={onChange} />
            )}
          </>
        )}
      </Field>
      {/* Only the evidence pair below the control: the label's state word
          says "typed by you", so the human tag would repeat it. The quote and
          meter are separate claims. A value picked off the legal notice has
          the page's words but no score, so it shows no meter. */}
      {row.evidence !== null && (
        <span className="ob-triage-row-proof">
          {isBand(row.state) && <ConfidenceMeter level={row.state} />}
          <EvidenceChip evidence={row.evidence} collapsed />
        </span>
      )}
    </li>
  );
}

function rowDomId(field: CompanyFieldName): string {
  return `ob-triage-row-${field}`;
}

export function groupDomId(key: string): string {
  return `ob-triage-group-${key}`;
}

export function jumpToRow(field: CompanyFieldName): void {
  const node = document.getElementById(rowDomId(field));
  // jsdom has no scrollIntoView; in the browser it always exists.
  node?.scrollIntoView?.({ block: "center", behavior: "smooth" });
  node?.focus({ preventScroll: true });
}

// The layout frozen at mount, once per group: row order and which rows open
// by default. `workCount` is frozen only to place the settled-cluster divider,
// which must not jump as a row is filled. What must stay live as the human
// types (nav count and list, a row's mark) is derived each render; see
// `outstandingSplit` in confirm-card.nav.tsx.
export type FrozenGroup = {
  key: string;
  labelKey: MessageKey;
  order: readonly CompanyFieldName[];
  workCount: number;
  openByDefault: ReadonlySet<CompanyFieldName>;
};

// The contacts section's key. The frozen layout, the section nav and the group
// renderer all agree this slot holds the read's contacts, not a profile field.
export const CONTACTS_KEY = "contacts";

// The facts section's key, alongside CONTACTS_KEY: another slot the frozen
// layout, the section nav and the group renderer all agree carries no
// profile field.
export const FACTS_KEY = "facts";
