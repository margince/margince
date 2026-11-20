// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SegmentedControl } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { WeekTeamSelect } from "./brief.teamselect";
import { type BriefAddress, type BriefView, VIEWS } from "./brief.view";

import "./brief.dials.css";

// The dials above the page: the day or the week, and on Weekly whose week.
export function BriefDials({
  address,
  onChange,
}: Readonly<{
  address: BriefAddress;
  onChange: (next: BriefAddress) => void;
}>) {
  const t = useT();
  return (
    <div className="brief-dials">
      <SegmentedControl<BriefView>
        label={t("brief.view.label")}
        options={VIEWS}
        value={address.view}
        labels={{
          morning: t("brief.view.morning"),
          weekly: t("brief.view.weekly"),
        }}
        onChange={(view) => onChange({ ...address, view })}
      />
      {address.view === "weekly" && (
        <WeekTeamSelect
          team={address.team}
          onChange={(team) => onChange({ ...address, team })}
        />
      )}
    </div>
  );
}
