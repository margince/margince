// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Children } from "react";
import type { components } from "../../api/schema";
import { EmptyState, Skeleton } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import "./company360.css";
import type { TodayReading } from "./companytoday.reading";

export { useTodayReading } from "./companytoday.reading";

export type Company360 = components["schemas"]["Company360"];

/**
 * What needs a contact, as rows of the list it sits in. The agent's finds
 * come first, then the manual moves, or one sentence for a day with none.
 * A read in flight or failed never says the day is quiet: it has no basis
 * for that claim. The foot is the list's to draw.
 */
export function NeedsList({ reading }: Readonly<{ reading: TodayReading }>) {
  const t = useT();
  if (reading.state === "loading") {
    return (
      <PanelBody>
        <Skeleton width="100%" height={64} />
      </PanelBody>
    );
  }
  if (reading.state === "failed") {
    return (
      <PanelBody>
        <EmptyState>{t("today.failed")}</EmptyState>
      </PanelBody>
    );
  }
  // Counted rather than tested for truthiness: the rows are arrays, and an
  // empty array is truthy.
  const quiet = Children.toArray(reading.rows).length === 0;
  return (
    <>
      {reading.rows}
      {quiet && reading.complete && (
        <PanelBody>
          <p className="co-tasklist-none">{t("today.quiet")}</p>
        </PanelBody>
      )}
      {reading.notice}
    </>
  );
}
