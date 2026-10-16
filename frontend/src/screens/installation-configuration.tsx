// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, EmptyState, SearchField } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { lockFor } from "../format/settinglocks";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";

type Entry = components["schemas"]["ConfigurationEntry"];
type Source = Entry["origin"]["source"];

// The sources that are not boot sources. A boot source is worded by `lockFor`,
// so the lock sentence and this column cannot tell two stories.
const UNLOCKED_SOURCE: Partial<Record<Source, MessageKey>> = {
  database: "configuration.source.database",
  default: "configuration.source.default",
  unset: "configuration.source.unset",
};

function useConfiguration() {
  return useQuery({
    queryKey: ["installation-configuration"],
    queryFn: async () => unwrap(await api.GET("/installation/configuration")),
  });
}

function sourceWords(entry: Entry, t: Translator): string {
  const lock = lockFor([entry], entry.key, t);
  if (lock.disabled) {
    return lock.reason;
  }
  const label = UNLOCKED_SOURCE[entry.origin.source];
  return label ? t(label) : entry.origin.source;
}

function ValueCell({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  if (entry.secret) {
    return <Badge>{t("configuration.secret")}</Badge>;
  }
  if (entry.value === null) {
    return <span className="t-caption">{t("configuration.unset")}</span>;
  }
  if (entry.value === "") {
    return <span className="t-caption">{t("configuration.emptyValue")}</span>;
  }
  return <span>{entry.value}</span>;
}

function EntryDetail({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  const facts = [
    entry.secret
      ? null
      : t("configuration.detail.default", {
          value: entry.default ?? t("configuration.detail.none"),
        }),
    t("configuration.detail.type", { type: entry.type }),
    entry.flag
      ? t("configuration.detail.flag", { flag: `--${entry.flag}` })
      : null,
    t("configuration.detail.readBy", { programs: entry.roles.join(", ") }),
  ].filter((fact) => fact !== null);
  return (
    <CellStack>
      <span>{entry.doc}</span>
      <span className="t-caption">{facts.join(" · ")}</span>
    </CellStack>
  );
}

function columns(t: Translator): DataTableColumn<Entry>[] {
  return [
    {
      key: "key",
      header: t("configuration.col.key"),
      grow: true,
      fold: "title",
      render: (entry) => entry.key,
    },
    {
      key: "value",
      header: t("configuration.col.value"),
      render: (entry) => <ValueCell entry={entry} />,
    },
    {
      key: "source",
      header: t("configuration.col.source"),
      render: (entry) => sourceWords(entry, t),
    },
    {
      key: "env",
      header: t("configuration.col.env"),
      render: (entry) => entry.env,
    },
  ];
}

function matching(entries: readonly Entry[], filter: string): Entry[] {
  const needle = filter.trim().toLowerCase();
  if (needle === "") {
    return [...entries];
  }
  return entries.filter((entry) =>
    [entry.key, entry.env].some((text) => text.toLowerCase().includes(needle)),
  );
}

export function InstallationConfigurationPage() {
  const t = useT();
  const [filter, setFilter] = useState("");
  const query = useConfiguration();

  return (
    <Panel title={t("configuration.title")}>
      <PanelBody>
        <PanelIntro>{t("configuration.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("configuration.title")}>
        {(entries) => {
          const shown = matching(entries, filter);
          return (
            <>
              <PanelBody>
                <SearchField
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                  placeholder={t("configuration.search")}
                  aria-label={t("configuration.search")}
                />
              </PanelBody>
              {shown.length > 0 ? (
                <DataTable
                  bleed
                  fold
                  label={t("configuration.title")}
                  columns={columns(t)}
                  rows={shown}
                  rowKey={(entry) => entry.key}
                  detail={{
                    header: t("configuration.col.detail"),
                    toggleLabel: (entry) =>
                      t("configuration.toggle", { key: entry.key }),
                    render: (entry) => <EntryDetail entry={entry} />,
                  }}
                />
              ) : (
                <PanelBody>
                  <EmptyState>{t("configuration.noMatch")}</EmptyState>
                </PanelBody>
              )}
            </>
          );
        }}
      </QueryGate>
    </Panel>
  );
}
