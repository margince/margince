// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { useLocale } from "../i18n";
import { blankRequired } from "../lib/problem";
import { Button, Field, Modal, TextInput } from "./atoms";
import { ChoiceList } from "./choicelist";
import { DateInput, type ISODate } from "./dateinput";
import { DrawerBody, DrawerFoot, DrawerHead } from "./drawerbands";
import { ErrorLine } from "./errorline";
import { refuseUnreadableField } from "./formvalidity";
import { Heading } from "./heading";
import { useFieldFaults } from "./savepage";

// Answering a finding from the nightly input check.
//
// The six outcomes are not interchangeable, and which fields a contact must
// fill depends on which one they picked. Those rules MIRROR the server's
// refusals rather than inventing their own: a sheet that let somebody submit
// what the server will reject spends their attention on a round trip that was
// always going to fail, and one that demanded more than the server does
// invents a rule nobody agreed to.
//
// The sheet holds no copy. Every word arrives through props, because three
// surfaces open it — Analytics, Brief and the Worklist — and a sheet that spelled
// its own labels would give the same answer three names.
export function ResolveSheet({
  open,
  context,
  labels,
  onSubmit,
  onClose,
  pending,
  error,
  returnFocusTo,
  outcomeFields,
  canSubmit,
  asksExpiry,
}: Readonly<{
  open: boolean;
  // What is being answered, shown above the choices. The sheet covers the list
  // that opened it, so without this the reader answers blind.
  context?: ReactNode;
  outcomeFields?: (outcome: ResolveOutcome | "") => ReactNode;
  canSubmit?: (outcome: ResolveOutcome) => boolean;
  // Whether a hiding answer offers an end date. An answer that closes the
  // record for good has nothing to end.
  asksExpiry?: (outcome: ResolveOutcome) => boolean;
  labels: ResolveSheetLabels;
  onSubmit: (answer: ResolveAnswer) => void;
  onClose: () => void;
  pending?: boolean;
  /** The refused save, drawn above the actions so the sheet never fails silently. */
  error?: unknown;
  /** Where focus lands on close, when a save unmounts the control that opened it. */
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const [outcome, setOutcome] = useState<ResolveOutcome | "">("");
  const [reason, setReason] = useState("");
  // ISODate rather than a bare string: the field takes a calendar day, and a
  // type that admitted any text would let a caller pass one the input silently
  // ignores.
  const [remindAt, setRemindAt] = useState<ISODate | "">("");
  const [expiresAt, setExpiresAt] = useState<ISODate | "">("");
  const titleID = useId();

  const needsReason = suppresses(outcome);
  const needsExpiry =
    needsReason && outcome !== "" && (asksExpiry?.(outcome) ?? true);
  const needsRemind = outcome === "remind_later";
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  // Nothing chosen is not an answer. Submitting with no outcome would post a
  // body the server refuses, and the reader would read the refusal as a bug.
  const blank = () =>
    blankRequired(locale, {
      outcome: [labels.outcomeLegend, outcome],
      ...(needsReason ? { reason: [labels.reason, reason] as const } : {}),
      ...(needsRemind
        ? { remindAt: [labels.remindAt, remindAt] as const }
        : {}),
    });
  // A state the sheet cannot word, such as data its caller still loads.
  const held = outcome !== "" && !(canSubmit?.(outcome) ?? true);

  return (
    <Modal
      open={open}
      labelledBy={titleID}
      onClose={onClose}
      intent="drawer"
      returnFocusTo={returnFocusTo}
    >
      <DrawerHead>
        <Heading size="large" id={titleID} className="modal-title">
          {labels.title}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <div className="form-stack" ref={faultsRoot}>
          {context}
          <ChoiceList
            legend={labels.outcomeLegend}
            value={outcome}
            choices={labels.outcomes}
            onChange={(next) => {
              setOutcome(next);
              faults.edited("outcome");
            }}
          />
          <ErrorLine>{faults.of("outcome")}</ErrorLine>

          {outcomeFields?.(outcome)}

          {/* A hidden finding owes the next reader the reason it is not flagged. */}
          {needsReason && (
            <Field
              label={labels.reason}
              hint={labels.reasonHelp}
              required
              error={faults.of("reason")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  value={reason}
                  onChange={(event) => {
                    setReason(event.target.value);
                    faults.edited("reason");
                  }}
                />
              )}
            </Field>
          )}

          {/* A deferral without a date is a dismissal. */}
          {needsRemind && (
            <Field
              label={labels.remindAt}
              required
              error={faults.of("remindAt")}
            >
              {(control) => (
                <DateInput
                  {...control}
                  value={remindAt}
                  onChange={(event) => {
                    setRemindAt(event.target.value);
                    faults.edited("remindAt");
                  }}
                />
              )}
            </Field>
          )}

          {/* Past the server's ceiling the date is refused, not shortened, so the
              hint states the limit. */}
          {needsExpiry && (
            <Field label={labels.expiresAt} hint={labels.expiresHelp}>
              {(control) => (
                <DateInput
                  {...control}
                  value={expiresAt}
                  onChange={(event) => setExpiresAt(event.target.value)}
                />
              )}
            </Field>
          )}

          <ErrorLine error={error} />
        </div>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose}>{labels.cancel}</Button>
        <Button
          variant="primary"
          disabled={held || pending}
          onClick={(event) => {
            if (
              faults.show(blank()) ||
              outcome === "" ||
              refuseUnreadableField(event.currentTarget)
            ) {
              return;
            }
            // Only the fields this outcome takes. Sending an expiry with a
            // deferral would record a suppression nobody chose.
            onSubmit({
              outcome,
              reason: needsReason ? reason.trim() : undefined,
              remindAt: needsRemind ? remindAt : undefined,
              expiresAt:
                needsExpiry && expiresAt !== "" ? expiresAt : undefined,
            });
          }}
        >
          {labels.submit}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

// The two answers that take a finding off every screen. They are the reason
// this sheet has conditional fields at all.
function suppresses(outcome: ResolveOutcome | ""): boolean {
  return outcome === "value_correct" || outcome === "not_relevant";
}

// ResolveOutcome is the six answers a contact can give.
//
// `condition_cleared` is deliberately absent: that is the check's own answer,
// and a contact naming it would be saying the condition stopped being true
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
