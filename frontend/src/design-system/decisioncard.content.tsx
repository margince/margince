// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import type { DecisionApproval, DecisionCardLabels } from "./decisioncard";
import type {
  DecisionDiff,
  DecisionDraft,
  PayloadField,
} from "./decisioncard.payload";
import { type Fact, FactList } from "./factlist";
import { EvidenceChip, FieldDiff } from "./trust";

// What a DecisionCard DRAWS of the proposal it carries: the drafted message,
// the field changes, the remaining facts, and the receipts under them.
//
// Split from the card for the same reason the payload reading is: the card's
// anatomy is a header band, a body, a fold and a row of verbs, and every one of
// those was being read past to reach the three components that fill the body.
// They are internals of the card rather than primitives of their own — the
// catalog names them inside its DecisionCard row, and nothing outside this pair
// of files draws a staged proposal.
// Past this many characters the drafted body is clamped. A card that grows with
// its content stops being a card: one long email pushes every verb below the
// fold, and in the deck it pushes the card behind it off the plate entirely.
const BODY_CLAMP_CHARS = 320;

// The clamped body and its expander.
//
// A `.link-button` rather than `Disclosure`, and the difference is the whole
// point: a disclosure HIDES its content until asked, and a draft nobody can see
// any of is a question with the answer removed. The reader gets the opening
// lines unasked, and the control only lifts the clamp — which is why it is an
// `aria-expanded` toggle over one paragraph rather than a second copy of the
// text behind a summary.
export function DraftBody({
  body,
  labels,
}: Readonly<{ body: string; labels: DecisionCardLabels }>) {
  const [open, setOpen] = useState(false);
  const bodyId = useId();
  // Expandable only where the caller named both halves of the toggle: a control
  // whose label the surface has no words for is a control nobody can act on.
  const expandable =
    body.length > BODY_CLAMP_CHARS &&
    labels.showMore !== undefined &&
    labels.showLess !== undefined;
  const clampable = body.length > BODY_CLAMP_CHARS;
  return (
    <>
      <p
        id={bodyId}
        className="dcard-draft-body"
        data-clamped={clampable && !open ? "" : undefined}
      >
        {body}
      </p>
      {expandable && (
        <button
          type="button"
          className="link-button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen((shown) => !shown)}
        >
          {open ? labels.showLess : labels.showMore}
        </button>
      )}
    </>
  );
}

// What the proposal actually says, in the order a reader needs it: why they
// are being asked, then the words they are being asked to put their name on,
// then the values that would move, then the rest.
//
// The reason comes FIRST and unlabelled. It is a sentence the server wrote for
// a contact — the close-date sweep calls its own field "the plain-language
// derivation" — so captioning it would frame an explanation as a data point,
// and burying it under the values it explains asks the reader to work out the
// question from the answer.
export function DecisionContent({
  draft,
  diffs,
  lead,
  rest,
  raw,
  labels,
}: Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  lead: string | null;
  rest: readonly PayloadField[];
  /** The facts are wire keys, not declared fields — see restFields. */
  raw: boolean;
  labels: DecisionCardLabels;
}>) {
  // The ELEMENT each field is drawn in is decided here and nowhere else: the
  // payload module reads the map and returns two strings per row, so a wire key
  // reading as an identifier is one rule rather than one per call site.
  const facts: readonly Fact[] = rest.map((field) => ({
    key: field.key,
    term: raw ? (
      <span className="t-mono">{field.label}</span>
    ) : (
      <span>{field.label}</span>
    ),
    value: <span className="dcard-fact">{field.value}</span>,
  }));
  return (
    <>
      {lead && <p className="dcard-lead">{lead}</p>}
      {draft.body && (
        <div className="dcard-draft">
          <span className="t-eyebrow dcard-draft-label">
            {labels.draftBody}
          </span>
          <DraftBody body={draft.body} labels={labels} />
        </div>
      )}
      {diffs.map((diff) => (
        <div className="dcard-diff" key={diff.field}>
          <span className="t-eyebrow dcard-draft-label">
            {diff.field.replaceAll("_", " ")}
          </span>
          <FieldDiff oldValue={diff.from} newValue={diff.to} />
        </div>
      ))}
      {facts.length > 0 && (
        <FactList
          facts={facts}
          className={raw ? "dcard-rest dcard-rest-raw" : "dcard-rest"}
        />
      )}
    </>
  );
}

// The receipts, always on the card and never behind a popover: this is the one
// surface where a contact has to be able to check a claim BEFORE agreeing to it.
// Collapsed in the row layout, where a queue of verbatim snippets would bury
// the verbs; open in the deck, where there is one card and room to read it.
export function DecisionEvidence({
  evidence,
  collapsed,
}: Readonly<{
  evidence: DecisionApproval["evidence"];
  collapsed: boolean;
}>) {
  // Two rows of one source can open with the same twelve characters — a quoted
  // thread quotes itself — and a duplicate key hands one chip's expansion state
  // to another on the next render. So the key carries an occurrence count of the
  // otherwise-identical string: derived from the data rather than from the
  // position, which is what makes it survive a list that arrives in a different
  // order.
  const seen = new Map<string, number>();
  const keyOf = (item: { source_id?: string | null; snippet: string }) => {
    // The FULL snippet, not a prefix: two quotes from one source that open the
    // same way are different evidence, and a key that could not tell them apart
    // handed one chip the other's expansion state whenever the list reordered.
    const base = JSON.stringify([item.source_id ?? "", item.snippet]);
    const before = seen.get(base) ?? 0;
    seen.set(base, before + 1);
    return before === 0 ? base : `${base}#${before}`;
  };
  return (
    <>
      {evidence?.map((item) =>
        item.evidence_snippet ? (
          <EvidenceChip
            key={keyOf({
              source_id: item.source_id,
              snippet: item.evidence_snippet,
            })}
            collapsed={collapsed}
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
