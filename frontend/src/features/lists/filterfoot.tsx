// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The filter Panel's footer band. The leading edge says what state the filter
// is in. The trailing edge holds the ways to keep it, the one accent last. The
// export stands on the results panel, beside the count it exports.

import type { ReactNode } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isComplete, type Node } from "../../lib/segmentpredicate";
import { proposedCount } from "./filterproposal";
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
  tree,
  mode,
  onSave,
  problem,
}: Readonly<{
  tree: Node;
  mode: FootMode;
  /** Opens "Save this filter": the new filter's Save, or "Save as new view". */
  onSave: () => void;
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
      primary={<FootVerbs tree={tree} mode={mode} onSave={onSave} />}
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
  tree,
  mode,
  onSave,
}: Readonly<{
  tree: Node;
  mode: FootMode;
  onSave: () => void;
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
  return mode.kind === "list" ? (
    <>
      {complete && <Button onClick={onSave}>{t("filters.saveAsNew")}</Button>}
      {mode.saveTo}
    </>
  ) : (
    <Button variant="primary" disabled={!complete} onClick={onSave}>
      {t("filters.save")}
    </Button>
  );
}
