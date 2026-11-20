// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PageZones } from "../../design-system/pagezones";
import { useT } from "../../i18n";
import { useUrlParams } from "../../lib/urlstate";
import { PlanSection } from "./brief.plan";
import { PlanConstraints } from "./brief.plan.contract";
import { TeamWeeklySection } from "./brief.teamweekly";
import { WeeklySection } from "./brief.weekly";
import { useWeeklyRailShown, WeeklyRail } from "./brief.weekly.rail";

export function WeeklyPage({ team }: Readonly<{ team?: string }>) {
  const t = useT();
  const [params] = useUrlParams();
  // An aside handed over empty still draws its column and landmark.
  const railShown = useWeeklyRailShown();
  if (team)
    return (
      <PageZones
        shape="aside"
        panes
        className="brief-weekly-page"
        mainClassName="brief-main"
        main={<TeamWeeklySection teamId={team} week={params.get("week")} />}
      />
    );
  return (
    <PageZones
      shape="aside"
      panes
      className="brief-weekly-page"
      mainClassName="brief-main"
      asideClassName="brief-rail"
      asideLabel={t("brief.weekly.rail")}
      main={<WeeklySection plan={<PlanSection withContract={false} />} />}
      aside={
        railShown ? <WeeklyRail constraints={<PlanConstraints />} /> : undefined
      }
    />
  );
}
