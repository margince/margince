// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed on a list, newest first: every change of what the list is,
// every Shortlist membership change, and every record a Live List's checks saw
// joining or leaving, about a record this reader can see now.

import { type ReactNode, useMemo } from "react";
import { Button } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { provenanceLabel } from "../../design-system/provenance";
import { provenanceOf } from "../../design-system/provenanceof";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { recordRoute } from "../../lib/entity";
import { useFilterVocabulary } from "../../lib/filterdata";
import { labelFor } from "../../lib/labelfor";
import {
  type List,
  type ListHistoryEntry,
  useListHistory,
} from "../../lib/lists.queries";
import { navigate } from "../../lib/router";
import { decode } from "../../lib/segmentpredicate";
import { useViewerZone } from "../../lib/viewerzone";
import {
  filterSentence,
  referencedIdsOf,
  useSentenceWords,
} from "./filtersentence";

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

/** What each part a revision changed is called; the filter gets its own lines. */
const CHANGE_LABEL: Record<
  Exclude<NonNullable<ListHistoryEntry["changes"]>[number], "filter">,
  MessageKey
> = {
  name: "lists.history.renamed",
  purpose: "lists.history.repurposed",
  sharing: "lists.history.reshared",
  steward: "lists.history.handedOver",
};

/** A change a Live List's check saw, rather than one somebody made. */
function isObserved(row: ListHistoryEntry): boolean {
  return row.kind === "member_entered" || row.kind === "member_left";
}

/**
 * Who made a change. A member is named; an agent, a connector or a system task
 * is called what it is, never "Someone", which says a member did it.
 */
function useWhoActed(): (row: ListHistoryEntry) => ReactNode {
  const t = useT();
  return (row) => {
    if (isObserved(row)) {
      return t("lists.history.checker");
    }
    if (row.actor_name) {
      return row.actor_name;
    }
    const provenance = provenanceOf(row.actor);
    if (provenance.kind === "human") {
      return t("lists.history.someone");
    }
    return provenanceLabel(provenance, t, undefined);
  };
}

export function ListHistoryPanel({ list }: Readonly<{ list: List }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const history = useListHistory(list.id);
  const live = list.list_type === "dynamic";
  const rows = history.data?.data ?? [];
  const when = (row: ListHistoryEntry) =>
    formatDateTime(row.occurred_at, locale, readerZone);
  const reasonOf = (row: ListHistoryEntry) => {
    const key = row.reason ? REASON_LABEL[row.reason] : null;
    return key ? ` · ${t(key)}` : "";
  };
  const whoActed = useWhoActed();
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
                render: (row) =>
                  row.kind === "revised" ? (
                    <RevisionChanges row={row} list={list} />
                  ) : (
                    <CellStack>
                      <span>
                        {t(labelFor(KIND_LABEL, row.kind))}
                        {reasonOf(row)}
                      </span>
                      <MemberName row={row} />
                      {row.note && (
                        <span className="t-caption">{row.note}</span>
                      )}
                    </CellStack>
                  ),
              },
              {
                key: "who",
                header: t("lists.history.who"),
                render: whoActed,
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
 * history because members are worked out again on every open. The check dates
 * the joins and leaves below, not the members above. The caveat rides at the
 * end of the sentence. A panel head never wraps, so a trigger beside its title
 * would cut the title short. Link ink marks it pressable within prose.
 */
function ListCheckIntro({ check }: Readonly<{ check: List["last_check"] }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  let sentence = t("lists.history.notChecked");
  if (check) {
    const when = formatDateTime(check.checked_at, locale, readerZone);
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

/** The record that joined or left, opening it where its kind has a page. */
function MemberName({ row }: Readonly<{ row: ListHistoryEntry }>) {
  const t = useT();
  const name = row.entity_name ?? t("lists.unnamed");
  const route = recordRoute(row.entity_type, row.entity_id);
  if (!route) {
    return <span>{name}</span>;
  }
  return (
    <span>
      <Button variant="link" onClick={() => navigate(route)}>
        {name}
      </Button>
    </span>
  );
}

/**
 * What one revision changed, a line for each part. A filter reads before and
 * after as sentences. A revision with no earlier version kept says only that
 * the list changed, unless it is the first, which made the list.
 */
function RevisionChanges({
  row,
  list,
}: Readonly<{ row: ListHistoryEntry; list: List }>) {
  const t = useT();
  const changes = row.changes;
  if (!changes || changes.length === 0) {
    const made = !changes && row.version === 1;
    return (
      <span>{t(made ? "lists.history.created" : "lists.history.revised")}</span>
    );
  }
  return (
    <CellStack>
      {changes.map((change) =>
        change === "filter" ? (
          <FilterChange key={change} row={row} list={list} />
        ) : (
          <span key={change}>
            {t(labelFor(CHANGE_LABEL, change), {
              before: row.previous_name ?? "",
              after: row.name ?? "",
            })}
          </span>
        ),
      )}
    </CellStack>
  );
}

/** A filter change, with the filter before and after it in words. */
function FilterChange({
  row,
  list,
}: Readonly<{ row: ListHistoryEntry; list: List }>) {
  const t = useT();
  const vocabulary = useFilterVocabulary(list.entity_type, true);
  const fields = vocabulary.data?.fields;
  // Decoding mints a fresh id for every node, so it runs once per definition.
  const before = useMemo(
    () => decode(row.previous_definition),
    [row.previous_definition],
  );
  const after = useMemo(() => decode(row.definition), [row.definition]);
  const words = useSentenceWords(
    referencedIdsOf(
      [before, after].filter((tree) => tree !== null),
      fields,
    ),
  );
  const said = (tree: typeof before) =>
    (tree && filterSentence(tree, fields, words)) ||
    t("lists.history.noFilter");
  return (
    <>
      <span>{t("lists.history.refiltered")}</span>
      <span className="t-caption">
        {t("lists.history.filterBefore", { sentence: said(before) })}
      </span>
      <span className="t-caption">
        {t("lists.history.filterAfter", { sentence: said(after) })}
      </span>
    </>
  );
}
