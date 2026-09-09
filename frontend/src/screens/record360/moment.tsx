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
// Every type here is already the kit's own (`Grounding`, `StandingTone`),
// which is the tell that this was kit material sitting on a screen.

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { formatDate } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { type Grounding, Proof, type StandingTone } from "./verdict";
// The row's classes — `co-move`, `co-move-lead`, `co-dim` — live in the screen
// stylesheet, as every `co-`prefixed class in this kit still does (README.md
// says why the rename is its own job). Imported HERE and not left to whichever
// page mounts the row: relying on a caller's import is a row that renders
// unstyled the first time a page draws it without that stylesheet in its
// graph, which is a defect nothing fails on.
import "../company360.css";

type PersonMoment = components["schemas"]["PersonMoment"];
type PersonMomentEvidence = components["schemas"]["PersonMomentEvidence"];

// The rule that fired, in one word over the sentence it produced.
//
// The server picks exactly one rule from a fixed ladder, so the word is the
// server's judgement rather than a page's reading of a headline. Named
// rather than left implicit: "Gone quiet" and "Promise overdue" lead to
// different moves, and a reader who sees only the sentence has to infer which
// kind of thing they are looking at.
export const MOMENT_RULE_LABEL = {
  meeting_prep: "person.moment.rule.meeting_prep",
  re_engaged: "person.moment.rule.re_engaged",
  job_change: "person.moment.rule.job_change",
  overdue_promise: "person.moment.rule.overdue_promise",
  gone_quiet: "person.moment.rule.gone_quiet",
  open_promise: "person.moment.rule.open_promise",
  role_change: "person.moment.rule.role_change",
  public_signal: "person.moment.rule.public_signal",
  missing_next_step: "person.moment.rule.missing_next_step",
  thin_relationship: "person.moment.rule.thin_relationship",
  nothing_needed: "person.moment.rule.nothing_needed",
} as const satisfies Record<PersonMoment["rule"], MessageKey>;

export const MOMENT_EVIDENCE_LABEL = {
  activity: "person.moment.evidence.activity",
  task: "person.moment.evidence.task",
  relationship_change: "person.moment.evidence.relationship_change",
} as const satisfies Record<PersonMomentEvidence["type"], MessageKey>;

/**
 * What one piece of a moment's evidence says, in the shape every claim on a
 * record states it. The QUOTE is the verbatim excerpt when the server has
 * one — the words that were actually written — and the label otherwise; the
 * origin line names the kind of record it came from and when. A label put
 * where the quote goes, over a kind word, read "record" twice and quoted
 * nothing.
 */
export function momentGrounding(
  evidence: readonly PersonMomentEvidence[],
  t: ReturnType<typeof useT>,
  locale: Locale,
  recordZone: string,
): Grounding[] {
  return evidence.map((item) => {
    const observed = item.observed_at
      ? formatDate(item.observed_at, locale, recordZone)
      : undefined;
    return {
      key: `${item.type}-${item.id ?? item.label}`,
      quote: item.snippet ?? item.label,
      from: [
        item.snippet ? item.label : undefined,
        t(MOMENT_EVIDENCE_LABEL[item.type]),
        observed,
      ]
        .filter(Boolean)
        .join(" · "),
    };
  });
}

// The standing's colour. Two rules mean somebody is being kept waiting and
// read as warnings; the quiet success state reads as settled rather than as
// something nobody has judged. Everything else is a live thread — a fact about
// the relationship that wants a move rather than a verdict on it.
export function standingTone(rule: PersonMoment["rule"]): StandingTone {
  if (isLate(rule)) {
    return "warn";
  }
  return rule === "nothing_needed" ? "calm" : "accent";
}

// The two rules that mean somebody is being kept waiting. A promise past its
// date is late whether it was read out of an email or filed as a task — one
// rung covers both — while a promise not yet due is a live thread, not a
// warning.
export function isLate(rule: PersonMoment["rule"]): boolean {
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
  moment: PersonMoment,
  othersInTheList: boolean,
): boolean {
  return moment.rule !== "nothing_needed" || !othersInTheList;
}

/**
 * The moment as the lead row of the needs list: the rule it fired on as the
 * eyebrow, the headline in the display face, why now, the evidence one
 * disclosure away, and the one filled verb on the page.
 *
 * The button appears only where the server said the action can be taken AND
 * named somewhere to go. A card whose verb lands nowhere is worse than a card
 * with no verb: the reader clicks, nothing happens, and they stop trusting
 * the ones that work.
 */
export function MomentRow({
  moment,
  onOpenRecord,
}: Readonly<{
  moment: PersonMoment;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const destination = moment.recommended_action.destination;
  const target =
    moment.recommended_action.state === "available" &&
    destination?.entity_type != null &&
    destination.entity_id != null
      ? { type: destination.entity_type, id: destination.entity_id }
      : undefined;
  const tone = standingTone(moment.rule);
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
        <Proof
          label={t("record.restsOn")}
          items={momentGrounding(moment.evidence, t, locale, recordZone)}
          count
        />
        {target && onOpenRecord && (
          <span className="co-move-do">
            <span className="co-move-actions">
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
          </span>
        )}
      </span>
    </PanelRow>
  );
}
