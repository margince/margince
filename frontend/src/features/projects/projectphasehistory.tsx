// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { OverviewRow } from "../../design-system/overviewcard";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { daysPast } from "../../format/lateness";
import { useLocale, usePlural, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { phaseMoverSentence } from "./projectphasemover";
import { OverviewSection, type Project360, stateOf } from "./projectsections";
import { phaseName } from "./projectstanding.words";

type PhaseTransition = components["schemas"]["Project360PhaseTransition"];
type PhaseCause = components["schemas"]["Project360PhaseCause"];

/**
 * Every visit to a phase, oldest first: when it began and ended, why or by
 * whom, and how long that visit lasted. A reopened project lists a phase
 * twice, once per visit, so each row's days are its own.
 */
export function PhaseHistoryCard({
  view,
}: Readonly<{ view: Project360 | undefined }>) {
  const t = useT();
  const history = view?.phase_history;
  const rows = history?.data ?? [];
  const state = stateOf(view, "phase_history", Boolean(history), rows.length);
  return (
    <OverviewSection
      title={t("project.history.title")}
      state={state}
      emptyLabel={t("project.history.empty")}
    >
      {view &&
        rows.map((visit, index) => (
          <VisitRow
            key={visit.id}
            visit={visit}
            leftAt={rows[index + 1]?.changed_at}
            asOf={view.as_of}
          />
        ))}
    </OverviewSection>
  );
}

function VisitRow({
  visit,
  leftAt,
  asOf,
}: Readonly<{
  visit: PhaseTransition;
  // When the next move ended this visit; absent while it is the current one.
  leftAt?: string;
  asOf: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const from = formatDayMonth(visit.changed_at, locale, zone);
  const span = leftAt
    ? t("project.history.moved", {
        from,
        to: formatDayMonth(leftAt, locale, zone),
      })
    : t("project.history.since", { date: from });
  // A caused move names its cause in place of the person whose act set it off.
  const credit = isKnownCause(visit.cause) ? (
    <PhaseCauseWords cause={visit.cause} />
  ) : (
    phaseMoverSentence(visit.changed_by, !visit.from_phase, t)
  );
  const days = daysPast(
    Date.parse(visit.changed_at),
    Date.parse(leftAt ?? asOf),
  ).days;
  const length = plural("standing.days", days, {
    count: formatNumber(days, locale),
  });
  return (
    <OverviewRow
      title={phaseName(visit.to_phase, t)}
      sub={
        <>
          {`${span} · `}
          {credit}
          {visit.reason && ` · ${visit.reason}`}
        </>
      }
      right={leftAt ? length : <Badge>{length}</Badge>}
    />
  );
}

// A cause kind this build has no words for credits the mover instead, never
// something wrong.
function isKnownCause(cause: PhaseCause | undefined): cause is PhaseCause {
  return cause?.kind === "deal_won";
}

function PhaseCauseWords({ cause }: Readonly<{ cause: PhaseCause }>) {
  const t = useT();
  if (!cause.deal_id || !cause.deal_name) {
    return t("project.history.dealWonUnnamed");
  }
  const [before, after = ""] = t("project.history.dealWon").split("{name}");
  return (
    <>
      {before}
      <EntityRef kind="deal" id={cause.deal_id} name={cause.deal_name} />
      {after}
    </>
  );
}
