import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
} from "@tanstack/react-query";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import type { EntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import {
  Button,
  Card,
  EmptyState,
  SegmentedControl,
} from "../design-system/atoms";
import type { TimelineEntry } from "../design-system/composed";
import {
  EvidenceChip,
  PassportChip,
  ProvenanceTag,
  toEvidence,
} from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  LoadMoreButton,
  QueryStates,
  throwProblem,
  useViewerId,
} from "./common";
import {
  type ActorFacet,
  distinctFields,
  type FieldGroup,
  groupByField,
  provenanceOfEntry,
} from "./history.logic";
import { HistoryFieldDiff } from "./historyfielddiff";
import { historyFieldLabel } from "./historyfieldlabels";
import type { HistoryValueCtx } from "./historyvalues";
import "./history.css";

// The per-field old→new diff view (B-EP09.x): every field-change row the
// audit spine projects for one record, grouped by field and narrowable by
// actor and field — both filters are server-side query params (part of the
// queryKey), never a client-side re-slice of an already-fetched page.

type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];
type FieldHistoryListResponse =
  components["schemas"]["FieldHistoryListResponse"];

const ACTOR_FACETS = ["all", "human", "agent"] as const;

export function useFieldHistory(
  kind: EntityKind,
  id: string,
  opts: Readonly<{
    field?: string;
    actorType?: "human" | "agent";
    // Off by default on a surface that only shows changes on request — the
    // record page's timeline filter — so opening an account does not spend a
    // read nobody asked for. Omitted means enabled, for the callers that ARE
    // the change view.
    enabled?: boolean;
  }>,
): UseInfiniteQueryResult<InfiniteData<FieldHistoryListResponse>> {
  const { field, actorType, enabled } = opts;
  return useInfiniteQuery({
    queryKey: ["field-history", kind, id, field ?? "", actorType ?? ""],
    enabled: enabled ?? true,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/field-history", {
        params: {
          query: {
            entity_type: kind,
            entity_id: id,
            limit: 20,
            ...(pageParam ? { cursor: pageParam } : {}),
            ...(field ? { field } : {}),
            ...(actorType ? { actor_type: actorType } : {}),
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last?.page?.next_cursor ?? null,
  });
}

// Every actor type gets a base ProvenanceTag (human/agent — system and
// connector read as "agent", same as the record-level HistoryEntryRow and
// settings.tsx's AuditLogRow), so no actor ever renders a blank attribution;
// the passport/evidence chips layer on top only when the change carries them.
function ChangeWho({ change }: Readonly<{ change: FieldHistoryEntry }>) {
  const viewerId = useViewerId();
  return (
    <span className="who t-caption">
      <ProvenanceTag provenance={provenanceOfEntry(change, viewerId)} />
      <ChangeGrounding change={change} />
    </span>
  );
}

// What makes an agent's change checkable: the passport it acted under and the
// evidence it cited. Both surfaces that render a change row use this one, so
// neither can quietly stop showing them.
function ChangeGrounding({ change }: Readonly<{ change: FieldHistoryEntry }>) {
  const evidence = toEvidence(change.evidence);
  if (!change.passport_id && !evidence) {
    return null;
  }
  return (
    <>
      {change.passport_id && <PassportChip id={change.passport_id} />}
      {evidence && <EvidenceChip evidence={evidence} />}
    </>
  );
}

function FieldGroupSection({
  group,
  currency,
}: Readonly<{ group: FieldGroup; currency: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const valueCtx: HistoryValueCtx = { currency, locale, zone: recordZone };
  return (
    <div className="fgroup">
      <div className="fgroup-head t-caption">
        {historyFieldLabel(group.field, t)}
      </div>
      <ul>
        {group.changes.map((change) => (
          <li key={change.id} className="change">
            <HistoryFieldDiff
              field={group.field}
              oldValue={change.old_value}
              newValue={change.new_value}
              values={valueCtx}
            />
            <span className="tl-meta">
              {formatDateTime(change.changed_at, locale, recordZone)}
            </span>
            <ChangeWho change={change} />
          </li>
        ))}
      </ul>
    </div>
  );
}

export function FieldHistoryTimeline({
  kind,
  id,
  currency,
}: Readonly<{
  kind: EntityKind;
  id: string;
  // The record's ISO currency, which is what gives a minor-unit column its
  // scale. Only a deal carries one; every other record type this panel serves
  // has no money column for it to reach.
  currency?: string | null;
}>) {
  const t = useT();
  const [actorFacet, setActorFacet] = useState<ActorFacet>("all");
  const [fieldFilter, setFieldFilter] = useState<string | undefined>(undefined);
  // Accumulates every field name this record has ever shown, across facet/
  // field narrowing — a chip the user has already discovered stays clickable
  // even after a fetch that only returned one field's rows.
  const [fieldOptions, setFieldOptions] = useState<string[]>([]);

  // This component isn't remounted on navigation between records of the same
  // kind (App.tsx keys screens by route, not by record id), so without an
  // explicit reset the accumulator above — and the actor/field filter
  // selections below — would keep carrying the previous record's state onto
  // the newly-viewed one. Adjusted during render (React's documented pattern
  // for resetting state on a prop change) rather than in an Effect, so the
  // reset always lands before the accumulate Effect below reads
  // `fieldOptions`, even when the new record's data is already cached and
  // ready on the very render the id changes.
  const recordKey = `${kind}:${id}`;
  const [resetFor, setResetFor] = useState(recordKey);
  if (resetFor !== recordKey) {
    setResetFor(recordKey);
    setFieldOptions([]);
    setActorFacet("all");
    setFieldFilter(undefined);
  }

  const query = useFieldHistory(kind, id, {
    field: fieldFilter,
    actorType: actorFacet === "all" ? undefined : actorFacet,
  });
  // The Agent/Human facet already narrows server-side via the actor_type
  // query param (part of the queryKey, so a facet change refetches) — these
  // rows are trusted as-is rather than re-sliced client-side, which also
  // keeps pagination (hasNextPage) honest against what the server counted.
  const entries = useMemo(
    () => query.data?.pages.flatMap((page) => page.data) ?? [],
    [query.data],
  );

  useEffect(() => {
    if (entries.length === 0) {
      return;
    }
    const discovered = distinctFields(entries);
    setFieldOptions((prev) => {
      const next = [...prev];
      for (const field of discovered) {
        if (!next.includes(field)) {
          next.push(field);
        }
      }
      return next;
    });
  }, [entries]);

  const isFiltered = actorFacet !== "all" || fieldFilter !== undefined;
  const clearFilters = () => {
    setActorFacet("all");
    setFieldFilter(undefined);
  };

  // Honest state matrix (§3a): the pending/error halves are QueryStates';
  // filter-empty (a narrowing that found nothing) vs. truly empty (no edits
  // at all) is this component's own success rendering.
  let body: ReactNode;
  if (entries.length === 0 && isFiltered) {
    body = (
      <EmptyState>
        <p>{t("history.filterEmpty")}</p>
        <Button
          small
          onClick={clearFilters}
          style={{ marginTop: "var(--space-3)" }}
        >
          {t("history.clearFilter")}
        </Button>
      </EmptyState>
    );
  } else if (entries.length === 0) {
    body = <EmptyState>{t("history.fieldEmpty")}</EmptyState>;
  } else {
    const groups = groupByField(entries);
    body = (
      <>
        {groups.map((group) => (
          <FieldGroupSection
            key={group.field}
            group={group}
            currency={currency}
          />
        ))}
        <LoadMoreButton query={query} />
      </>
    );
  }

  const actorLabels: Record<ActorFacet, string> = {
    all: t("history.actorAll"),
    human: t("history.actorHuman"),
    agent: t("history.actorAgent"),
  };

  return (
    <Card style={{ marginBottom: "var(--space-4)" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: "var(--space-3)",
          marginBottom: "var(--space-3)",
        }}
      >
        <SegmentedControl
          options={ACTOR_FACETS}
          value={actorFacet}
          onChange={setActorFacet}
          labels={actorLabels}
        />
        {fieldOptions.length > 0 && (
          <div
            style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}
          >
            <Button
              small
              variant={fieldFilter === undefined ? "primary" : "ghost"}
              onClick={() => setFieldFilter(undefined)}
            >
              {t("history.allFields")}
            </Button>
            {fieldOptions.map((field) => (
              <Button
                key={field}
                small
                variant={fieldFilter === field ? "primary" : "ghost"}
                onClick={() => setFieldFilter(field)}
              >
                {historyFieldLabel(field, t)}
              </Button>
            ))}
          </div>
        )}
      </div>
      <QueryStates query={query} pendingLabel={t("history.allFields")}>
        {body}
      </QueryStates>
    </Card>
  );
}

// Field changes as timeline rows.
//
// A record page has ONE chronology. What was said to an account and what was
// changed about it were two separate screens, and a reader who wanted them in
// order had to interleave two lists by hand. These rows carry the same shape
// as the activity rows they sit beside, so the merge is a sort, not a second
// rendering.
export function changeTimeline(
  changes: FieldHistoryEntry[],
  label: (field: string) => string,
  // The record's own value context. A minor-unit column is an integer count of
  // units its currency defines, a timestamp is only readable at the record's
  // zone, and a bare id is only readable through the resolver — the reason
  // every diff goes through HistoryFieldDiff rather than to FieldDiff itself,
  // on every field, whether or not the record type holds each shape today.
  valueCtx: HistoryValueCtx,
  // What the record DID, in the reader's words. Handed in rather than read
  // from the catalog here, because this adapter takes every other word it
  // renders from its caller too.
  qualifier: string,
  viewerUserId?: string,
): TimelineEntry[] {
  return changes.map((change) => ({
    // `id` is the AUDIT row's id, and one audit row projects one entry per
    // field it touched — so it repeats across a flat list and only the pair
    // identifies a row. The per-field view never saw this: its groups hold
    // one field each, where the audit id alone happens to be unique.
    id: `change:${change.id}:${change.field}`,
    kind: "change",
    title: label(change.field),
    // The badge says this is a record entry; without this the row does not say
    // what the record did.
    qualifier,
    atIso: change.changed_at,
    provenance: provenanceOfEntry(change, viewerUserId),
    // The grounding travels with the row. An agent's change is only checkable
    // through the passport that made it and the evidence it cited, and moving
    // these rows into the account timeline had left both behind in the
    // per-field view — the reader kept the claim and lost the proof.
    via: <ChangeGrounding change={change} />,
    detail: (
      <HistoryFieldDiff
        field={change.field}
        oldValue={change.old_value}
        newValue={change.new_value}
        values={valueCtx}
      />
    ),
  }));
}
