// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useClaimSettle } from "../../components/taskactions.rowwrites";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { activityDay, type OccurredPrecision } from "../../format/activitywhen";
import { formatDayMonth } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useViewerZone } from "../../lib/viewerzone";

/** What a promise that looks kept points at: who accepted which meeting. */
export type LooksKept = {
  guest: string;
  at: string;
  precision: OccurredPrecision;
};

/**
 * A promise to invite someone that looks kept. The button is the ordinary
 * settle, so the audit, the task and the offers behave as for any colleague who
 * marks a promise kept.
 */
export function LooksKeptLine({
  id,
  looksKept,
}: Readonly<{ id: string; looksKept: LooksKept }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const settle = useClaimSettle([]);
  const canSettle = useCanWrite("contact", "update");
  return (
    <span className="pe-loop-quote t-caption">
      {t("commitment.looksKept", {
        name: looksKept.guest,
        date: activityDay(
          looksKept.at,
          looksKept.precision,
          locale,
          zone,
          formatDayMonth,
        ),
      })}{" "}
      {canSettle && (
        <Button
          variant="link"
          pending={settle.isPending}
          onClick={() => settle.mutate({ id, outcome: "done" })}
        >
          {t("commitment.markKept")}
        </Button>
      )}
      <ErrorLine error={settle.error} inline />
    </span>
  );
}
