// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead's team oversight on the Worklist.

import { useState } from "react";
import { SegmentedControl } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { TeamExceptionsPanel } from "./worklist.exceptions";
import { CloseTheGap } from "./worklist.gaplist";
import { AFTER_THE_DAY } from "./worklist.layout";

const OVERSIGHT_TABS = ["exceptions", "gap"] as const;

type OversightTab = (typeof OVERSIGHT_TABS)[number];

/**
 * On the team scope it is two tabs: what is going wrong, and which deals close
 * the gap to the goal. Any wider scope spans teams with no one goal, so it
 * keeps the exceptions alone.
 */
export function TeamOversight({
  scope,
  enabled,
  onOwner,
}: Readonly<{
  scope: string;
  enabled: boolean;
  onOwner: (id: string) => void;
}>) {
  const t = useT();
  const [tab, setTab] = useState<OversightTab>("exceptions");
  if (scope !== "team" || !enabled) {
    return <TeamExceptionsPanel enabled={enabled} onOwner={onOwner} />;
  }
  return (
    <>
      <div className={AFTER_THE_DAY}>
        <SegmentedControl
          options={OVERSIGHT_TABS}
          value={tab}
          onChange={setTab}
          label={t("worklist.gap.tabs")}
          labels={{
            exceptions: t("worklist.gap.exceptionsTab"),
            gap: t("worklist.gap.tab"),
          }}
        />
      </div>
      {tab === "exceptions" ? (
        <TeamExceptionsPanel enabled onOwner={onOwner} />
      ) : (
        <CloseTheGap />
      )}
    </>
  );
}
