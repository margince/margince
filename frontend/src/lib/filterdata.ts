// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reads a filter builder needs: the vocabulary, named from the custom-field
// catalog, and the preview.
//
// Kept apart from the screen for one reason: the screen decides what a human
// SEES, and these decide what the server was ASKED. The interesting rules here
// are about when not to ask — a tree mid-edit is not worth a round trip, and a
// vocabulary is worth exactly one per record type.

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useCallback } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useLocale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { catalogOfRecord, customFieldLabel } from "./customfields.logic";
import { customFieldsQuery } from "./customfields.query";
import { previousOfScope, useHealthScope } from "./healthscope";
import { historyFieldLabelKey } from "./historyfieldlabels";
import { unwrap } from "./problem";
import {
  type EncodedPredicate,
  encode,
  isComplete,
  type Node,
} from "./segmentpredicate";

/** A vocabulary field. A custom one carries its admin's label in the reader's language once the catalog answers. */
export type VocabularyField = components["schemas"]["FilterVocabularyField"] & {
  label?: string;
};
export type FilterVocabulary = Omit<
  components["schemas"]["FilterVocabulary"],
  "fields"
> & { fields: VocabularyField[] };
type CustomField = components["schemas"]["CustomField"];
export type FilterPreview = components["schemas"]["FilterPreview"];

/** The record types a filter may name — the contract's own closed set. */
export type FilterResource = FilterVocabulary["resource"];

/**
 * The page this screen asks for, sent on every request rather than left absent.
 * The generated body type makes `limit` required: openapi-typescript reads a
 * schema `default` as "always present".
 *
 * The contract's ceiling, and a whole multiple of every size the table's dial
 * offers (`PAGE_SIZES` in `listtable.paging.ts`), so a rendered page never
 * straddles two fetches.
 */
export const PREVIEW_PAGE = 100;

export function vocabularyQueryKey(resource: FilterResource) {
  return ["filter-vocabulary", resource] as const;
}

/**
 * What this record type may be filtered on.
 *
 * Cached per resource and never invalidated by editing: the vocabulary changes
 * when an admin adds or retires a custom field, not when somebody builds a
 * clause, so one fetch serves a whole editing session. That is also why the
 * field picker reads THIS rather than a list of its own — the server owns which
 * fields exist, and a second copy would offer one the engine refuses.
 *
 * `enabled` lets a surface that needs it only sometimes call the hook
 * unconditionally and still ask nothing when it does not.
 */
export function useFilterVocabulary(resource: FilterResource, enabled = true) {
  const { locale } = useLocale();
  const catalog = useQuery(
    customFieldsQuery(catalogOfRecord(resource), enabled),
  ).data?.data;
  const labelled = useCallback(
    (vocabulary: FilterVocabulary) =>
      withCatalogLabels(vocabulary, catalog, locale),
    [catalog, locale],
  );
  return useQuery<FilterVocabulary>({
    queryKey: vocabularyQueryKey(resource),
    enabled,
    queryFn: async () => {
      return unwrap(
        await api.GET("/filters/vocabulary", {
          params: { query: { resource } },
        }),
      );
    },
    select: labelled,
  });
}

/** Each custom field labelled from its catalog entry. One the catalog lacks stays bare. */
function withCatalogLabels(
  vocabulary: FilterVocabulary,
  catalog: readonly CustomField[] | undefined,
  locale: string,
): FilterVocabulary {
  if (!catalog?.length) {
    return vocabulary;
  }
  const byColumn = new Map(catalog.map((field) => [field.column_name, field]));
  return {
    ...vocabulary,
    fields: vocabulary.fields.map((field) => {
      const entry = field.custom ? byColumn.get(field.name) : undefined;
      return entry
        ? {
            ...field,
            label: customFieldLabel(entry, locale),
          }
        : field;
    }),
  };
}

/**
 * The match count and every page fetched so far for a tree nobody has saved.
 * `data` reads as one answer: the count and columns of the first page, and the
 * rows of every page in order. `fetchNextPage` asks for the page after them.
 *
 * Two things this deliberately does not do.
 *
 * It does not ask about an INCOMPLETE tree. A clause whose value is still empty
 * would be refused with a 422 naming the field, which is correct of the server
 * and useless to a human who has simply not finished typing — so `isComplete`
 * gates the request and the caller renders the previous count until the clause
 * is whole. The server's refusal stays the backstop for a tree that is complete
 * and still wrong.
 *
 * And it does not key on the tree's identity — it keys on the ENCODED tree, so
 * two edits that produce the same filter (retyping a value back, reordering
 * nothing) reuse one cached answer instead of spending a request to learn the
 * count did not move.
 */
