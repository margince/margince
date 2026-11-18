// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which deals a team should work to meet its quarter goal.
//
// The server ranks them once a day, when it freezes the team's forecast, and
// stores each rank with the facts behind it. This draws that stored list, so
// two managers looking on one day see one list.

import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatMoneyOrAbsent, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { signalSentence } from "../../lib/forecastsignals";
import { listReadState } from "../../lib/worklist.listread";
import { AFTER_THE_DAY } from "./worklist.layout";
import {
  type ForecastFocus,
  type ForecastFocusDeal,
  useForecastFocus,
} from "./worklist.queries";

type FocusTeam = ForecastFocus["teams"][number];

/** The ranked deals of every team the reader may read, one panel each. */
export function CloseTheGap() {
  const t = useT();
  const focus = useForecastFocus(true);
  const teams = focus.data?.teams;
  const state = listReadState(focus, teams);
  // A team held back by the individual-performance switch is not a team
  // without a list, so the page says which of the two it is.
  const withheld = focus.data?.withheld === true;
  return (
    <SurfaceState
      state={state}
      emptyLabel={t(withheld ? "worklist.gap.withheld" : "worklist.gap.empty")}
      loadingLabel={t("worklist.gap.loading")}
      detail={{ onRetry: () => void focus.refetch() }}
    >
      {teams?.map((team) => (
        <TeamGap key={team.team_id} team={team} />
      ))}
      {withheld && <p className="t-caption">{t("worklist.gap.withheld")}</p>}
    </SurfaceState>
  );
}

const STATUS_LINE = {
  behind: "worklist.gap.behind",
  on_target: "worklist.gap.onTarget",
  no_goal: "worklist.gap.noGoal",
} as const;

function TeamGap({ team }: Readonly<{ team: FocusTeam }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const number = (value: number) => formatNumber(value, locale);
  const line =
    team.status === "on_target" && team.deals.length === 0
      ? t("worklist.gap.nothingAtRisk")
      : t(STATUS_LINE[team.status] ?? "worklist.gap.noGoal");
  return (
    <Panel
      className={AFTER_THE_DAY}
      title={t("worklist.gap.title")}
      titleAction={<EntityRef kind="team" id={team.team_id} />}
    >
      <PanelBody>
        <p className="t-body">{line}</p>
        {team.deals.length > 0 && (
          <DataTable
            label={t("worklist.gap.title")}
            rows={team.deals}
            rowKey={(deal) => deal.deal_id}
            columns={[
              {
                key: "rank",
                header: t("worklist.gap.rank"),
                align: "end",
                render: (deal: ForecastFocusDeal) => number(deal.rank),
              },
              {
                key: "deal",
                header: t("worklist.gap.deal"),
                render: (deal: ForecastFocusDeal) => (
                  <EntityRef
                    kind="deal"
                    id={deal.deal_id}
                    name={deal.deal_name}
                  />
                ),
              },
              {
                key: "owner",
                header: t("worklist.gap.owner"),
                render: (deal: ForecastFocusDeal) => (
                  <EntityRef kind="user" id={deal.owner_id} />
                ),
              },
              {
                key: "weighted",
                header: t("worklist.gap.weighted"),
                align: "end",
                render: (deal: ForecastFocusDeal) =>
                  formatMoneyOrAbsent(
                    deal.weighted_minor,
                    team.base_currency,
                    locale,
                  ),
              },
              {
                key: "why",
                header: t("worklist.gap.why"),
                render: (deal: ForecastFocusDeal) => (
                  <>
                    {reasonLines(deal, t, plural, number).map((sentence) => (
                      <Badge key={sentence} tone="warning">
                        {sentence}
                      </Badge>
                    ))}
                  </>
                ),
              },
            ]}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * The stored facts behind one rank, each as a sentence: when the deal closes,
 * whether that date is a guess, and its risk signals. The score is never
 * stored, so it is never shown.
 */
function reasonLines(
  deal: ForecastFocusDeal,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  number: (value: number) => string,
): string[] {
  const out: string[] = [];
  const days = deal.reason.closes_in_days;
  if (days !== undefined) {
    out.push(closeSentence(days, t, plural, number));
  }
  if (deal.reason.close_provisional) {
    out.push(t("worklist.gap.provisional"));
  }
  for (const signal of deal.reason.signals) {
    out.push(signalSentence(signal, t, plural, () => undefined, number));
  }
  return [...new Set(out)];
}

function closeSentence(
  days: number,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  number: (value: number) => string,
): string {
  if (days === 0) return t("worklist.gap.closesToday");
  if (days < 0) {
    return plural("worklist.gap.closedAgo", -days, { count: number(-days) });
  }
  return plural("worklist.gap.closesIn", days, { count: number(days) });
}
