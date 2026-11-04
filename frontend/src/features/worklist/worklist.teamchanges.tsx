// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { bundleKinds } from "../../components/marginchanges";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatMoney, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  type TeamMarginceChanges,
  useTeamMarginceChanges,
} from "../../lib/marginchanges";
import { listReadState } from "../../lib/worklist.listread";
import { AFTER_THE_DAY } from "./worklist.layout";

type Group = TeamMarginceChanges["groups"][number];

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
                  key: "value",
                  header: t("changes.team.value"),
                  render: (row: Group) =>
                    base ? formatMoney(row.value_base_minor, base, locale) : "",
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
    </Panel>
  );
}
