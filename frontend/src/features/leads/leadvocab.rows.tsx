// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useRef } from "react";
import { Button, OverflowMenu } from "../../design-system/atoms";
import type { DataTableColumn } from "../../design-system/datatable";
import { formatNumber } from "../../format/format";
import { type PluralTranslator, type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Locale } from "../../i18n/locale";
import { useCanWrite } from "../../lib/capability";
import { usePageReadOnly } from "../settings/settingsnav";
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

type VocabVerbs = Readonly<{ canEdit: boolean; canRemove: boolean }>;
type VocabPage = "leads" | "acquisition" | "recordroles";

// A page the seat only consults carries its own notice, so a card states a
// partial posture only where the page leaves it unsaid.
export function useVocabGrants(page: VocabPage) {
  const readOnlyPage = usePageReadOnly(page);
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const refusal =
    readOnlyPage || canEdit
      ? undefined
      : vocabRefusal({ canCreate, canRemove });
  return { canCreate, canEdit, canRemove, refusal };
}

function vocabRefusal({
  canCreate,
  canRemove,
}: Readonly<{
  canCreate: boolean;
  canRemove: boolean;
}>): MessageKey | undefined {
  if (canCreate) return "leadSources.addOnlyTitle";
  return canRemove ? "leadSources.removeOnlyTitle" : undefined;
}

type VocabRow = Readonly<{
  label: string;
  refusal?: string;
  onRename: () => void;
  onRemove?: () => void;
}>;

// No column for a seat with neither verb, or every row ends in an empty cell.
export function vocabMenuColumn<Row>(
  t: Translator,
  verbs: VocabVerbs,
  menu: (row: Row) => VocabRow,
): DataTableColumn<Row>[] {
  if (!verbs.canEdit && !verbs.canRemove) return [];
  return [
    {
      key: "verbs",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (row) => {
        const { label, refusal, onRename, onRemove } = menu(row);
        return (
          <VocabRowMenu
            label={label}
            verbs={verbs}
            refusal={refusal}
            onRename={onRename}
            onRemove={onRemove}
          />
        );
      },
    },
  ];
}

// A refused remove stays in the menu with its reason in words, as the server
// would answer it with a 409.
export function VocabRowMenu({
  label,
  verbs,
  refusal,
  onRename,
  onRemove,
}: VocabRow & Readonly<{ verbs: VocabVerbs }>) {
  const t = useT();
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: label })}>
        {verbs.canEdit && (
          <Button onClick={onRename}>{t("leadSources.rename")}</Button>
        )}
        {verbs.canRemove && (
          <Button variant="danger" reason={refusal} onClick={onRemove}>
            {t("leadSources.remove")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}

// A landed Remove takes its row's menu with it. Focus goes to the add verb,
// or to the card's title for a seat that may remove but not add.
export function useRemovalFocus() {
  const landed = useRef(false);
  const addVerb = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  return {
    addVerb,
    titleId,
    opened: () => {
      landed.current = false;
    },
    landed: () => {
      landed.current = true;
    },
    returnFocusTo: (): HTMLElement | null => {
      if (!landed.current) return null;
      if (addVerb.current) return addVerb.current;
      return document.getElementById(titleId);
    },
  };
}
