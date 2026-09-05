// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { EntityRef } from "./entityref";
import type {
  TeamWeeklyFocusKind,
  TeamWeeklyRep,
  TeamWeeklyReview,
} from "./teamweekly.queries";

import "./brief.teamweeklyagenda.css";

// The Monday agenda: the team's own week, in the order the review already
// ranks it.
//
// As many items as there are members, never a fixed number, and no minute
// budgets: a week with two things worth discussing is a two-item meeting, and a
// budget beside an item would read as guidance derived from a team size and a
// meeting length the product does not know.

/** Which focus rules are a coaching prompt and which are something to copy. */
const CELEBRATED: ReadonlySet<TeamWeeklyFocusKind> = new Set([
  "strong_week",
  "quiet_week",
]);

const FOCUS_LABEL: Readonly<Record<TeamWeeklyFocusKind, MessageKey>> = {
  help_requested: "teamweekly.focus.help_requested",
  leads_breached: "teamweekly.focus.leads_breached",
  commitments_missed: "teamweekly.focus.commitments_missed",
  meetings_without_next_step: "teamweekly.focus.meetings_without_next_step",
  strong_week: "teamweekly.focus.strong_week",
  quiet_week: "teamweekly.focus.quiet_week",
};

/**
 * The agenda, as rows to draw.
 *
 * `agenda` is an ORDER — the server sends the rep ids permuted into the order a
 * lead should raise them, and the rows themselves are the reps already on the
 * snapshot. Reading it this way is what keeps the ranking in one place: the
 * priority that decided the order is the same rule that picked each focus, and
 * it is spelled in Go and nowhere else.
 *
 * An id the reps do not carry is dropped rather than drawn as a blank row. The
 * contract says the agenda is a permutation of `reps` and the server holds it,
 * so this cannot happen against a current server — but a client that trusted it
 * would draw a nameless row against an older one, and a meeting agenda with an
 * anonymous item on it is worse than one item short.
 */
export function agendaRows(review: TeamWeeklyReview): readonly TeamWeeklyRep[] {
  const byId = new Map(review.reps.map((rep) => [rep.user_id, rep]));
  return review.agenda
    .map((id) => byId.get(id))
    .filter((rep): rep is TeamWeeklyRep => rep !== undefined);
}

/**
 * The agenda as text, which is what Copy puts on the clipboard.
 *
 * Composed from the rows on screen rather than from the review again, so what
 * is copied is what was read — the point of the action in an actual Monday
 * meeting is that the two agree.
 */
export function agendaText(
  rows: readonly TeamWeeklyRep[],
  heading: string,
): string {
  const items = rows.map(
    (rep, index) => `${index + 1}. ${rep.display_name} — ${rep.focus_label}`,
  );
  return [heading, ...items].join("\n");
}

/**
 * The one-line reading of the agenda for the header band.
 *
 * It says how long the meeting is and what opens it, which are the two facts
 * somebody scanning the header wants; the items themselves are in the panel.
 * Both are drawn from agendaRows, so the summary cannot promise an item the
 * list below does not have.
 */
export function AgendaSummary({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = agendaRows(review);
  if (rows.length === 0) {
    return null;
  }
  return (
    <p
      className="teamweekly-agenda-summary"
      data-testid="teamweekly-agenda-summary"
    >
      {t("teamweekly.agenda.summary", {
        count: formatNumber(rows.length, locale),
        first: rows[0].display_name,
      })}
    </p>
  );
}

/** Copy agenda, and what to say when the browser will not hand over a clipboard. */
function CopyAgenda({ rows }: Readonly<{ rows: readonly TeamWeeklyRep[] }>) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);

  async function copy() {
    // navigator.clipboard is UNDEFINED outside a secure context, so this is a
    // missing capability rather than a rejected promise, and asking for
    // writeText on it would throw before any catch could say why.
    const writer = navigator.clipboard;
    if (!writer) {
      setFailed(true);
      return;
    }
    try {
      await writer.writeText(agendaText(rows, t("teamweekly.agenda.title")));
      setCopied(true);
      setFailed(false);
    } catch {
      setCopied(false);
      setFailed(true);
    }
  }

  return (
    <>
      <Button small onClick={() => void copy()}>
        {copied ? t("teamweekly.agenda.copied") : t("teamweekly.agenda.copy")}
      </Button>
      {failed && (
        <Callout tone="danger" live="alert">
          {t("teamweekly.agenda.copyFailed")}
        </Callout>
      )}
    </>
  );
}

/**
 * The agenda itself: one numbered item per member, the thing to raise first at
 * the top and the quiet week last.
 *
 * Every member gets an item, including the one whose week went well — a meeting
 * that lists only the troubled people reads as a team where only those people
 * exist, which is both untrue and demoralising to be named in.
 */
export function AgendaPanel({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = agendaRows(review);
  return (
    <Panel
      title={t("teamweekly.agenda.title")}
      sub={t("teamweekly.agenda.sub")}
      titleAction={rows.length > 0 ? <CopyAgenda rows={rows} /> : undefined}
    >
      {rows.length === 0 && (
        <PanelBody>
          <p>{t("teamweekly.agenda.empty")}</p>
        </PanelBody>
      )}
      {rows.map((rep, index) => (
        <PanelRow key={rep.user_id}>
          <div className="teamweekly-agenda-item">
            <span className="teamweekly-agenda-ordinal" aria-hidden="true">
              {formatNumber(index + 1, locale)}
            </span>
            <span className="teamweekly-agenda-name">
              <EntityRef kind="user" id={rep.user_id} name={rep.display_name} />
            </span>
            <span className="teamweekly-agenda-focus">
              <Badge
                quiet
                tone={CELEBRATED.has(rep.focus_kind) ? "success" : "warn"}
              >
                {t(FOCUS_LABEL[rep.focus_kind])}
              </Badge>
              {/* Composed by the server from the stored figures, never
                  model-written, so it cannot say what the snapshot does not
                  hold. */}
              <span>{rep.focus_label}</span>
            </span>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}
