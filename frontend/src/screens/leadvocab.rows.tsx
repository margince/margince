// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemMessageOf } from "./common";
import "./leadvocab.css";

// The contract promises the arrays. A body that lost one reads as the empty
// list it claims, not as a crash in the render.
export function rowsOf<Row>(
  rows: readonly Row[] | null | undefined,
): readonly Row[] {
  return Array.isArray(rows) ? rows : [];
}

/** A refused name write, split the way `NameDialog` draws it. */
export function nameRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
  duplicate: MessageKey,
): { problem: string | null; nameProblem: string | null } {
  if (error === null || error === undefined) {
    return { problem: null, nameProblem: null };
  }
  return problemCodeOf(error) === "conflict"
    ? { problem: null, nameProblem: t(duplicate) }
    : { problem: problemMessageOf(error, t), nameProblem: null };
}

// Folded, the column heading is out of sight, so the count reads as a sentence.
export function LeadCount({ count }: Readonly<{ count: number }>) {
  const { locale } = useLocale();
  const plural = usePlural();
  const figure = formatNumber(count, locale);
  return (
    <>
      <span className="lead-vocab-figure">{figure}</span>
      <span className="lead-vocab-sentence">
        {plural("leadSources.leads", count, { count: figure })}
      </span>
    </>
  );
}

// Rename and Remove for one entry. A refused remove stays in the menu with its
// reason in words, because the server would answer the delete with a 409.
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
      <OverflowMenu label={t("leadSources.rowActions", { label })}>
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

// Said once for the whole card rather than on each refused control.
export function VocabNotices({
  readOnly = false,
  error,
}: Readonly<{ readOnly?: boolean; error: unknown }>) {
  const t = useT();
  if (!readOnly && error === undefined) return null;
  return (
    <PanelBody className="lead-vocab-notices">
      {readOnly && (
        <Callout kind="standing" title={t("leadSources.readOnlyTitle")} />
      )}
      {error !== undefined && (
        <Callout kind="outcome" tone="danger" title={t("leadSources.notSaved")}>
          {problemMessageOf(error, t)}
        </Callout>
      )}
    </PanelBody>
  );
}
