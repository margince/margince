// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef } from "../../components/entityref";
import { bundleKinds } from "../../components/marginchanges";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody, PanelGroupHead } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatMoney, formatNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import {
  type TeamMarginceChanges,
  type TeamMarginceOwnerlessDeal,
  useTeamMarginceChanges,
} from "../../lib/marginchanges";
import { listReadState } from "../../lib/worklist.listread";
import { AFTER_THE_DAY } from "./worklist.layout";

type Group = TeamMarginceChanges["groups"][number];
type Ownerless = TeamMarginceOwnerlessDeal;

/**
 * What Margince changed on the lead's team this week, per teammate and kind.
 * Mounted only for a lead, the tier the server answers.
 */
export function TeamMarginceChangesPanel({
  enabled,
}: Readonly<{ enabled: boolean }>) {
  return enabled ? <TeamChanges /> : null;
}

function TeamChanges() {
  const t = useT();
  const { locale } = useLocale();
  const read = useTeamMarginceChanges(true);
  const groups = read.data?.groups;
  const state = listReadState(read, groups);
  const base = read.data?.base_currency ?? "";
  // A server from before the list sends none.
  const ownerless = read.data?.ownerless ?? [];
  return (
    <Panel className={AFTER_THE_DAY} title={t("changes.team.title")}>
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("changes.team.empty")}
          loadingLabel={t("changes.team.loading")}
          detail={{ onRetry: () => void read.refetch() }}
        >
          {groups && (
            <DataTable
              label={t("changes.team.title")}
              rows={groups}
              rowKey={(row: Group) => `${row.owner_id}:${row.kind}`}
              columns={[
                {
                  key: "owner",
                  header: t("changes.team.owner"),
                  render: (row: Group) => row.owner_name,
                },
                {
                  key: "kind",
                  header: t("changes.team.kind"),
                  render: (row: Group) => bundleKinds([row.kind], t),
                },
                {
                  key: "deals",
                  header: t("changes.team.deals"),
                  render: (row: Group) => formatNumber(row.deals, locale),
                },
                {
                  key: "moved",
                  header: t("changes.team.valueMoved"),
                  render: (row: Group) =>
                    base
                      ? formatMoney(row.value_moved_base_minor, base, locale)
                      : "",
                },
                {
                  key: "undone",
                  header: t("changes.team.undone"),
                  render: (row: Group) => formatNumber(row.undone, locale),
                },
              ]}
            />
          )}
          {read.data?.truncated && (
            <p className="t-caption">{t("changes.team.truncated")}</p>
          )}
        </SurfaceState>
      </PanelBody>
      {ownerless.length > 0 && (
        <OwnerlessDeals
          deals={ownerless}
          truncated={read.data?.ownerless_truncated ?? false}
          base={base}
        />
      )}
    </Panel>
  );
}

/**
 * The open deals Margince leaves alone because nobody can act for them, each
 * a link to the deal where somebody can give it an owner.
 */
function OwnerlessDeals({
  deals,
  truncated,
  base,
}: Readonly<{ deals: Ownerless[]; truncated: boolean; base: string }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <PanelGroupHead title={t("changes.team.ownerless.title")} level="h3" />
      <PanelBody>
        <DataTable
          label={t("changes.team.ownerless.title")}
          rows={deals}
          rowKey={(row: Ownerless) => row.deal_id}
          columns={[
            {
              key: "deal",
              header: t("changes.team.ownerless.deal"),
              render: (row: Ownerless) => (
                <EntityRef kind="deal" id={row.deal_id} name={row.deal_name} />
              ),
            },
            {
              key: "reason",
              header: t("changes.team.ownerless.reason"),
              render: (row: Ownerless) => ownerlessReason(row, t),
            },
            {
              key: "value",
              header: t("changes.team.ownerless.value"),
              render: (row: Ownerless) =>
                base && row.value_base_minor !== undefined
                  ? formatMoney(row.value_base_minor, base, locale)
                  : "",
            },
          ]}
        />
        {truncated && (
          <p className="t-caption">{t("changes.team.ownerless.truncated")}</p>
        )}
      </PanelBody>
    </>
  );
}

/** Why Margince leaves a deal alone, naming the owner who left. */
export function ownerlessReason(row: Ownerless, t: Translator): string {
  if (row.reason === "no_owner") {
    return t("changes.team.ownerless.no_owner");
  }
  return row.owner_name
    ? t("changes.team.ownerless.departed_owner", { name: row.owner_name })
    : t("changes.team.ownerless.departed_unnamed");
}
