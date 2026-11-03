// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatMoneyOrAbsent, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { reveal } from "../../lib/reveal";
import { type Project360, stateOf } from "./projectsections";
import { commitmentsReading, phaseTimes } from "./projectstanding";
import { commitmentsPeek, documentsPeek } from "./projectstanding.peeks";
import { phaseName } from "./projectstanding.words";

// The figures at the foot of How it stands: time in the current phase, the
// open commitments and the files on the project. A cell whose source has a
// card or a drawer on the page opens it.

// Where a card's door leads: a scroll to the card, named here and placed by
// `projectcards.tsx`, so the two cannot drift apart.
export const PROJECT_DEALS_ANCHOR = "project-deals";
export const PROJECT_COMMITMENTS_ANCHOR = "project-commitments";
export const PROJECT_CONTRACTS_ANCHOR = "project-contracts";

export function ProjectFigures({
  view,
  onOpenDocuments,
}: Readonly<{ view: Project360; onOpenDocuments?: () => void }>) {
  return (
    <StatStrip density="plate" testId="project-figures">
      <PhaseTimeCell view={view} />
      <DealValueCell view={view} kind="open" />
      <DealValueCell view={view} kind="won" />
      <OpenCommitmentsCell view={view} />
      <DocumentsCell view={view} onOpen={onOpenDocuments} />
    </StatStrip>
  );
}

function DealValueCell({
  view,
  kind,
}: Readonly<{ view: Project360; kind: "open" | "won" }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const totals = view.rollups;
  const money =
    kind === "open" ? totals?.open_deal_value : totals?.won_deal_value;
  const unpriced =
    kind === "open" ? totals?.open_deals_unpriced : totals?.won_deals_unpriced;
  return (
    <StatCard
      label={t(
        kind === "open" ? "project.openDealValue" : "project.wonDealValue",
      )}
      value={
        totals
          ? formatMoneyOrAbsent(money?.amount_minor, money?.currency, locale)
          : t("state.withheld")
      }
      detail={
        unpriced
          ? plural("brief.readings.unpricedCount", unpriced, {
              count: formatNumber(unpriced, locale),
            })
          : undefined
      }
      onOpen={totals ? reveal(PROJECT_DEALS_ANCHOR) : undefined}
    />
  );
}

function PhaseTimeCell({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = t("project.standing.phaseTime");
  if (!view.phase_history) {
    return <StatCard label={label} value={t("state.withheld")} />;
  }
  const phase = view.project.phase;
  const days = phaseTimes(view).get(phase)?.days ?? 0;
  return (
    <StatCard
      label={label}
      value={plural("standing.days", days, {
        count: formatNumber(days, locale),
      })}
      detail={t("project.standing.inPhase", { phase: phaseName(phase, t) })}
    />
  );
}

function OpenCommitmentsCell({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = t("project.openTasks");
  const zone = useRecordZone();
  const reading = commitmentsReading(view);
  if (!reading) {
    return <StatCard label={label} value={t("state.withheld")} />;
  }
  const open = reveal(PROJECT_COMMITMENTS_ANCHOR);
  const overdue = formatNumber(reading.overdue, locale);
  return (
    <StatCard
      label={label}
      value={
        reading.open > 0
          ? plural("standing.open", reading.open, {
              count: formatNumber(reading.open, locale),
            })
          : t("project.standing.noneOpen")
      }
      tone={reading.overdue > 0 ? "danger" : undefined}
      detail={
        reading.overdue > 0
          ? plural(
              reading.overdueIsFloor
                ? "project.standing.overdueFloor"
                : "standing.overdue",
              reading.overdue,
              { count: overdue },
            )
          : undefined
      }
      peek={commitmentsPeek(view, { t, plural, locale, zone }, open)}
      onOpen={open}
    />
  );
}

// The newest file leads as the detail, so the cell says what the count is of.
function DocumentsCell({
  view,
  onOpen,
}: Readonly<{ view: Project360; onOpen?: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = t("project.documents.title");
  const zone = useRecordZone();
  const documents = view.documents?.data ?? [];
  const state = stateOf(
    view,
    "documents",
    Boolean(view.documents),
    documents.length,
  );
  if (state === "unavailable") {
    return (
      <StatCard
        label={label}
        value={t("project.standing.documentsUnavailable")}
      />
    );
  }
  if (documents.length === 0) {
    return (
      <StatCard
        label={label}
        value={t("project.standing.noDocuments")}
        onOpen={onOpen}
      />
    );
  }
  const newest = documents[0];
  const count = { count: formatNumber(documents.length, locale) };
  return (
    <StatCard
      label={label}
      value={
        view.documents?.page.has_more
          ? plural("project.standing.documentsFloor", documents.length, count)
          : formatNumber(documents.length, locale)
      }
      detail={newest?.title || newest?.filename}
      peek={documentsPeek(view, { t, plural, locale, zone })}
      onOpen={onOpen}
    />
  );
}
