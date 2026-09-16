// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE MOMENT, as every record page reads it: the words for the rule that
// fired, the colour it carries, what it rests on, whether it belongs in the
// day's work at all, and the row the account brief draws it as.
//
// It lives in the kit rather than on a page because the contact page and the
// account brief both read it, and the second one used to reach into the first
// for the vocabulary — the same borrowing that leaves two pages free to drift
// on what "Nothing needed" is coloured or which evidence line quotes what.
// Every type here is already the kit's own (`StandingTone`), which is the
// tell that this was kit material sitting on a screen.

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { MomentEvidence } from "./momentevidence";
import type { StandingTone } from "./verdict";
// The row's classes — `co-move`, `co-move-lead`, `co-dim` — live in the screen
// stylesheet, as every `co-`prefixed class in this kit still does (README.md
// says why the rename is its own job). Imported HERE and not left to whichever
// page mounts the row: relying on a caller's import is a row that renders
// unstyled the first time a page draws it without that stylesheet in its
// graph, which is a defect nothing fails on.
import "../company360.css";
// The verb column: `.today-actions` and `.today-verb`, the kit's own now that
// two record pages draw one shape for it.
import "./record360.css";

type ContactMoment = components["schemas"]["ContactMoment"];
export type ContactMomentEvidence =
  components["schemas"]["ContactMomentEvidence"];

// The rule that fired, in one word over the sentence it produced.
//
// The server picks exactly one rule from a fixed ladder, so the word is the
// server's judgement rather than a page's reading of a headline. Named
// rather than left implicit: "Gone quiet" and "Promise overdue" lead to
// different moves, and a reader who sees only the sentence has to infer which
// kind of thing they are looking at.
export const MOMENT_RULE_LABEL = {
  meeting_prep: "contact.moment.rule.meeting_prep",
  re_engaged: "contact.moment.rule.re_engaged",
  job_change: "contact.moment.rule.job_change",
  overdue_promise: "contact.moment.rule.overdue_promise",
  gone_quiet: "contact.moment.rule.gone_quiet",
  open_promise: "contact.moment.rule.open_promise",
  public_signal: "contact.moment.rule.public_signal",
  missing_next_step: "contact.moment.rule.missing_next_step",
  thin_relationship: "contact.moment.rule.thin_relationship",
  nothing_needed: "contact.moment.rule.nothing_needed",
} as const satisfies Record<ContactMoment["rule"], MessageKey>;

// The standing's colour. Two rules mean somebody is being kept waiting and
// read as warnings; the quiet success state reads as settled rather than as
// something nobody has judged. Everything else is a live thread — a fact about
// the relationship that wants a move rather than a verdict on it.
export function standingTone(rule: ContactMoment["rule"]): StandingTone {
  if (isLate(rule)) {
    return "warn";
  }
  return rule === "nothing_needed" ? "calm" : "accent";
}

// The two rules that mean somebody is being kept waiting. A promise past its
// date is late whether it was read out of an email or filed as a task — one
// rung covers both — while a promise not yet due is a live thread, not a
// warning.
export function isLate(rule: ContactMoment["rule"]): boolean {
  return rule === "gone_quiet" || rule === "overdue_promise";
}

/**
 * Whether the moment is a ROW in the day's work, given what else is in the
 * list beside it.
 *
 * Every moment is, except the quiet one — and the quiet one only when there is
 * nothing else. "Nothing needs you today" drawn above a row that asks for
 * something is the panel disagreeing with itself inside one glance, and it is
 * the panel the reader stops believing: the row below it is checkable and the
 * verdict above it is not. The account brief reached that state routinely,
 * because its card fires on owed promises alone while the suggestions under it
 * fire on unanswered mail, a stalled deal, an account with nothing scheduled.
 *
 * Nothing is lost by dropping it. The row goes only where the list has
 * something in it, and what is in the list IS the answer to what needs the
 * reader — the card was not carrying an answer there, it was carrying a wrong
 * one. Where the list is empty the card stands, reason and verb included, and
 * a panel handed no rows at all still has its own one sentence.
 */
export function momentIsARow(
  moment: ContactMoment,
  othersInTheList: boolean,
): boolean {
  return moment.rule !== "nothing_needed" || !othersInTheList;
}

/**
 * The moment as the lead row of the needs list: the rule it fired on as the
 * eyebrow, the headline in the display face, why now, what it rests on
 * under it, and a verb to answer it with.
 *
 * The server's own verb draws first, and only where it said the action can be
 * taken AND named somewhere to go. Where it did not, `action` is the page's
 * own fallback — a verb the page can back with a control it already owns
 * (a task modal, the composer) rather than a destination the server never
 * named. A card that names what is owed and offers nothing to do about it is
 * an answer with no working verb; the fallback is what keeps that promise
 * even when the server's own move has nowhere to land.
 */
export function MomentRow({
  moment,
  onOpenRecord,
  onOpenEvidence,
  evidenceKind,
  action,
}: Readonly<{
  moment: ContactMoment;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens one piece of evidence the moment rests on. Absent where the record
  // has nowhere to send the reader for it: an item with an id then renders as
  // a source rather than a control that goes nowhere.
  onOpenEvidence?: (item: ContactMomentEvidence) => void;
  // Resolves the glyph's kind for one evidence item: the contact page looks
  // an activity item up in its own activity list for the email glyph rather
  // than the generic "activity" one. Absent draws the item's own `type`.
  evidenceKind?: (item: ContactMomentEvidence) => string | undefined;
  // The page's own fallback verb, drawn only when the server named no
  // destination of its own — zero or more verbs, each already in the shared
  // `.today-verb` shape (record360.css). This row owns the `.today-actions`
  // column itself, the same way `FoundMove` owns its own: a caller handing in
  // a column of its own nested one inside the other.
  action?: ReactNode;
}>): ReactNode {
  const t = useT();
  const destination = moment.recommended_action.destination;
  const target =
    moment.recommended_action.state === "available" &&
    destination?.entity_type != null &&
    destination.entity_id != null
      ? { type: destination.entity_type, id: destination.entity_id }
      : undefined;
  const tone = standingTone(moment.rule);
  const verb =
    target && onOpenRecord ? (
      <span className="today-verb">
        {/* Indigo, because pressing it hands the work to Margince: the
            hue is the product's one claim about who is acting, and a
            verb the agent performs drawn in the accent would read as
            the reader's own move. */}
        <Button
          small
          variant="ai"
          onClick={() => onOpenRecord(target.type, target.id)}
        >
          {moment.recommended_action.label}
        </Button>
      </span>
    ) : (
      action
    );
  return (
    <PanelRow className="co-move co-move-lead">
      <span className="co-move-body">
        <span className="co-move-by">
          <span className={`co-dim co-dim-${tone} t-sub`}>
            {t(MOMENT_RULE_LABEL[moment.rule])}
          </span>
        </span>
        <span className="co-move-ask co-move-headline">{moment.headline}</span>
        <span className="co-move-reason t-sub">{moment.why_now}</span>
        {moment.evidence.length > 0 && (
          <div className="co-move-basis">
            <span className="co-move-basis-head t-caption">
              {t("co.suggest.basedOn")}
            </span>
            <MomentEvidence
              evidence={moment.evidence}
              kindOf={evidenceKind}
              onOpen={onOpenEvidence}
            />
          </div>
        )}
        {verb && (
          <span className="co-move-do">
            <div className="today-actions">{verb}</div>
          </span>
        )}
      </span>
    </PanelRow>
  );
}
