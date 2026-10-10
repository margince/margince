// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { KeyedName } from "../design-system/keyedname";
import { RowOpen } from "../design-system/rowopen";
import { forReader } from "../format/collate";
import { useLocale, useT } from "../i18n";
import { ProviderCallsLine } from "./ai-call-figures";
import {
  ProviderHealthBadge,
  type ProviderHealthEntry,
} from "./ai-provider-health-notice";
import { providerName } from "./ai-provider-names";
import {
  type ProviderState,
  type ProviderUsage,
  providerState,
  STATE_LABEL,
  STATE_TONE,
} from "./ai-provider-sheet";
import { TierChips } from "./ai-terms";

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];

type Row = Readonly<{
  status: ProviderStatus;
  name: string;
  usage: ProviderUsage | undefined;
  health: ProviderHealthEntry | undefined;
  state: ProviderState;
}>;

// Attention first: failing or bound without a key, then in use, then the rest.
const ATTENTION: Readonly<Record<ProviderState, number>> = {
  needs_key: 0,
  active: 1,
  ready: 2,
  inactive: 3,
};

export function ProviderTable({
  providers,
  usage,
  health,
  canManage,
  onOpen,
}: Readonly<{
  providers: readonly ProviderStatus[];
  usage: ReadonlyMap<string, ProviderUsage> | null;
  // Present for a reader who may see diagnostics.
  health: readonly ProviderHealthEntry[] | undefined;
  // Without it the sheet opens read-only, so the verb says Open, not Edit.
  canManage: boolean;
  onOpen: (provider: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = providers
    .map((status) => {
      const used = usage?.get(status.provider);
      return {
        status,
        name: providerName(status.provider, t),
        usage: used,
        health: health?.find((h) => h.provider === status.provider),
        state: providerState(status, used),
      };
    })
    .sort((a, b) => rank(a) - rank(b) || forReader(a.name, b.name, locale));
  return (
    <DataTable<Row>
      bleed
      fold
      label={t("aiProviderKeys.title")}
      rows={rows}
      rowKey={(row) => row.status.provider}
      rowTestId={(row) => `ai-provider-row-${row.status.provider}`}
      onRowClick={(row) => onOpen(row.status.provider)}
      columns={[
        {
          key: "provider",
          header: t("aiTerms.provider"),
          render: (row) => (
            <CellStack>
              <KeyedName name={row.name} code={row.status.provider} />
              <ProviderCallsLine provider={row.status.provider} />
            </CellStack>
          ),
        },
        {
          key: "used",
          header: t("aiProviders.colUsedBy"),
          grow: true,
          render: (row) =>
            row.usage ? <TierChips tiers={row.usage.for} /> : null,
        },
        {
          key: "state",
          header: t("aiProviders.colStatus"),
          fold: "end",
          render: (row) => <StateBadges row={row} />,
        },
        {
          key: "open",
          header: t("table.actions"),
          headerHidden: true,
          align: "end",
          fold: "end",
          render: (row) => (
            <RowOpen
              label={t(
                canManage ? "aiRouting.editNamed" : "aiRouting.openNamed",
                {
                  name: row.name,
                },
              )}
              onOpen={() => onOpen(row.status.provider)}
            />
          ),
        },
      ]}
    />
  );
}

function rank(row: Row): number {
  return row.health ? 0 : ATTENTION[row.state];
}

// A vendor in use says nothing of its state, since its tier chips already do.
// A failing one adds how, beside the state and never in its place.
function StateBadges({ row }: Readonly<{ row: Row }>) {
  const t = useT();
  return (
    <span className="ai-provider-badges">
      {row.state === "active" ? null : (
        <Badge tone={STATE_TONE[row.state]}>{t(STATE_LABEL[row.state])}</Badge>
      )}
      {row.health && <ProviderHealthBadge health={row.health.health} />}
    </span>
  );
}
