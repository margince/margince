// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import { AccountCoverageView } from "./analytics.accounts";
import type { AnalyticsSelection } from "./analytics.context";
import { DataCoverageView } from "./analytics.coverageview";
import { MyOutcomesView } from "./analytics.outcomes";

// The sections that need the frame alone: the zone for a date, the seat's
// default population, or the chosen scope.
export function FramedSection({
  section,
  context,
  selection,
  scopeControl,
  locale,
}: Readonly<{
  section: "coverage" | "accounts" | "outcomes";
  context: components["schemas"]["AnalyticsContext"];
  selection: AnalyticsSelection | null;
  scopeControl: ReactNode;
  locale: Locale;
}>) {
  switch (section) {
    case "coverage":
      return <DataCoverageView locale={locale} timezone={context.timezone} />;
    case "accounts":
      return selection ? (
        <AccountCoverageView
          scope={selection.scope}
          scopeControl={scopeControl}
          locale={locale}
          timezone={context.timezone}
        />
      ) : null;
    case "outcomes":
      return (
        <MyOutcomesView defaultScope={context.default_scope} locale={locale} />
      );
  }
}
