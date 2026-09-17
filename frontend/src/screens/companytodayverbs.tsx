// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The leading card's own fallback verb column, split out of companytoday.tsx
// (the file-length ratchet) rather than folded into it: `MomentRow`'s own doc
// says why a verb belongs here whenever the server's own recommended action
// names no destination, and the split keeps that reasoning and the column it
// draws in one place a reader can hold at once.

import { CheckSquare, FileText, Send } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { stable } from "../format/collate";
import type { useT } from "../i18n";
import "./record360/record360.css";

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
  icon: ReactNode;
  primary?: boolean;
  onAct: () => void;
}>) {
  return (
    <span className="today-verb">
      <Button variant={primary ? "ai" : "ghost"} small onClick={onAct}>
        {icon}
        {label}
      </Button>
    </span>
  );
}

// The leading card's own fallback verb, drawn only where the moment named no
// destination of its own (`MomentRow` prefers the server's verb whenever it
// has one). Read off the account's own state rather than the moment: a task
// already on the list, whether we owe the reply or they owe us, are facts
// this page holds regardless of which rule fired, and every one of them backs
// a control the page already owns — a task modal, the composer, the log
// drawer — so the fallback never offers a press that lands nowhere.
//
// Returns a fragment of `.today-verb` items rather than a column of its own:
// `MomentRow` owns the one `.today-actions` column its row draws, the same
// way `FoundMove` owns its.
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
