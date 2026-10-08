// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The filter Panel's footer band: what state the filter is in on the leading
// edge, and the one emerald way to keep it on the trailing edge, with the
// rarer verbs folded into More beside it.

import type { ReactNode } from "react";
import { ActionRow } from "../design-system/actionrow";
import { Button, OverflowMenu } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { FilterResource } from "./filterdata";
import {
  ExportFilterItems,
  ExportProgress,
  type FilterExportRun,
} from "./filterexport";
import { proposedCount } from "./filterproposal";
import { isComplete, type Node } from "./segmentpredicate";
import "./filters.css";

/** What the filter on the page is, which decides what keeping it means. */
export type FootMode =
  /** Nobody has kept it yet: Save asks how to keep it. */
  | Readonly<{ kind: "new" }>
  /** An opened saved view, edited in place and saved back over itself. */
  | Readonly<{
      kind: "view";
      changed: boolean;
      onDone: () => void;
      onDiscard: () => void;
      onSaveChanges: () => void;
      /** Save changes is out: nothing else may move the tree it sent. */
      saving: boolean;
    }>
  /** A Live List's filter: its own "Save to {name}" is the primary. */
  | Readonly<{
      kind: "list";
      changed: boolean;
      onDiscard: () => void;
      saveTo: ReactNode;
    }>;

export function FilterFoot({
  resource,
  tree,
  mode,
  onSave,
  exportRun,
  problem,
}: Readonly<{
  resource: FilterResource;
  tree: Node;
  mode: FootMode;
  /** Opens "Save this filter": the new filter's Save, or "Save as new view". */
  onSave: () => void;
  exportRun: FilterExportRun;
  /** Why the last save was refused, with the way out of it. */
  problem?: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const proposed = proposedCount(tree);
  const saving = mode.kind === "view" && mode.saving;
  // Proposed rows never hold Save back: a save stores them as plain
  // conditions, and the band says so before the press.
  const status =
    proposed > 0
      ? plural("filters.foot.proposed", proposed, {
          count: formatNumber(proposed, locale),
        })
      : t(statusOf(mode));
  return (
    <ActionRow
      className="filters-foot"
      primary={
        <FootVerbs
          resource={resource}
          tree={tree}
          mode={mode}
          onSave={onSave}
          exportRun={exportRun}
        />
      }
    >
      <span className="t-caption">{status}</span>
      {mode.kind === "view" && !mode.changed && (
        <Button variant="link" onClick={mode.onDone}>
          {t("filters.done")}
        </Button>
      )}
      {mode.kind !== "new" && mode.changed && (
        <Button variant="link" disabled={saving} onClick={mode.onDiscard}>
          {t("filters.discardChanges")}
        </Button>
      )}
      {/* An opened view exports from its head, so its progress shows there. */}
      {mode.kind !== "view" && <ExportProgress run={exportRun} tree={tree} />}
      {problem}
    </ActionRow>
  );
}

function statusOf(mode: FootMode): MessageKey {
  if (mode.kind === "new") {
    return "filters.unsavedFilter";
  }
  return mode.changed ? "filters.unsavedChanges" : "filters.noChanges";
}

/**
 * The trailing edge. Every verb here sends the tree, and an unfinished one is a
 * tree the engine refuses, so each waits for a complete filter.
 */
function FootVerbs({
  resource,
  tree,
  mode,
  onSave,
  exportRun,
}: Readonly<{
  resource: FilterResource;
  tree: Node;
  mode: FootMode;
  onSave: () => void;
  exportRun: FilterExportRun;
}>) {
  const t = useT();
  const complete = isComplete(tree);
  if (mode.kind === "view") {
    return mode.changed ? (
      <>
        <Button disabled={!complete || mode.saving} onClick={onSave}>
          {t("filters.saveAsNew")}
        </Button>
        <Button
          variant="primary"
          disabled={!complete}
          pending={mode.saving}
          onClick={mode.onSaveChanges}
        >
          {t("filters.saveChanges")}
        </Button>
      </>
    ) : null;
  }
  return (
    <>
      {complete && (
        <OverflowMenu label={t("filters.footMore")}>
          {mode.kind === "list" && (
            <Button onClick={onSave}>{t("filters.saveAsNew")}</Button>
          )}
          <ExportFilterItems run={exportRun} resource={resource} tree={tree} />
        </OverflowMenu>
      )}
      {mode.kind === "list" ? (
        mode.saveTo
      ) : (
        <Button variant="primary" disabled={!complete} onClick={onSave}>
          {t("filters.save")}
        </Button>
      )}
    </>
  );
}
