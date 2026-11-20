// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ListOrdered } from "lucide-react";
import { Badge, Button } from "../../design-system/atoms";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { formatNumber, formatPercent } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { routeHash } from "../../lib/router";
import { ReportCard } from "./brief.reportcard";
import type {
  TeamWeeklyFocusKind,
  TeamWeeklyRep,
  TeamWeeklyReview,
} from "./teamweekly.queries";

// The Monday agenda: one item per member, in the order the review ranks them.
// No minute budgets: the product knows neither the team's meeting length nor
// what a fair share of it would be.

/** The focus rules that name something to copy rather than to coach. */
const CELEBRATED: ReadonlySet<TeamWeeklyFocusKind> = new Set([
  "strong_week",
  "quiet_week",
]);

const FOCUS_LABEL: Readonly<Record<TeamWeeklyFocusKind, MessageKey>> = {
  help_requested: "teamweekly.focus.help_requested",
  deals_at_risk: "teamweekly.focus.deals_at_risk",
  forecast_downgrades: "teamweekly.focus.forecast_downgrades",
  stage_regressions: "teamweekly.focus.stage_regressions",
  close_dates_provisional: "teamweekly.focus.close_dates_provisional",
  no_next_step_coverage: "teamweekly.focus.no_next_step_coverage",
  leads_breached: "teamweekly.focus.leads_breached",
  commitments_missed: "teamweekly.focus.commitments_missed",
  meetings_without_next_step: "teamweekly.focus.meetings_without_next_step",
  strong_week: "teamweekly.focus.strong_week",
  quiet_week: "teamweekly.focus.quiet_week",
};

/**
 * The agenda as rows. `agenda` is the server's order over the reps already on
 * the snapshot, so the ranking rule lives in Go alone. An id the reps do not
 * carry is dropped rather than drawn as a nameless item.
 */
export function agendaRows(review: TeamWeeklyReview): readonly TeamWeeklyRep[] {
  const byId = new Map(review.reps.map((rep) => [rep.user_id, rep]));
  return review.agenda
    .map((id) => byId.get(id))
    .filter((rep): rep is TeamWeeklyRep => rep !== undefined);
}

/** The agenda as the text Copy puts on the clipboard, built from the rows shown. */
export function agendaText(
  rows: readonly TeamWeeklyRep[],
  heading: string,
): string {
  const items = rows.map((rep, index) =>
    [
      `${index + 1}. ${rep.display_name} — ${rep.focus_label}`,
      ...(rep.patterns ?? []).slice(1).map((pattern) => pattern.label),
    ].join("; "),
  );
  return [heading, ...items].join("\n");
}

/** How long the meeting is and who opens it, from the same rows as the list. */
export function AgendaSummary({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const rows = agendaRows(review);
  const [first] = rows;
  if (first === undefined) return null;
  return (
    <p
      className="teamweekly-agenda-summary"
      data-testid="teamweekly-agenda-summary"
    >
      {plural("teamweekly.agenda.summary", rows.length, {
        count: formatNumber(rows.length, locale),
        first: first.display_name,
      })}
    </p>
  );
}

function CopyAgenda({ rows }: Readonly<{ rows: readonly TeamWeeklyRep[] }>) {
  const t = useT();
  const copy = useClipboardCopy(
    agendaText(rows, t("teamweekly.agenda.title")),
    {
      copy: t("teamweekly.agenda.copy"),
      copied: t("teamweekly.agenda.copied"),
      remedy: t("teamweekly.agenda.copyFailed"),
    },
  );
  return (
    <>
      <Button onClick={copy.copy}>{copy.label}</Button>
      {copy.notice}
    </>
  );
}

/**
 * Every member gets an item, the one whose week went well too. An agenda that
 * names only the troubled members reads as a team of only them.
 */
export function TeamAgenda({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = agendaRows(review).map((rep) =>
    rep.focus_kind === "quiet_week"
      ? { ...rep, focus_label: t("teamweekly.noPriority") }
      : rep,
  );
  return (
    <ReportCard
      icon={ListOrdered}
      title={t("teamweekly.agenda.title")}
      aside={rows.length > 0 ? <CopyAgenda rows={rows} /> : undefined}
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
            <a
              className="entity-link"
              href={routeHash({ screen: "worklist", id: rep.user_id })}
            >
              {rep.display_name}
            </a>
            <span className="teamweekly-agenda-focus">
              <Badge
                tone={
                  rep.focus_kind === "quiet_week"
                    ? undefined
                    : CELEBRATED.has(rep.focus_kind)
                      ? "success"
                      : "warning"
                }
              >
                {t(labelFor(FOCUS_LABEL, rep.focus_kind))}
              </Badge>
              {/* Composed by the server from the stored figures, never by a
                  model, so it cannot say what the snapshot does not hold. */}
              <span>{rep.focus_label}</span>
              {(rep.patterns ?? []).slice(1).map((pattern) => (
                <Badge key={pattern.kind}>
                  {t(labelFor(FOCUS_LABEL, pattern.kind))}
                </Badge>
              ))}
            </span>
            <FunnelLines rep={rep} />
          </div>
        </PanelRow>
      ))}
    </ReportCard>
  );
}

/** Where this member's stages convert differently from the team's. */
function FunnelLines({ rep }: Readonly<{ rep: TeamWeeklyRep }>) {
  const t = useT();
  const { locale } = useLocale();
  const funnel = rep.funnel ?? [];
  if (funnel.length === 0) return null;
  const rate = (part: number, whole: number) =>
    formatPercent(whole === 0 ? 0 : part / whole, locale);
  return (
    <ul className="teamweekly-agenda-funnel">
      {funnel.map((row) => {
        const values = {
          stage: row.stage_name,
          next: row.next_stage_name ?? "",
          rep: rate(row.rep_converted, row.rep_entered),
          team: rate(row.team_converted, row.team_entered),
        };
        return (
          <li key={row.stage_id}>
            {row.next_stage_name
              ? t("teamweekly.funnel.line", values)
              : t("teamweekly.funnel.lineLast", values)}
          </li>
        );
      })}
    </ul>
  );
}
