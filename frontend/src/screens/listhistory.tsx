// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed on a list, newest first: every change of what the list is,
// every Shortlist membership change, and every record a Live List's checks saw
// joining or leaving, about a record this reader can see now.

import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type ListHistoryEntry, useListHistory } from "./lists.queries";

const KIND_LABEL: Record<ListHistoryEntry["kind"], MessageKey> = {
  member_added: "lists.history.added",
  member_removed: "lists.history.removed",
  member_entered: "lists.history.entered",
  member_left: "lists.history.left",
  revised: "lists.history.revised",
};

/** Why a change happened; an ordinary check needs no words beyond its row. */
const REASON_LABEL: Record<
  NonNullable<ListHistoryEntry["reason"]>,
  MessageKey | null
> = {
  chosen: "lists.history.reason.chosen",
  bulk: "lists.history.reason.bulk",
  record_archived: "lists.history.reason.archived",
  record_restored: "lists.history.reason.restored",
  evaluated: null,
  filter_changed: "lists.history.reason.filterChanged",
  automation: "lists.history.reason.automation",
};

/** A change a Live List's check saw, rather than one somebody made. */
function isObserved(row: ListHistoryEntry): boolean {
  return row.kind === "member_entered" || row.kind === "member_left";
}

export function ListHistoryPanel({
  listID,
  live,
}: Readonly<{ listID: string; live: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const history = useListHistory(listID);
  const rows = history.data?.data ?? [];
  const when = (row: ListHistoryEntry) =>
    formatDateTime(row.occurred_at, locale, viewerZone());
  const reasonOf = (row: ListHistoryEntry) => {
    const key = row.reason ? REASON_LABEL[row.reason] : null;
    return key ? ` · ${t(key)}` : "";
  };
  return (
    <Panel title={t("lists.history.title")}>
      <PanelBody>
        {live && <p className="t-caption">{t("lists.history.liveNote")}</p>}
        <SurfaceState
          state={
            history.isPending
              ? "loading"
              : history.isError
                ? "unavailable"
                : rows.length > 0
                  ? "ready"
                  : "empty"
          }
          emptyLabel={t("lists.history.empty")}
          loadingLabel={t("lists.history.loading")}
          loadingLines={3}
        >
          <DataTable
            label={t("lists.history.title")}
            rows={rows}
            rowKey={(row) => row.id}
            columns={[
              {
                key: "when",
                header: t("lists.history.when"),
                render: when,
              },
              {
                key: "what",
                header: t("lists.history.what"),
                grow: true,
                render: (row) => (
                  <span>
                    {t(KIND_LABEL[row.kind], { when: when(row) })}
                    {reasonOf(row)}
                    {row.note && (
                      <span className="t-caption"> — {row.note}</span>
                    )}
                  </span>
                ),
              },
              {
                key: "who",
                header: t("lists.history.who"),
                render: (row) =>
                  isObserved(row)
                    ? t("lists.history.checker")
                    : (row.actor_name ?? t("lists.history.someone")),
              },
            ]}
          />
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
