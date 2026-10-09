// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed on a list, newest first: every change of what the list is,
// every Shortlist membership change, and every record a Live List's checks saw
// joining or leaving, about a record this reader can see now.

import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type List,
  type ListHistoryEntry,
  useListHistory,
} from "./lists.queries";

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

export function ListHistoryPanel({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const { locale } = useLocale();
  const history = useListHistory(list.id);
  const live = list.list_type === "dynamic";
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
        {live && <ListCheckIntro check={list.last_check} />}
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
                  <CellStack>
                    <span>
                      {t(KIND_LABEL[row.kind])}
                      {reasonOf(row)}
                    </span>
                    {row.note && <span className="t-caption">{row.note}</span>}
                  </CellStack>
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

/**
 * When a Live List was last checked for who joined and left. It sits with the
 * history because members are worked out again on every open: the check dates
 * the joins and leaves below, not the members above. The caveat rides at the
 * end of the sentence, because a panel head never wraps and would cut its own
 * title to fit a trigger beside it; link ink marks it pressable within prose.
 */
function ListCheckIntro({ check }: Readonly<{ check: List["last_check"] }>) {
  const t = useT();
  const { locale } = useLocale();
  let sentence = t("lists.history.notChecked");
  if (check) {
    const when = formatDateTime(check.checked_at, locale, viewerZone());
    sentence =
      check.outcome === "too_large"
        ? t("lists.history.tooLarge", { when })
        : t("lists.history.lastChecked", { when });
  }
  return (
    <PanelIntro>
      {sentence}{" "}
      <Popover variant="link" label={t("lists.history.howChecks")}>
        <p>{t("lists.history.liveNote")}</p>
      </Popover>
    </PanelIntro>
  );
}
