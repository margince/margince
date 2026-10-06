// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The filter Panel's footer band, drawn once the filter is one worth keeping:
// what state it is in on the leading edge, and the one emerald way to keep it
// on the trailing edge, with the rarer verbs folded into More beside it.

import type { ReactNode } from "react";
import { ActionRow } from "../design-system/actionrow";
import { Button, OverflowMenu } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { FilterResource } from "./filterdata";
import {
  canExportFilter,
  ExportFilterItems,
  type FilterExportRun,
} from "./filterexport";
import { proposedCount } from "./filterproposal";
import type { Node } from "./segmentpredicate";
import "./filters.css";

export function FilterFoot({
  resource,
  tree,
  onSave,
  exportRun,
  saveTo,
}: Readonly<{
  resource: FilterResource;
  tree: Node;
  /** Opens "Save this filter". */
  onSave: () => void;
  exportRun: FilterExportRun;
  /**
   * A Live List's own "Save to {name}", which takes Save's place. Saving the
   * filter as a new view then moves into More, beside the exports.
   */
  saveTo?: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const exportable = canExportFilter(tree);
  const proposed = proposedCount(tree);
  const more = saveTo !== undefined || exportable;
  return (
    <ActionRow
      className="filters-foot"
      primary={
        <>
          {more && (
            <OverflowMenu label={t("filters.footMore")}>
              {saveTo !== undefined && (
                <Button onClick={onSave}>{t("filters.saveAsNew")}</Button>
              )}
              {exportable && (
                <ExportFilterItems
                  run={exportRun}
                  resource={resource}
                  tree={tree}
                />
              )}
            </OverflowMenu>
          )}
          {saveTo ?? (
            <Button variant="primary" onClick={onSave}>
              {t("filters.save")}
            </Button>
          )}
        </>
      }
    >
      {/* Proposed rows never hold Save back: a save stores them as plain
          conditions, and the band says so before the press. */}
      <span className="t-caption">
        {proposed > 0
          ? plural("filters.foot.proposed", proposed, {
              count: formatNumber(proposed, locale),
            })
          : t("filters.unsavedFilter")}
      </span>
      {/* The menu that asked has closed, so the band says a file is coming. */}
      {exportRun.isPending && (
        <span className="t-caption" role="status">
          {t("filters.exporting")}
        </span>
      )}
      {/* The server's own reason, beside the band the export was asked from,
          and only while the filter on screen is the one it refused: the run
          outlives an edit that takes the footer away and brings it back. */}
      <ErrorLine
        inline
        error={exportRun.variables?.tree === tree ? exportRun.error : null}
      />
    </ActionRow>
  );
}