export function useFilterPreview(resource: FilterResource, tree: Node) {
  const askable = isComplete(tree);
  const filter = encode(tree);
  // The preview's rows are records the reader's row scope narrows.
  const readerScope = useHealthScope();
  const query = useInfiniteQuery({
    queryKey: ["filter-preview", resource, filter, readerScope ?? ""],
    enabled: askable && readerScope !== undefined,
    // A count that flickers to a spinner on every keystroke is harder to read
    // than one that lags: the previous answer stays on screen while the next is
    // in flight, and the caller shows staleness rather than absence.
    placeholderData: previousOfScope(readerScope),
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }): Promise<FilterPreview> => {
      return unwrap(
        await api.POST("/filters/preview", {
          body: {
            resource,
            filter,
            limit: PREVIEW_PAGE,
            ...(pageParam === null ? {} : { cursor: pageParam }),
          },
        }),
      );
    },
    getNextPageParam: (last) => last.next_cursor ?? null,
  });
  return { ...query, data: joinedPages(query.data?.pages) };
}

/** The pages of one preview read as one answer, the last page saying what follows. */
function joinedPages(
  pages: readonly FilterPreview[] | undefined,
): FilterPreview | undefined {
  const first = pages?.[0];
  const last = pages?.at(-1);
  if (!pages || !first || !last) {
    return undefined;
  }
  return {
    ...first,
    rows: pages.flatMap((page) => page.rows),
    truncated: last.truncated,
    next_cursor: last.next_cursor,
  };
}

/**
 * The offerable fields grouped for a picker: core first, then custom.
 *
 * Core-before-custom because a reader looking for "Owner" should not scroll past
 * a workspace's twenty bespoke columns to find it. Order within a group is the
 * picker's to set, by the label it shows.
 */
export function groupFields(fields: readonly VocabularyField[]) {
  const core: VocabularyField[] = [];
  const custom: VocabularyField[] = [];
  for (const field of fields) {
    (field.custom ? custom : core).push(field);
  }
  return { core, custom };
}

// What each core filter field is CALLED. The vocabulary carries wire names
// (`owner_id`), and a builder that printed them spoke a column's language to a
// reader who never sees one. A column the History tab already names takes that
// word from `historyFieldLabelKey`, so a field reads the same on both surfaces;
// this map holds only the fields History never names.
//
// Held to the engine by backend/gates/frontendfilterfieldlabels_test.go: an
// engine field neither map names fails, and so does an entry here for a field
// no engine has or one History already names.
const FILTER_ONLY_FIELD_LABELS = new Map<string, MessageKey>([
  ["amount", "filters.field.amount"],
  ["city", "filters.field.city"],
  ["classification", "filters.field.classification"],
  ["company_country", "filters.field.company_country"],
  ["company_industry", "filters.field.company_industry"],
  ["company_lifecycle", "filters.field.company_lifecycle"],
  ["company_size_band", "filters.field.company_size_band"],
  ["country", "filters.field.country"],
  ["created_at", "filters.field.created_at"],
  ["domain", "history.field.domain"],
  ["hosting_provider", "filters.field.hosting_provider"],
  ["last_activity_at", "filters.field.last_activity_at"],
  ["location", "filters.field.location"],
  ["mail_provider", "filters.field.mail_provider"],
  ["operated_service", "filters.field.operated_service"],
  ["owner_team_id", "filters.field.owner_team_id"],
  ["phase", "filters.field.phase"],
  ["pipeline_id", "filters.field.pipeline_id"],
  ["relationship_type", "filters.field.relationship_type"],
  ["stage_id", "filters.field.stage_id"],
  ["tag", "filters.field.tag"],
  ["technology", "filters.field.technology"],
]);

/**
 * The label a picker shows for a field.
 *
 * A custom field reads by the label the catalog joined on. One the catalog has
 * not named reads as its column, minus the `cf_` prefix and underscores. A core
 * field the map does not name falls back the same way rather than vanishing from
 * the picker.
 */
export function fieldLabel(
  field: VocabularyField,
  t: (key: MessageKey) => string,
): string {
  if (field.custom) {
    return field.label ?? customColumnLabel(field.name);
  }
  const key =
    historyFieldLabelKey(field.name) ??
    FILTER_ONLY_FIELD_LABELS.get(field.name);
  return key ? t(key) : field.name.replaceAll("_", " ");
}

/**
 * A custom column named by its catalog, which still holds a retired field the
 * vocabulary no longer offers. One the catalog lacks reads as its column.
 */
export function useCustomColumnLabel(recordKey: string, enabled = true) {
  const { locale } = useLocale();
  const catalog = useQuery(
    customFieldsQuery(catalogOfRecord(recordKey), enabled),
  ).data?.data;
  return (column: string) => {
    const field = catalog?.find((entry) => entry.column_name === column);
    return field ? customFieldLabel(field, locale) : customColumnLabel(column);
  };
}

/** A custom column's `cf_` name read as words, as `fieldLabel` reads it. */
export function customColumnLabel(column: string): string {
  return column.replace(/^cf_/, "").replaceAll("_", " ");
}

/** The wire shape a caller hands the preview, for tests that assert the body. */
export type { EncodedPredicate };
