// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, EmptyState } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";
import type { AnalyticsScope } from "./analytics.context";

// Said when the server reports that the population an answer measured holds
// no deal, archived ones included. The server computes the flag with the
// figures, so the notice and the numbers cannot describe different
// populations. Unknown (absent) and true say nothing.
export function FirstRunNotice({
  holdsDeals,
  scope,
  allowedScopes,
  onSelectScope,
}: Readonly<{
  holdsDeals: boolean | undefined;
  scope: AnalyticsScope;
  allowedScopes: readonly AnalyticsScope[];
  onSelectScope: (scope: AnalyticsScope) => void;
}>) {
  const t = useT();
  if (holdsDeals !== false) {
    return null;
  }
  const company = allowedScopes.find((offer) => offer.kind === "workspace");
  if (scope.kind === "owner" && company) {
    return (
      <EmptyState
        title={t("analytics.emptyOwnTitle")}
        action={
          <Button onClick={() => onSelectScope(company)}>
            {t("analytics.emptyOwnAction")}
          </Button>
        }
      >
        {t("analytics.emptyOwnBody")}
      </EmptyState>
    );
  }
  if (scope.kind === "team") {
    return (
      <EmptyState title={t("analytics.emptyTeamTitle", { scope: scope.label })}>
        {t("analytics.emptyTeamBody", { scope: scope.label })}
      </EmptyState>
    );
  }
  return (
    <EmptyState
      title={t("analytics.firstRunTitle")}
      action={
        <Button onClick={() => navigate({ screen: "deals" })}>
          {t("analytics.firstRunAction")}
        </Button>
      }
    >
      {t("analytics.firstRunBody")}
    </EmptyState>
  );
}
