// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// My views: everything private to the reader. Their private Live Lists and
// Shortlists first, each opening its page, then their saved filters grouped
// by the record type each filters, each opening in the builder.

import { navigate } from "../app/router";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ListTable, NewShortlistAction } from "./listlibrary";
import { PRIVATE_LISTS, useLists } from "./lists.queries";
import { filterTreeOf, useSavedViews, type ViewResource } from "./savedviews";

const VIEW_GROUPS: readonly Readonly<{
  resource: ViewResource;
  title: MessageKey;
}>[] = [
  { resource: "contacts", title: "filters.tab.contacts" },
  { resource: "companies", title: "filters.tab.companies" },
  { resource: "deals", title: "filters.tab.deals" },
  { resource: "leads", title: "filters.tab.leads" },
];

/** What a Shortlist made from My views starts as: the reader's alone. */
const ONLY_ME = { sharing: "private", teamId: null } as const;

export function MyViews() {
  const t = useT();
  return (
    <Panel
      title={t("lists.section.views")}
      actions={<NewShortlistAction defaultAudience={ONLY_ME} />}
    >
      <MyLists />
      {VIEW_GROUPS.map((group) => (
        <ViewGroup
          key={group.resource}
          resource={group.resource}
          title={t(group.title)}
        />
      ))}
    </Panel>
  );
}

/** The reader's private lists. Shared, a list moves to Shared views. */
function MyLists() {
  const t = useT();
  const lists = useLists({ sharing: PRIVATE_LISTS });
  const rows = lists.data?.data ?? [];
  return (
    <PanelBody>
      <SurfaceState
        label={t("lists.myLists.title")}
        state={
          lists.isPending
            ? "loading"
            : lists.isError
              ? "unavailable"
              : rows.length > 0
                ? "ready"
                : "empty"
        }
        emptyLabel={t("lists.myLists.empty")}
        loadingLabel={t("lists.library.loading")}
        loadingLines={2}
      >
        <ListTable label={t("lists.myLists.title")} rows={rows} />
      </SurfaceState>
    </PanelBody>
  );
}

function ViewGroup({
  resource,
  title,
}: Readonly<{ resource: ViewResource; title: string }>) {
  const t = useT();
  const views = useSavedViews(resource);
  const rows = (views.data ?? []).filter((view) => filterTreeOf(view) !== null);
  return (
    <PanelBody>
      <SurfaceState
        label={title}
        state={
          views.isPending
            ? "loading"
            : views.isError
              ? "unavailable"
              : rows.length > 0
                ? "ready"
                : "empty"
        }
        emptyLabel={t("lists.views.empty")}
        loadingLabel={t("lists.views.loading")}
        loadingLines={2}
      >
        <DataTable
          label={title}
          rows={rows}
          rowKey={(view) => view.id}
          // The builder reads the object and the view off the address, so the
          // view opens already loaded and Back returns here.
          onRowClick={(view) =>
            navigate({ screen: "filters", id: resource, id2: view.id })
          }
          columns={[
            {
              key: "name",
              header: t("lists.col.name"),
              grow: true,
              render: (view) => view.name,
            },
          ]}
        />
      </SurfaceState>
    </PanelBody>
  );
}
