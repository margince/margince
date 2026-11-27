// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { PageHeadAction } from "../../components/pageheadaction";
import { FilterBar } from "../../design-system/filterbar";
import { useT } from "../../i18n";

/**
 * An Overview view's head: which view, whose records and which period stand
 * beside the page's name, so the question reads before the figures. Under the
 * tab row, one quiet line says what the figures cover.
 */
export function OverviewHead({
  switcher,
  dials,
  caption,
}: Readonly<{
  switcher: ReactNode;
  // Scope and period, in that order: the two a reader turns most.
  dials: ReactNode;
  caption?: ReactNode;
}>) {
  const t = useT();
  return (
    <>
      <PageHeadAction>
        <div className="analytics-head">
          <FilterBar label={t("analytics.filters")} quiet labels="inside">
            {switcher}
            <span className="analytics-head-dials">{dials}</span>
          </FilterBar>
        </div>
      </PageHeadAction>
      {caption != null && (
        <p className="analytics-asof t-caption" role="status">
          {caption}
        </p>
      )}
    </>
  );
}
