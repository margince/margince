// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Badge, Button, TableScroll } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { RecordTabs } from "../../design-system/recordtabs";
import { Select } from "../../design-system/select";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { ColumnGuideButton } from "./import.guide";
import { ImportRefusal } from "./import.notices";
import type { WizardFile } from "./importflow";
import type {
  ImportColumn,
  ImportFormat,
  ImportFormatColumn,
  ImportObject,
} from "./importtypes";
import { DONT_IMPORT } from "./importtypes";
import type { ImportWizardState } from "./importwizard";

// The mapping surface: one row per column of the uploaded file. Each row shows
// the name the file gives it, how often it is filled in, and a few values.
//
// The fill rate tells a mapping mistake from a real column. A column at 4% is
// usually a mistake, and it looks like a column at 98% from its name alone.
//
// The destination defaults to the server's suggestion. It matches normalized
// names only, so an unmatched column arrives as "don't import", not a guess.

export function ImportMappingTable({
  columns,
  rowsProfiled,
  targets,
  file,
  mapping,
  locked,
  onChange,
}: Readonly<{
  columns: readonly ImportColumn[];
  rowsProfiled: number;
  // An option's value is the key the mapping sends (`cf_tier`); its text is
  // the label the screen shows (`Tier`), or the core field's own name in the
  // user's language (`Employees` for `size_band`).
  targets: readonly ImportFormatColumn[];
  /** The file the targets belong to, named in a chosen field's guide. */
  file: string;
  mapping: Record<string, string>;
  // While the mapping is being validated it stops accepting changes: a choice
  // made now cannot reach the request already in flight, and a table showing a
  // destination the run does not carry is the screen lying about the import.
  locked: boolean;
  onChange: (column: string, target: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const options = [
    { value: DONT_IMPORT, label: t("import.dontImport") },
    ...targets.map((target) => ({
      value: target.name,
      label: target.label ?? target.field_label ?? target.name,
    })),
  ];

  return (
    <div className="import__mapping">
      <p className="import__hint t-sub">
        {t("import.profiled", { rows: formatNumber(rowsProfiled, locale) })}
      </p>
      <TableScroll label={t("import.mappingTable")}>
        <table className="import__table">
          <colgroup>
            <col className="import__colWidthName" />
            <col className="import__colWidthFill" />
            <col className="import__colWidthSamples" />
            <col className="import__colWidthField" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">{t("import.col.column")}</th>
              <th scope="col" title={t("import.col.filledHint")}>
                {t("import.col.filled")}
              </th>
              <th scope="col">{t("import.col.samples")}</th>
              <th scope="col">{t("import.col.destination")}</th>
            </tr>
          </thead>
          <tbody>
            {columns.map((column) => {
              const chosen = mapping[column.header] ?? DONT_IMPORT;
              const target = targets.find((one) => one.name === chosen);
              return (
                <tr key={column.header}>
                  <th scope="row" className="import__colName">
                    {column.header}
                  </th>
                  <td className="import__fill">{fillLabel(column)}</td>
                  <td
                    className="import__samples"
                    title={column.samples.join(", ") || undefined}
                  >
                    {column.samples.length > 0 ? (
                      column.samples.join(", ")
                    ) : (
                      <span className="import__empty">
                        {t("import.noSamples")}
                      </span>
                    )}
                  </td>
                  <td>
                    <div className="import__target">
                      <Select
                        className="import__select"
                        options={options}
                        value={chosen}
                        onChange={(next) => onChange(column.header, next)}
                        disabled={locked}
                        placeholder={t("import.dontImport")}
                        aria-label={t("import.destinationFor", {
                          column: column.header,
                        })}
                      />
                      {target ? (
                        <TargetMarks target={target} file={file} />
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </TableScroll>
      <p className="import__hint t-caption">{t("import.col.filledHint")}</p>
    </div>
  );
}

// What a chosen field is beyond its name. It may be a link to another record
// or one of this installation's own fields, and it has a guide to its content.
function TargetMarks({
  target,
  file,
}: Readonly<{ target: ImportFormatColumn; file: string }>) {
  const t = useT();
  return (
    <span className="import__marks">
      {isLink(target) ? (
        <Badge tone="info">{t("import.map.link")}</Badge>
      ) : null}
      {target.custom ? (
        <Badge tone="accent">{t("import.guide.custom")}</Badge>
      ) : null}
      <ColumnGuideButton column={target} file={file} />
    </span>
  );
}

// A target that names another record by its ref. The file's own `ref` is its
// key, not a link.
function isLink(target: ImportFormatColumn): boolean {
  return target.name !== "ref" && target.name.endsWith("_ref");
}

// fillLabel renders the rate as a percentage of the rows actually profiled.
// Rounded to whole numbers: the decision it informs is "is this column worth
// mapping", and nobody decides that differently at 4.2% than at 4%.
function fillLabel(column: ImportColumn): string {
  return `${Math.round(column.fill_rate * 100)}%`;
}

type TypedFile = Readonly<{ file: WizardFile; object: ImportObject }>;

// The typed files in the order the engine lands them, read from the format.
function typedInOrder(
  wizard: ImportWizardState,
  order: ImportFormat["order"],
): TypedFile[] {
  return order.flatMap((object) =>
    wizard.files
      .filter((file) => wizard.objects[file.file] === object)
      .map((file) => ({ file, object })),
  );
}

function unmapped(file: WizardFile, mapping: Record<string, string>): number {
  return file.columns.filter(
    (column) => (mapping[column.header] ?? DONT_IMPORT) === DONT_IMPORT,
  ).length;
}

/**
 * One tab per typed file, in import order, each with the count of columns it
 * leaves out. The targets are the format's columns for the file's type, so a
 * custom field is offered by its label and sent by its key.
 */
export function MapStep({
  wizard,
  format,
}: Readonly<{ wizard: ImportWizardState; format: ImportFormat }>) {
  const t = useT();
  const typed = typedInOrder(wizard, format.order);
  const [open, setOpen] = useState(typed[0]?.file.file ?? "");
  const current = typed.find((entry) => entry.file.file === open) ?? typed[0];
  if (!current) return null;
  const name = current.file.file;
  const mapping = wizard.mapping[name] ?? {};
  const formatColumns =
    format.files.find((file) => file.object === current.object)?.columns ?? [];
  // A contact's page fills the links, so the column is no longer required.
  const targets = wizard.presetContact
    ? formatColumns.map((column) =>
        column.name === "links"
          ? { ...column, required_on_new_row: false }
          : column,
      )
    : formatColumns;
  const test = wizard.test;
  const refused = test.error ?? wizard.saveFailure;
  // A test run that stopped left the run a draft, with the reason on it.
  const stopped =
    wizard.run.data?.status === "pending" ? wizard.run.data.error : undefined;
  return (
    <div className="import">
      <RecordTabs
        label={t("import.map.tabs")}
        options={typed.map((entry) => entry.file.file)}
        value={name}
        onChange={setOpen}
        labels={Object.fromEntries(
          typed.map((entry) => [
            entry.file.file,
            t(`import.object.${entry.object}`),
          ]),
        )}
        counts={Object.fromEntries(
          typed.flatMap((entry) => {
            const left = unmapped(
              entry.file,
              wizard.mapping[entry.file.file] ?? {},
            );
            return left > 0 ? [[entry.file.file, left]] : [];
          }),
        )}
      />
      <p className="import__hint t-sub">
        {t("import.map.lead", {
          file: name,
          type: t(`import.object.${current.object}`),
        })}
      </p>
      <ImportMappingTable
        key={name}
        columns={current.file.columns}
        rowsProfiled={current.file.rowsProfiled}
        targets={targets}
        file={name}
        mapping={mapping}
        locked={test.isPending}
        onChange={(column, target) =>
          wizard.setColumnTarget(name, column, target)
        }
      />
      <div className="import__actions">
        <Button variant="ghost" onClick={() => wizard.goTo("files")}>
          {t("import.wizard.back")}
        </Button>
        <Button
          variant="primary"
          pending={test.isPending}
          busyLabel={t("import.map.running")}
          onClick={wizard.runTest}
        >
          {t("import.map.run")}
        </Button>
      </div>
      {refused ? (
        <ImportRefusal
          title={t("import.map.stageFailed")}
          error={refused}
          keepsDraft
        />
      ) : stopped ? (
        <Callout
          tone="danger"
          kind="standing"
          title={t("import.map.testStopped")}
        >
          {stopped}
        </Callout>
      ) : null}
    </div>
  );
}
