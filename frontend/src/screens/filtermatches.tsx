// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a filter selects: the count and the first page of rows behind it, the
// one rule for when they are on screen at all, and the line that says what
// would bring them while they are not.

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryStates } from "./common";
import {
  PREVIEW_PAGE,
  useFilterPreview,
  type VocabularyField,
} from "./filterdata";
import { type VocabularyRead, vocabularyState } from "./filtereditor";
import { FilterResults } from "./filterresults";
import {
  MATCH_LABEL,
  type ObjectTab,
  RESOURCE_OF,
  UNIT_LABEL,
} from "./filtersaddress";
import {
  fieldsNamed,
  type Group,
  isComplete,
  isGroup,
  type Node,
} from "./segmentpredicate";
import "./filters.css";

type Preview = ReturnType<typeof useFilterPreview>;

/**
 * The most rows the reader may ask the preview for. The authority is the
 * contract's ceiling on `FilterPreviewRequest.limit`, which the server holds.
 */
const MOST_ROWS = 100;

/**
 * The count, and whether it is behind.
 *
 * Three readings, and keeping them apart is the point. A count the server has
 * answered reads plainly. A count being recomputed reads as the LAST answer,
 * marked stale — not as a spinner, because a number that vanishes on every
 * keystroke is harder to read than one that lags a moment. And a count the
 * server was asked for and refused says exactly that. Before the first answer
 * there is no count at all: zero would mean "nothing matches".
 *
 * The refusal outranks the others. It is read first because the previous
 * answer survives a failed refetch, so a stale number would otherwise be
 * presented as current.
 */
export function MatchCount({
  label,
  count,
  stale,
  failed,
}: Readonly<{
  label: PluralBase;
  count: number | undefined;
  stale: boolean;
  failed: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (failed) {
    // Silent: the results card carries the reason in an assertive live
    // region, and announcing the same failure twice fragments it.
    return (
      <span className="filters-count">
        <ErrorLine inline standing>
          {t("filters.countUnavailable")}
        </ErrorLine>
      </span>
    );
  }
  if (count === undefined) {
    return null;
  }
  return (
    <span
      className="filters-count"
      // Spoken, because the count changing is the feedback for every edit — a
      // sighted reader sees the number move and a screen-reader user would
      // otherwise get nothing back from adding a clause.
      role="status"
      aria-busy={stale}
      data-stale={stale ? "true" : undefined}
    >
      {plural(label, count, { count: formatNumber(count, locale) })}
    </span>
  );
}

/**
 * Whether the results belong on screen for this tree.
 *
 * From the first complete condition, and still while the reader finishes a
 * second one — then the rows are the last answer and the page says the count
 * is waiting. Never from a preview held over an emptied tree: the preview
 * keeps its last answer even while it asks nothing, so whether this tree was
 * ever answered is remembered here instead, and forgotten when it empties.
 */
function useResultsShown(tree: Group, preview: Preview): boolean {
  const [answered, setAnswered] = useState(false);
  const empty = tree.children.length === 0;
  const complete = isComplete(tree);
  const fresh = preview.isSuccess && !preview.isPlaceholderData;
  if (empty && answered) {
    setAnswered(false);
  } else if (complete && fresh && !answered) {
    setAnswered(true);
  }
  return !empty && (complete || answered);
}

/**
 * The results panel: the count beside its title, the rows, and the way to ask
 * for more of them. The table's own page-size dial is the request's limit, so
 * the rows it shows are the rows the server sent and no pager walks past them.
 */
export function FilterMatches({
  preview,
  tab,
  fields,
  named,
  andJoined,
  limit,
  onLimit,
}: Readonly<{
  preview: Preview;
  tab: ObjectTab;
  fields: readonly VocabularyField[];
  /** The fields the filter names, in the order they were written. */
  named: readonly string[];
  /** A group joins conditions with "and", so turning it to "or" loosens it. */
  andJoined: boolean;
  limit: number;
  onLimit: (next: number) => void;
}>) {
  const t = useT();
  const answer = preview.data;
  const unit = t(UNIT_LABEL[tab]);
  return (
    <Panel
      title={t("filters.resultsTitle")}
      titleAction={
        <MatchCount
          label={MATCH_LABEL[tab]}
          count={answer?.match_count}
          stale={preview.isFetching}
          failed={preview.isError}
        />
      }
    >
      <PanelBody>
        <QueryStates
          query={preview}
          pendingLabel={t("filters.resultsTitle")}
          pendingLines={6}
        >
          {answer && (
            <div className="filters-matches">
              <FilterResults
                preview={answer}
                fields={fields}
                named={named}
                unit={unit}
                emptyNote={t(
                  andJoined ? "filters.noMatches" : "filters.noMatchesLoosen",
                  { records: unit },
                )}
                // Per object, so switching tabs does not hand a deal's table
                // the widths a reader dragged for a contact's columns.
                widthsKey={`filter-preview-${tab}`}
                pending={preview.isFetching}
                total={answer.match_count}
                perPage={limit}
                onPerPage={onLimit}
              />
              {answer.match_count > answer.rows.length && limit < MOST_ROWS && (
                <p className="filters-more">
                  <Button variant="link" onClick={() => onLimit(MOST_ROWS)}>
                    {t("filters.showMore")}
                  </Button>
                </p>
              )}
            </div>
          )}
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

/**
 * Everything under a filter's editor: the line saying what would bring a
 * count, and the results once there is one. The page's preview lives here, its
 * page size set by the table's own dial.
 */
export function FilterOutcome({
  tab,
  tree,
  vocabulary,
}: Readonly<{
  tab: ObjectTab;
  tree: Group;
  vocabulary: VocabularyRead;
}>) {
  const [limit, setLimit] = useState(PREVIEW_PAGE);
  const preview = useFilterPreview(RESOURCE_OF[tab], tree, limit);
  const shown = useResultsShown(tree, preview);
  return (
    <>
      {/* Only beside an editor the reader can use: while the fields are
          read, or could not be, there is no condition for it to ask for. */}
      {vocabularyState(vocabulary) === "ready" && (
        <WaitingLine
          shown={shown}
          empty={tree.children.length === 0}
          complete={isComplete(tree)}
          records={UNIT_LABEL[tab]}
        />
      )}
      {shown && (
        <FilterMatches
          preview={preview}
          tab={tab}
          fields={vocabulary.data?.fields ?? []}
          named={fieldsNamed(tree)}
          andJoined={joinsWithAnd(tree)}
          limit={limit}
          onLimit={setLimit}
        />
      )}
    </>
  );
}

/** Whether some group joins two or more conditions with "and". */
function joinsWithAnd(node: Node): boolean {
  return (
    isGroup(node) &&
    ((node.join === "and" && node.children.length > 1) ||
      node.children.some(joinsWithAnd))
  );
}

/**
 * The line under the editor while the results are not the whole story: what
 * would bring a count, or that the count on screen waits for the condition
 * the reader is still writing.
 */
function WaitingLine({
  shown,
  empty,
  complete,
  records,
}: Readonly<{
  shown: boolean;
  empty: boolean;
  complete: boolean;
  records: MessageKey;
}>) {
  const t = useT();
  if (shown && complete) {
    return null;
  }
  return (
    <p className="t-sub filters-hint">
      {shown
        ? t("filters.hint.update")
        : t(empty ? "filters.hint.start" : "filters.hint.finish", {
            records: t(records),
          })}
    </p>
  );
}
