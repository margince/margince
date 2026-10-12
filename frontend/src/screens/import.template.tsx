// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Disclosure } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import { QueryStates } from "./common";
import { ColumnGuide } from "./import.guide";
import { useImportFormat } from "./importflow";
import type { ImportFormat, ImportFormatColumn } from "./importtypes";

/** Where the server builds the template ZIP for this installation's fields. */
export const IMPORT_TEMPLATE_HREF = "/v1/imports/template";

type FormatFile = ImportFormat["files"][number];

/**
 * The template before it is downloaded: every file in the order an import
 * lands them, every column with its guide, and the rules for keys. The ZIP
 * itself is built on the server from the same format, so what is shown here is
 * what arrives.
 */
export function TemplatePreview() {
  const t = useT();
  const { locale } = useLocale();
  const format = useImportFormat(locale);
  return (
    <Panel
      title={t("import.template.title")}
      titleAction={
        <a className="link-button" href={IMPORT_TEMPLATE_HREF} download>
          {t("import.template.download")}
        </a>
      }
    >
      <PanelBody>
        <QueryStates query={format} pendingLabel={t("common.loading")}>
          {format.data ? <TemplateBody format={format.data} /> : null}
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

function TemplateBody({ format }: Readonly<{ format: ImportFormat }>) {
  const t = useT();
  return (
    <div className="import">
      <p className="import__hint t-sub">{t("import.template.lead")}</p>
      <DataTable<FormatFile>
        label={t("import.template.files")}
        rows={format.files}
        rowKey={(file) => file.file}
        columns={[
          {
            key: "file",
            header: t("import.template.col.file"),
            render: (file) => <code>{file.file}</code>,
          },
          {
            key: "columns",
            header: t("import.template.col.columns"),
            grow: true,
            render: (file) => (
              <div className="import-template__chips">
                {file.columns.map((column) => (
                  <ColumnGuide
                    key={column.name}
                    column={column}
                    file={file.file}
                    label={<ColumnChip column={column} />}
                  />
                ))}
              </div>
            ),
          },
        ]}
      />
      <p className="import__hint t-caption">{t("import.template.legend")}</p>
      <Disclosure summary={t("import.template.rules")}>
        <ul className="import__issues t-sub">
          {format.key_rules.map((rule) => (
            <li key={rule}>{rule}</li>
          ))}
        </ul>
      </Disclosure>
    </div>
  );
}

// A custom column wears the accent, so a reader sees which headers come from
// this installation's own fields. The dot marks a column a new row needs; the
// guide says the same in words, so the dot is hidden from a screen reader.
function ColumnChip({ column }: Readonly<{ column: ImportFormatColumn }>) {
  return (
    <Badge tone={column.custom ? "accent" : "default"}>
      {column.label ?? column.name}
      {column.required_on_new_row ? (
        <span aria-hidden className="import-template__required">
          {" ●"}
        </span>
      ) : null}
    </Badge>
  );
}
