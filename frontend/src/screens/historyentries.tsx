import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
} from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import type { EntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { Card, EmptyState } from "../design-system/atoms";
import { FieldDiff, ProvenanceTag } from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  LoadMoreButton,
  QueryStates,
  throwProblem,
  useViewerId,
} from "./common";
import { entryFieldChanges, provenanceOfEntry } from "./history.logic";
import { historyFieldLabel } from "./historyfieldlabels";
import { historyValue } from "./historyvalues";
import "./history.css";

// The per-record plain-language change list (B-EP09.x): every audit_log row
// for one record, rendered as a `summary` line the endpoint already wrote in
// prose — this panel never re-derives wording from before/after diffs, it
// just attributes and paginates what the contract hands back.

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];
type AuditHistoryListResponse =
  components["schemas"]["AuditHistoryListResponse"];

export function useRecordHistory(
  kind: EntityKind,
  id: string,
  // Whether to fetch at all. A caller that reads the history to describe ONE
  // recorded event — the lead page naming what its promotion did — has nothing
  // to read on a record where that event never happened, and a disabled query
  // says so rather than paying for a page nothing renders. Defaults to on, so
  // the History tab is unchanged.
  enabled = true,
): UseInfiniteQueryResult<InfiniteData<AuditHistoryListResponse>> {
  return useInfiniteQuery({
    queryKey: ["record-history", kind, id],
    enabled,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET(
        "/records/{entity_type}/{id}/history",
        {
          params: {
            path: { entity_type: kind, id },
            query: { limit: 20, ...(pageParam ? { cursor: pageParam } : {}) },
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last?.page?.next_cursor ?? null,
  });
}

// What one entry CHANGED, under the sentence saying that it did.
//
// It sits on this list rather than only in the per-field panel because this is
// the list a reader opens first, and a change nobody can see is a change
// nobody can check.
function EntryFieldDetail({
  entry,
  currency,
}: Readonly<{
  entry: AuditHistoryEntry;
  currency: string | null | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const changes = entryFieldChanges(entry);
  if (changes.length === 0) {
    return null;
  }
  return (
    <ul className="entry-fields">
      {changes.map((change) => (
        <li key={change.field} className="entry-field">
          <span className="entry-field-name">
            {historyFieldLabel(change.field, t)}
          </span>
          <FieldDiff
            oldValue={historyValue(
              change.field,
              change.oldValue,
              currency,
              locale,
            )}
            newValue={historyValue(
              change.field,
              change.newValue,
              currency,
              locale,
            )}
          />
        </li>
      ))}
    </ul>
  );
}

function HistoryEntryRow({
  entry,
  locale,
  currency,
}: Readonly<{
  entry: AuditHistoryEntry;
  locale: ReturnType<typeof useLocale>["locale"];
  currency: string | null | undefined;
}>) {
  const viewerId = useViewerId();
  const recordZone = useRecordZone();
  return (
    <li>
      <span className="tl-body">
        {/* `summary` already NAMES the granting human as its subject
            ("Ada Authority, via an agent, updated the record"), so the
            on-behalf-of suffix that used to complete the old machine-first
            sentence would now say the same person twice. */}
        <span className="tl-title">{entry.summary}</span>
        <span className="tl-meta">
          <span>{formatDateTime(entry.occurred_at, locale, recordZone)}</span>
          <ProvenanceTag
            provenance={provenanceOfEntry(entry, viewerId)}
            // The design system has no record lookups, so the resolved name
            // has to be handed in. The read path resolves it for exactly this:
            // without it the chip says a person entered the row without
            // claiming which one, which is the same "nobody to ask" the
            // sentence above was fixed to avoid.
            renderUser={() => entry.actor_name}
          />
        </span>
        <EntryFieldDetail entry={entry} currency={currency} />
      </span>
    </li>
  );
}

export function RecordHistory({
  kind,
  id,
  currency,
}: Readonly<{
  kind: EntityKind;
  id: string;
  // The record's ISO currency, which is what gives a minor-unit column in the
  // detail below a row its scale.
  currency?: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useRecordHistory(kind, id);
  const entries = query.data?.pages.flatMap((page) => page.data) ?? [];

  // Honest state matrix (§3a): the pending/error halves are QueryStates'
  // (shared with FieldHistoryTimeline and QueryGate); empty vs. the list is
  // this component's own success rendering.
  let body: ReactNode;
  if (entries.length === 0) {
    body = <EmptyState>{t("history.empty")}</EmptyState>;
  } else {
    body = (
      <>
        <ul className="timeline">
          {entries.map((entry) => (
            <HistoryEntryRow
              key={entry.id}
              entry={entry}
              locale={locale}
              currency={currency}
            />
          ))}
        </ul>
        <LoadMoreButton query={query} />
      </>
    );
  }

  return (
    <Card style={{ marginBottom: "var(--space-4)" }}>
      <QueryStates query={query}>{body}</QueryStates>
    </Card>
  );
}
