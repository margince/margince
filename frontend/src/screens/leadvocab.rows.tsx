// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, OverflowMenu } from "../design-system/atoms";
import type { DataTableColumn } from "../design-system/datatable";
import { formatNumber } from "../format/format";
import { type PluralTranslator, type Translator, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import "./leadvocab.css";

// Folded, the column heading is out of sight, so the count says its unit.
export function leadsColumn<Row>(
  {
    t,
    plural,
    locale,
  }: Readonly<{ t: Translator; plural: PluralTranslator; locale: Locale }>,
  count: (row: Row) => number,
): DataTableColumn<Row> {
  return {
    key: "leads",
    header: t("leadSources.colLeads"),
    align: "end",
    render: (row) => formatNumber(count(row), locale),
    folded: (row) =>
      plural("leadSources.leads", count(row), {
        count: formatNumber(count(row), locale),
      }),
  };
}

// Rename and Remove for one lead source or reason. A refused remove stays in
// the menu with its reason in words, as the server would answer it with a 409.
export function VocabRowMenu({
  label,
  canEdit,
  canRemove,
  refusal,
  onRename,
  onRemove,
}: Readonly<{
  label: string;
  canEdit: boolean;
  canRemove: boolean;
  refusal?: string;
  onRename: () => void;
  onRemove: () => void;
}>) {
  const t = useT();
  if (!canEdit && !canRemove) return null;
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: label })}>
        {canEdit && (
          <Button onClick={onRename}>{t("leadSources.rename")}</Button>
        )}
        {canRemove && (
          <Button variant="danger" reason={refusal} onClick={onRemove}>
            {t("leadSources.remove")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}
