// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed on a list, newest first: every change of what the list is, and
// every Shortlist membership change of a record this reader can see now.

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
  revised: "lists.history.revised",
};

const REASON_LABEL: Record<
  NonNullable<ListHistoryEntry["reason"]>,
  MessageKey
> = {
  chosen: "lists.history.reason.chosen",
  bulk: "lists.history.reason.bulk",
  record_archived: "lists.history.reason.archived",
  record_restored: "lists.history.reason.restored",
};

export function ListHistoryPanel({ listID }: Readonly<{ listID: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const history = useListHistory(listID);
  const rows = history.data?.data ?? [];
  return (
    <Panel title={t("lists.history.title")}>
      <PanelBody>
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
                render: (row) =>
                  formatDateTime(row.occurred_at, locale, viewerZone()),
              },
              {
                key: "what",
                header: t("lists.history.what"),
                grow: true,
                render: (row) => (
                  <span>
                    {t(KIND_LABEL[row.kind])}
                    {row.reason && ` · ${t(REASON_LABEL[row.reason])}`}
                    {row.note && (
                      <span className="t-caption"> — {row.note}</span>
                    )}
                  </span>
                ),
              },
              {
                key: "who",
                header: t("lists.history.who"),
                render: (row) => row.actor_name ?? t("lists.history.someone"),
              },
            ]}
          />
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
