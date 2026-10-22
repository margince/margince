// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, EmptyState, SearchField } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyedName } from "../../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import {
  CONFIGURATION_FILTER_PARAM,
  type ConfigurationEntry as Entry,
  matchesConfigurationFilter,
  useConfiguration,
} from "./configurationentries";

type Source = Entry["origin"]["source"];

const SOURCE_LABEL: Record<Source, MessageKey> = {
  flag: "configuration.source.flag",
  env: "configuration.source.env",
  env_file: "configuration.source.env_file",
  config_file: "configuration.source.config_file",
  database: "configuration.source.database",
  default: "configuration.source.default",
  unset: "configuration.source.unset",
};

function SourceCell({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  const { source, name, file } = entry.origin;
  const label = t(SOURCE_LABEL[source]);
  if (!name) {
    return label;
  }
  return <KeyedName name={file ? `${label} (${file})` : label} code={name} />;
}

// A struct or list value can run to hundreds of characters. The cell shows its
// start, and the row's detail shows it whole.
const VALUE_PREVIEW_CHARS = 40;

function preview(value: string): string {
  return value.length > VALUE_PREVIEW_CHARS
    ? `${value.slice(0, VALUE_PREVIEW_CHARS)}…`
    : value;
}

function ValueCell({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  if (entry.secret) {
    return <Badge>{t("configuration.secret")}</Badge>;
  }
  if (entry.value === null) {
    // `unset` means the api does not read it; any other source with no value is
    // a setting that has neither a value nor a default.
    const unread = entry.origin.source === "unset";
    return (
      <span className="t-caption">
        {t(unread ? "configuration.unset" : "configuration.noValue")}
      </span>
    );
  }
  if (entry.value === "") {
    return <span className="t-caption">{t("configuration.emptyValue")}</span>;
  }
  const shown = preview(entry.value);
  return <KeyedName name={shown} code={shown} />;
}

function EntryDetail({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  const facts = [
    entry.secret
      ? null
      : t("configuration.detail.default", {
          value:
            entry.default === null
              ? t("configuration.detail.none")
              : preview(entry.default),
        }),
    t("configuration.detail.type", { type: entry.type }),
    entry.flag
      ? t("configuration.detail.flag", { flag: `--${entry.flag}` })
      : null,
    t("configuration.detail.readBy", { programs: entry.roles.join(", ") }),
  ].filter((fact) => fact !== null);
  return (
    <CellStack>
      {!entry.secret &&
        entry.value !== null &&
        preview(entry.value) !== entry.value && (
          <KeyedName
            name={t("configuration.detail.value")}
            code={entry.value}
          />
        )}
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
      fold: "title",
      // The variable sits under its key rather than in a column of its own,
      // so the table keeps to three columns inside its panel.
      render: (entry) => <KeyedName name={entry.key} code={entry.env} />,
    },
    {
      key: "value",
      header: t("configuration.col.value"),
      render: (entry) => <ValueCell entry={entry} />,
    },
    {
      key: "source",
      header: t("configuration.col.source"),
      render: (entry) => <SourceCell entry={entry} />,
    },
  ];
}

export function InstallationConfigurationPage() {
  const t = useT();
  // In the address, so settings search can open the page on the setting it found.
  const [params] = useUrlParams();
  const filter = params.get(CONFIGURATION_FILTER_PARAM) ?? "";
  const query = useConfiguration();

  return (
    <Panel title={t("configuration.title")}>
      <PanelBody>
        <PanelIntro>{t("configuration.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("configuration.title")}>
        {(entries) => {
          const shown = entries.filter((entry) =>
            matchesConfigurationFilter(entry, filter),
          );
          return (
            <>
              <PanelBody>
                <SearchField
                  value={filter}
                  onChange={(event) =>
                    replaceDial(CONFIGURATION_FILTER_PARAM, event.target.value)
                  }
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
