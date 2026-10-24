// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The leading card's verb column, split out of companytoday.tsx (the
// file-length ratchet) rather than folded into it: which verb the card closes
// with is one question — the server's own wherever it named somewhere to go,
// the account's own where it did not — and the split keeps that reasoning and
// the column it draws in one place a reader can hold at once.

import { CheckSquare, FileText, Send } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useClaimSettle } from "../../components/taskactions";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { stable } from "../../format/collate";
import { useT } from "../../i18n";
import {
  basisAddsARecord,
  citationOpensRecord,
  MomentEvidence,
} from "../records/record360";
import type { DeferOf } from "./companysetaside";
import "../records/record360/record360.css";

type Company360 = components["schemas"]["Company360"];
type Company360Contact = NonNullable<Company360["contacts"]>["data"][number];

// The strongest-first ranking every generic "write to them" verb on this page
// uses — the account's own manual moves row (companytoday.tsx) and this
// column's own reply verbs — so the two cannot name two different contacts
// for the same question.
export function byStrengthThenId(
  a: Company360Contact,
  b: Company360Contact,
): number {
  const delta = (b.strength?.score ?? 0) - (a.strength?.score ?? 0);
  return delta !== 0 ? delta : stable(a.contact_id, b.contact_id);
}

// One verb in the leading card's fallback column: the label, the icon for its
// kind, and the button underneath — the shape the contact page's moment
// column already draws (contacttoday.tsx's `ActionVerb`), so a rep reading
// both pages meets one verb column rather than two spellings of it.
function MomentVerb({
  label,
  icon,
  primary,
  onAct,
}: Readonly<{
  label: string;
  // The glyph naming the verb's kind. Absent on the server's own verb, whose
  // label is the server's sentence rather than one of the kinds this page
  // draws — a guessed glyph there would name a kind nobody stated.
  icon?: ReactNode;
  primary?: boolean;
  onAct: () => void;
}>) {
  return (
    <span className="today-verb">
      <Button variant={primary ? "primary" : "ghost"} onClick={onAct}>
        {icon}
        {label}
      </Button>
    </span>
  );
}

// The leading card's verb: the moment's own wherever the server named a
// surface for it to open, and the account's own fallback where it did not. A
// page that invented a destination would offer a press that lands nowhere,
// and a card naming what is owed with nothing to press is an answer with no
// working verb — this is the one place that choice is made.
export function momentVerb({
  moment,
  onOpenRecord,
  fallback,
}: Readonly<{
  moment: NonNullable<Company360["moment"]>;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  fallback: ReactNode;
}>): ReactNode {
  const { destination, label, state } = moment.recommended_action;
  if (
    state !== "available" ||
    !onOpenRecord ||
    destination?.entity_type == null ||
    destination.entity_id == null ||
    // The press goes through the page's citation router, which opens only these kinds.
    !citationOpensRecord(destination.entity_type)
  ) {
    return fallback;
  }
  const { entity_type: type, entity_id: id } = destination;
  // The accent, not indigo: the moment is picked from a fixed rule ladder and
  // never written by a model. Indigo would claim that an agent wrote it.
  return (
    <MomentVerb primary label={label} onAct={() => onOpenRecord(type, id)} />
  );
}

// The leading card's own fallback verb, drawn only where the moment named no
// destination of its own (`momentVerb` above prefers the server's verb
// whenever it has one). Read off the account's own state rather than the
// moment: a task already on the list, whether we owe the reply or they owe
// us, are facts
// this page holds regardless of which rule fired, and every one of them backs
// a control the page already owns — a task modal, the composer, the log
// drawer — so the fallback never offers a press that lands nowhere.
//
// Returns a fragment of `.today-verb` items rather than a column of its own:
// `FoundMove` owns the one `.today-actions` column its row draws.
export function momentFallbackVerb({
  view,
  t,
  onOpenTask,
  onDraftTo,
  onLogActivity,
}: Readonly<{
  view: Company360;
  t: ReturnType<typeof useT>;
  onOpenTask?: (activityId: string) => void;
  onDraftTo?: (contactId: string) => void;
  onLogActivity?: () => void;
}>): ReactNode {
  const secondary = onLogActivity ? (
    <MomentVerb
      label={t("log.title")}
      icon={<FileText size={15} aria-hidden="true" />}
      onAct={onLogActivity}
    />
  ) : undefined;
  // A task already on the record's list outranks a reply, whatever is owed:
  // it is the more specific of the two, named by an activity rather than
  // inferred from the engagement state.
  const task = view.next_steps?.data[0];
  if (task) {
    return onOpenTask ? (
      <>
        <MomentVerb
          primary
          label={t("today.moment.act.openTask")}
          icon={<CheckSquare size={15} aria-hidden="true" />}
          onAct={() => onOpenTask(task.activity_id)}
        />
        {secondary}
      </>
    ) : undefined;
  }
  const recipient = [...(view.contacts?.data ?? [])].sort(byStrengthThenId)[0];
  if (!recipient || !onDraftTo) {
    return undefined;
  }
  const state = view.state_strip?.engagement?.state;
  const draftTo = () => onDraftTo(recipient.contact_id);
  const sendVerb = (label: string) => (
    <MomentVerb
      primary
      label={label}
      icon={<Send size={15} aria-hidden="true" />}
      onAct={draftTo}
    />
  );
  if (state === "waiting_on_us") {
    return (
      <>
        {sendVerb(t("today.draft.act"))}
        {secondary}
      </>
    );
  }
  if (state === "waiting_on_them") {
    return (
      <>
        {sendVerb(t("today.moment.act.followUp"))}
        {secondary}
      </>
    );
  }
  return (
    <>
      {sendVerb(t("today.moment.act.writeToThem"))}
      {secondary}
    </>
  );
}

// What the moment rests on, drawn only where that is a record its own
// headline does not already name (`basisAddsARecord` says why).
export function momentBasis(moment: Company360["moment"]): ReactNode {
  return moment && basisAddsARecord(moment) ? (
    <MomentEvidence evidence={moment.evidence} />
  ) : undefined;
}

// The quiet rung is an answer rather than an item, so there is nothing to put
// off. A promise card is set aside by its fingerprint, in the store the advice
// rows under it use.
export function momentDefer(
  moment: NonNullable<Company360["moment"]>,
  deferOf: DeferOf,
) {
  return moment.rule === "nothing_needed"
    ? undefined
    : deferOf(moment.evidence_fingerprint);
}

// Done on a "Done?" card settles the promise the card names. The server only
// asks this over a claim on the account page; it never settles one itself.
export function MayBeDoneVerb({
  moment,
  companyId,
}: Readonly<{
  moment: NonNullable<Company360["moment"]>;
  companyId: string;
}>) {
  const t = useT();
  const settle = useClaimSettle([["company360", companyId]]);
  const question = moment.may_be_done;
  const done = moment.recommended_action;
  if (question?.promise_type !== "claim") {
    return null;
  }
  return (
    <span className="today-verb">
      <Button
        variant="primary"
        disabled={settle.isPending}
        reason={
          done.state === "blocked"
            ? (done.blocked_reason ?? t("contact.rail.blocked"))
            : undefined
        }
        onClick={() =>
          settle.mutate({ id: question.promise_id, outcome: "done" })
        }
      >
        <CheckSquare aria-hidden="true" />
        {done.label}
      </Button>
      {settle.error && <ErrorLine error={settle.error} inline />}
    </span>
  );
}
