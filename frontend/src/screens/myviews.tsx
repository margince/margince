// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// My views: the reader's own saved filters, private to them, grouped by the
// record type each filters. Opening one loads it into the builder.

import { navigate } from "../app/router";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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

export function MyViews() {
  const t = useT();
  return (
    <Panel title={t("lists.section.views")}>
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
