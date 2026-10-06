// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Exporting what the filter selects (AC-filters-and-views-8).
//
// `/exports` takes the same predicate the preview does, so this sends the tree
// the reader is looking at rather than a saved view's id: what you export is
// what the count above it says, through the one filter engine. Nothing here
// re-derives a slice.
//
// Two things this file does NOT do, both deliberately. It does not page the
// export — the server renders the whole matching slice, and a client-side
// stitch of preview pages would be a second, wrong answer to the same question.
// And it does not ask the reader to confirm: clicking "Export CSV" IS the
// confirmation, and the server writes the audit row that makes the export
// accountable (P7/P12).

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { downloadBytes, filenameFromDisposition } from "./download";
import type { FilterResource } from "./filterdata";
import { encode, isComplete, type Node } from "./segmentpredicate";

/** The formats the contract offers. Closed — the server enumerates these two. */
const FORMATS = ["csv", "json"] as const;
type Format = (typeof FORMATS)[number];

const MIME: Record<Format, string> = {
  csv: "text/csv",
  json: "application/json",
};

/** A table rather than a ternary, so adding a third format cannot forget one. */
const LABEL: Record<Format, MessageKey> = {
  csv: "filters.exportCsv",
  json: "filters.exportJson",
};

/**
 * Whether this filter can be exported at all — the ONE reading of that.
 *
 * Gated on `isComplete` for the same reason Save is: an incomplete tree is one
 * the engine refuses, and an export button that answers 422 has told the reader
 * nothing they could not have been spared.
 *
 * Asked before the menu holding the items is drawn: a menu whose items all
 * refuse is a trigger that opens onto nothing.
 */
export function canExportFilter(tree: Node): boolean {
  return isComplete(tree);
}

/**
 * One export, run. Held by the page rather than by the items, because the
 * items live in a menu that closes as they are pressed, and a refusal has to
 * stay on screen after it does.
 */
export function useFilterExport() {
  return useMutation({
    // Everything arrives as a variable: the press belongs to the committed
    // render, so the tree it hands over is the filter the reader is looking at.
    mutationFn: async (
      input: Readonly<{
        format: Format;
        resource: FilterResource;
        tree: Node;
      }>,
    ) => {
      const { data, error, response } = await api.POST("/exports", {
        body: {
          object: input.resource,
          filter: encode(input.tree),
          format: input.format,
        },
        // The body is a rendered file, not a document to parse. Asking for text
        // keeps CSV intact — JSON.parse over a CSV would throw, and over the
        // JSON format it would reparse bytes we are about to write out verbatim.
        parseAs: "text",
      });
      if (error) {
        throwProblem(error);
      }
      downloadBytes(
        data,
        // The server knows which table and format it just rendered, so its name
        // is the true one; the fallback only covers a response that sent none.
        filenameFromDisposition(
          response.headers.get("Content-Disposition"),
          `${input.resource}-export.${input.format}`,
        ),
        MIME[input.format],
      );
    },
  });
}

export type FilterExportRun = ReturnType<typeof useFilterExport>;

/** "Export CSV" and "Export JSON", as items of the menu that holds them. */
export function ExportFilterItems({
  run,
  resource,
  tree,
}: Readonly<{ run: FilterExportRun; resource: FilterResource; tree: Node }>) {
  const t = useT();
  return (
    <>
      {FORMATS.map((format) => (
        <Button
          key={format}
          disabled={run.isPending}
          onClick={() => run.mutate({ format, resource, tree })}
        >
          {t(LABEL[format])}
        </Button>
      ))}
    </>
  );
}

/**
 * How the last export stands, beside the verbs that asked for it. The menu
 * that asked has closed, so this says a file is coming, then the server's
 * reason if it refused, and that only while the filter on screen is the one
 * it refused: the run outlives an edit that hides the band and brings it back.
 */
export function ExportProgress({
  run,
  tree,
}: Readonly<{ run: FilterExportRun; tree: Node }>) {
  const t = useT();
  return (
    <>
      {run.isPending && (
        <span className="t-caption" role="status">
          {t("filters.exporting")}
        </span>
      )}
      <ErrorLine
        inline
        error={run.variables?.tree === tree ? run.error : null}
      />
    </>
  );
}
