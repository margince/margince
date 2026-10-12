// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { useT } from "../i18n";
import type { ImportFormatColumn } from "./importtypes";

// What goes in one column of an import file, opened where the column is named:
// a chip in the template preview, or the `?` beside a chosen field on the map.
// The words come from the server's format, the same ones the template's README
// and the agent read.

/**
 * The guide behind a trigger that names the column. `column.name` is the key a
 * mapping sends (`cf_tier`); the trigger shows `column.label` (`Tier`) where
 * the column has one, because that is the header a person types.
 */
export function ColumnGuide({
  column,
  file,
  label,
}: Readonly<{
  column: ImportFormatColumn;
  file: string;
  label?: ReactNode;
}>) {
  const t = useT();
  const where = [column.name, file];
  if (column.custom) where.push(t("import.guide.custom"));
  return (
    <Popover
      onHover
      label={label ?? column.label ?? column.name}
      className="import-guide__trigger"
      panelClassName="import-guide"
    >
      <p>{column.description}</p>
      <p className="t-caption">{where.join(" · ")}</p>
      {column.required_on_new_row ? (
        <Badge tone="success">{t("import.guide.required")}</Badge>
      ) : null}
      {column.example ? (
        <p className="t-caption">
          {t("import.guide.example", { example: column.example })}
        </p>
      ) : null}
    </Popover>
  );
}

/** The same guide behind a `?`, beside a field already chosen for a column. */
export function ColumnGuideButton({
  column,
  file,
}: Readonly<{ column: ImportFormatColumn; file: string }>) {
  const t = useT();
  return (
    <ColumnGuide
      column={column}
      file={file}
      label={
        <>
          <span aria-hidden>?</span>
          <span className="sr-only">
            {t("import.guide.whatGoesIn", {
              column: column.label ?? column.name,
            })}
          </span>
        </>
      }
    />
  );
}
