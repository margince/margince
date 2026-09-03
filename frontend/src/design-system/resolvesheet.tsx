// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Modal, TextInput } from "./atoms";
import { ChoiceList } from "./choicelist";
import { DateInput, type ISODate, isISODate } from "./dateinput";

// Answering a finding from the nightly input check.
//
// The six outcomes are not interchangeable, and which fields a person must
// fill depends on which one they picked. Those rules MIRROR the server's
// refusals rather than inventing their own: a sheet that let somebody submit
// what the server will reject spends their attention on a round trip that was
// always going to fail, and one that demanded more than the server does
// invents a rule nobody agreed to.
//
// The sheet holds no copy. Every word arrives through props, because three
// surfaces open it — Analytics, Home and the Worklist — and a sheet that spelled
// its own labels would give the same answer three names.
export function ResolveSheet({
  open,
  labels,
  onSubmit,
  onClose,
  pending,
}: Readonly<{
  open: boolean;
  labels: ResolveSheetLabels;
  onSubmit: (answer: ResolveAnswer) => void;
  onClose: () => void;
  pending?: boolean;
}>) {
  const [outcome, setOutcome] = useState<ResolveOutcome | "">("");
  const [reason, setReason] = useState("");
  // ISODate rather than a bare string: the field takes a calendar day, and a
  // type that admitted any text would let a caller pass one the input silently
  // ignores.
  const [remindAt, setRemindAt] = useState<ISODate | "">("");
  const [expiresAt, setExpiresAt] = useState<ISODate | "">("");
  const titleID = useId();
  // Explicit id/htmlFor pairs rather than relying on the label wrapping the
  // control. The controls here are COMPONENTS, and neither a linter nor a
  // reader of this file can see through one to check the association holds.
  const reasonID = useId();
  const remindID = useId();
  const expiresID = useId();

  const needsReason = suppresses(outcome);
  const needsRemind = outcome === "remind_later";
  // Nothing chosen is not an answer. Submitting with no outcome would post a
  // body the server refuses, and the reader would read the refusal as a bug.
  const complete =
    outcome !== "" &&
    (!needsReason || reason.trim() !== "") &&
    (!needsRemind || remindAt !== "");

  return (
    <Modal open={open} labelledBy={titleID} onClose={onClose} placement="right">
      <h2 id={titleID}>{labels.title}</h2>
      <ChoiceList
        legend={labels.outcomeLegend}
        value={outcome}
        choices={labels.outcomes}
        onChange={setOutcome}
      />

      {/* An answer that HIDES a finding says why. The next person to meet the
          number is owed the reason it is not flagged, and the two suppressing
          outcomes are the only ones that take it away from them. */}
      {needsReason && (
        <>
          <label className="field" htmlFor={reasonID}>
            <span>{labels.reason}</span>
            <TextInput
              id={reasonID}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
          {/* Outside the label on purpose: inside, it joins the field's
              accessible name, and a screen reader would read the whole
              sentence where a sighted reader sees one word. */}
          <p className="sub">{labels.reasonHelp}</p>
        </>
      )}

      {/* A deferral names when it comes back, or it is a dismissal wearing a
          different word. */}
      {needsRemind && (
        <label className="field" htmlFor={remindID}>
          <span>{labels.remindAt}</span>
          <DateInput
            id={remindID}
            value={remindAt}
            onChange={(event) => setRemindAt(asDate(event.target.value))}
          />
        </label>
      )}

      {/* Optional, and bounded. Left empty the server applies its own ceiling;
          past the ceiling it refuses rather than shortening, so the help text
          says the limit rather than letting somebody discover it. */}
      {needsReason && (
        <>
          <label className="field" htmlFor={expiresID}>
            <span>{labels.expiresAt}</span>
            <DateInput
              id={expiresID}
              value={expiresAt}
              onChange={(event) => setExpiresAt(asDate(event.target.value))}
            />
          </label>
          <p className="sub">{labels.expiresHelp}</p>
        </>
      )}

      <div className="card-actions">
        <Button small onClick={onClose}>
          {labels.cancel}
        </Button>
        <Button
          small
          variant="primary"
          disabled={!complete || pending}
          onClick={() => {
            if (outcome === "") {
              return;
            }
            // Only the fields this outcome takes. Sending an expiry with a
            // deferral would record a suppression nobody chose.
            onSubmit({
              outcome,
              reason: needsReason ? reason.trim() : undefined,
              remindAt: needsRemind ? remindAt : undefined,
              expiresAt:
                needsReason && expiresAt !== "" ? expiresAt : undefined,
            });
          }}
        >
          {labels.submit}
        </Button>
      </div>
    </Modal>
  );
}

// asDate narrows what the element reported.
//
// The input only ever reports YYYY-MM-DD or the empty string, and dateinput
// ships the predicate for exactly this — asserting the type instead would claim
// what a check can establish.
function asDate(raw: string): ISODate | "" {
  return isISODate(raw) ? raw : "";
}

// The two answers that take a finding off every screen. They are the reason
// this sheet has conditional fields at all.
function suppresses(outcome: ResolveOutcome | ""): boolean {
  return outcome === "value_correct" || outcome === "not_relevant";
}

// ResolveOutcome is the six answers a person can give.
//
// `condition_cleared` is deliberately absent: that is the check's own answer,
// and a person naming it would be saying the condition stopped being true
// without anything having looked.
export type ResolveOutcome =
  | "fixed_record"
  | "added_evidence"
  | "value_correct"
  | "not_relevant"
  | "remind_later"
  | "reassign";

// ResolveAnswer is what the sheet submits: the outcome, and only the fields
// that outcome takes.
export type ResolveAnswer = Readonly<{
  outcome: ResolveOutcome;
  reason?: string;
  remindAt?: string;
  expiresAt?: string;
}>;

// ResolveSheetLabels is every word the sheet draws.
export type ResolveSheetLabels = Readonly<{
  title: string;
  outcomeLegend: string;
  outcomes: readonly {
    value: ResolveOutcome;
    label: string;
    description?: string;
  }[];
  reason: string;
  reasonHelp: string;
  remindAt: string;
  expiresAt: string;
  expiresHelp: string;
  cancel: string;
  submit: string;
}>;
