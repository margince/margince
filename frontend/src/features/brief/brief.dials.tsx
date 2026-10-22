// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SegmentedControl } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { type BriefAddress, type BriefView, VIEWS } from "./brief.view";

import "./brief.dials.css";

// The one dial above the page: the day or the week.
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
    </div>
  );
}
