// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SegmentedControl } from "../design-system/atoms";
import { useT } from "../i18n";
import {
  type BriefAddress,
  type BriefScope,
  type BriefView,
  SCOPES,
  scopesFor,
  VIEWS,
} from "./brief.view";

import "./brief.dials.css";

// The two dials, in one band above the page.
//
// A control is drawn only where it has something to choose between: a rep whose
// scope reaches no team gets the view dial alone, because a segmented control
// with one option asks a reader to confirm what they cannot change.

export function BriefDials({
  address,
  offered,
  onChange,
}: Readonly<{
  address: BriefAddress;
  /** Whether this reader's row scope reaches a team, off the worklist read. */
  offered: boolean;
  onChange: (next: BriefAddress) => void;
}>) {
  const t = useT();
  const scopes = scopesFor(address.view, offered);
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
      {scopes.length > 1 && (
        <SegmentedControl<BriefScope>
          label={t("brief.scope.label")}
          options={SCOPES}
          value={address.scope}
          labels={{ mine: t("brief.scope.mine"), team: t("brief.scope.team") }}
          onChange={(scope) => onChange({ ...address, scope })}
        />
      )}
    </div>
  );
}
