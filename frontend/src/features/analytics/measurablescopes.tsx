// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, PendingBody } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { useSalesGoalRoster } from "./progress.queries";

export function useMeasurableScopes(
  scopes: readonly AnalyticsScope[],
  year: number,
  enabled = true,
  writeOnly = false,
) {
  const allowed = useCan("sales_target", "read") && enabled;
  const roster = useSalesGoalRoster(year, allowed);
  const choices = new Map(scopes.map((scope) => [scopeKey(scope), scope]));
  // A disabled query may retain an earlier grant's data in the cache.
  for (const member of allowed ? (roster.data?.data ?? []) : []) {
    if (writeOnly && !member.can_edit) continue;
    const scope: AnalyticsScope = {
      kind: "owner",
      id: member.owner_id,
      label: member.name,
    };
    choices.set(scopeKey(scope), scope);
  }
  return { scopes: [...choices.values()], roster, allowed };
}

export function MeasurableRosterState({
  read,
}: Readonly<{ read: ReturnType<typeof useMeasurableScopes> }>) {
  const t = useT();
  if (!read.allowed) return null;
  if (read.roster.isPending) return <PendingBody label={t("goals.reading")} />;
  return (
    <ErrorLine
      error={read.roster.error}
      actions={
        read.roster.isError ? (
          <Button onClick={() => read.roster.refetch()}>
            {t("common.retry")}
          </Button>
        ) : undefined
      }
    />
  );
}
