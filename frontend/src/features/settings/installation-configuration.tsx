// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Badge, EmptyState, SearchField } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { type FilterPill, FilterPills } from "../../design-system/filterpills";
import { IconAction } from "../../design-system/iconaction";
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

const CONFIGURATION_SCOPE_PARAM = "source";

const SCOPES = ["all", "changed", "outside", "app", "default"] as const;

type Scope = (typeof SCOPES)[number];

// A setting the api does not read has a source the api cannot see, so only
// "all" holds it.
const SCOPE_SOURCES: Record<Scope, readonly Source[] | null> = {
  all: null,
  changed: ["flag", "env", "env_file", "config_file", "database"],
  outside: ["flag", "env", "env_file", "config_file"],
  app: ["database"],
  default: ["default"],
};

const SCOPE_LABEL: Record<Scope, MessageKey> = {
  all: "configuration.scope.all",
  changed: "configuration.scope.changed",
  outside: "configuration.scope.outside",
  app: "configuration.source.database",
  default: "configuration.source.default",
};

function scopeOf(dial: string | undefined): Scope {
  return SCOPES.find((scope) => scope === dial) ?? "all";
}

function inScope(entry: Entry, scope: Scope): boolean {
  const sources = SCOPE_SOURCES[scope];
  return sources === null || sources.includes(entry.origin.source);
}

function scopePills(
  t: Translator,
  entries: readonly Entry[],
): FilterPill<Scope>[] {
  return SCOPES.map((scope) => ({
    value: scope,
    label: t(SCOPE_LABEL[scope]),
    count: entries.filter((entry) => inScope(entry, scope)).length,
  }));
}

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

type Copied = "value" | "default";

// The clipboard holds one text, so the detail names which of its two copy
// controls wrote it last.
function FullText({
  label,
  copyLabel,
  text,
  current,
  onCopied,
}: Readonly<{
  label: string;
  copyLabel: string;
  text: string;
  current: boolean;
  onCopied: () => void;
}>) {
  const t = useT();
  const clip = useClipboardCopy(
    text,
    {
      copy: copyLabel,
      copied: t("configuration.copied"),
      remedy: t("configuration.copyRemedy"),
    },
    onCopied,
  );
  const shown = current && clip.copied;
  return (
    <CellStack>
      <span>
        {label}{" "}
        <IconAction
          inline
          label={shown ? t("configuration.copied") : copyLabel}
          icon={shown ? <Check aria-hidden /> : <Copy aria-hidden />}
          onClick={clip.copy}
        />
      </span>
      <KeyedName name={text} code={text} />
      {clip.notice}
    </CellStack>
  );
}

function EntryDetail({ entry }: Readonly<{ entry: Entry }>) {
  const t = useT();
  const [copied, setCopied] = useState<Copied | null>(null);
  const value = entry.secret ? null : entry.value;
  const fallback = entry.secret ? null : entry.default;
  const longValue = value !== null && preview(value) !== value;
  const longDefault = fallback !== null && preview(fallback) !== fallback;
  const facts = [
    entry.secret || longDefault
      ? null
      : t("configuration.detail.default", {
          value: fallback ?? t("configuration.detail.none"),
        }),
    t("configuration.detail.type", { type: entry.type }),
    entry.flag
      ? t("configuration.detail.flag", { flag: `--${entry.flag}` })
      : null,
    t("configuration.detail.readBy", { programs: entry.roles.join(", ") }),
  ].filter((fact) => fact !== null);
  return (
    <CellStack>
      {longValue && (
        <FullText
          label={t("configuration.detail.value")}
          copyLabel={t("configuration.copyValue")}
          text={value}
          current={copied === "value"}
          onCopied={() => setCopied("value")}
        />
      )}
      {longDefault && (
        <FullText
          label={t("configuration.detail.fullDefault")}
          copyLabel={t("configuration.copyDefault")}
          text={fallback}
          current={copied === "default"}
          onCopied={() => setCopied("default")}
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
  const scope = scopeOf(params.get(CONFIGURATION_SCOPE_PARAM));
  const query = useConfiguration();

  return (
    <Panel title={t("configuration.title")}>
      <PanelBody>
        <PanelIntro>{t("configuration.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("configuration.title")}>
        {(entries) => {
          const matched = entries.filter((entry) =>
            matchesConfigurationFilter(entry, filter),
          );
          const shown = matched.filter((entry) => inScope(entry, scope));
          return (
            <>
              <PanelBody>
                <div className="filter-tabs">
                  <FilterPills
                    pills={scopePills(t, matched)}
                    value={scope}
                    onChange={(next) =>
                      replaceDial(
                        CONFIGURATION_SCOPE_PARAM,
                        next === "all" ? undefined : next,
                      )
                    }
                    label={t("configuration.scope.label")}
                  />
                </div>
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
