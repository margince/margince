import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import type { EntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { Button, Card, EmptyState } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ProvenanceTag } from "../design-system/trust";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  LoadMoreButton,
  problemCodeOf,
  problemMessageOf,
  QueryStates,
  throwProblem,
  useViewerId,
} from "./common";
import {
  type EntryFieldChange,
  entryFieldChanges,
  provenanceOfEntry,
} from "./history.logic";
import { HistoryEdgeDetail } from "./historyedge";
import { HistoryFieldDiff } from "./historyfielddiff";
import { historyFieldLabel } from "./historyfieldlabels";
import { historyRows } from "./historyreversal";
import { actorName, ReversalPairRow } from "./historyreversalrow";
import { undoRefusalKey, VERSION_SKEW_CODE } from "./historyundo";
import { type HistoryValueCtx, historyValue } from "./historyvalues";
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
  changes,
  currency,
}: Readonly<{
  changes: readonly EntryFieldChange[];
  currency: string | null | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  if (changes.length === 0) {
    return null;
  }
  const valueCtx: HistoryValueCtx = { currency, locale, zone: recordZone };
  return (
    <ul className="entry-fields">
      {changes.map((change) => (
        <li key={change.field} className="entry-field">
          <span className="entry-field-name">
            {historyFieldLabel(change.field, t)}
          </span>
          <HistoryFieldDiff
            field={change.field}
            oldValue={change.oldValue}
            newValue={change.newValue}
            values={valueCtx}
          />
        </li>
      ))}
    </ul>
  );
}

// The variables one press carries. A mutationFn takes what it needs rather
// than closing over render state: the click belongs to the committed render,
// so what it passes cannot be older than the control that carried it.
type RestorePress = Readonly<{
  kind: EntityKind;
  id: string;
  auditId: string;
  version: number;
}>;

// What a change put back needs from the record it belongs to.
export type RecordRestore = Readonly<{
  // The last-seen version, which the restore pins with If-Match. Undefined is
  // a caller holding no claim about what it would overwrite, and the button is
  // not offered: last-write-wins is not something this control may choose.
  version: number | undefined;
  // Re-read the record. The history's own queries are this panel's to
  // invalidate; the record the change belongs to is the caller's, and the two
  // must arrive together or the new `restore` entry describes a record on
  // screen that does not yet show it.
  onRestored: () => void;
}>;

// A refusal in the reader's own words.
//
// The SAME words in both places: shown up front under a button nobody may
// press, and after a press the server refused with the same code. A refusal
// found at press time and one shown in advance must not read as two different
// products, so there is one sentence per reason and both readers ask for it
// here.
function refusalSentence(
  reason: string | null | undefined,
  detail: string | null | undefined,
  t: (key: MessageKey) => string,
): string | undefined {
  const key = undoRefusalKey(reason);
  if (key) {
    return t(key);
  }
  // A server naming a reason this build predates. Its own detail is the only
  // honest wording left — inventing one here would describe a case nobody has
  // written down.
  return detail ?? undefined;
}

function UndoButton({
  entry,
  kind,
  id,
  changes,
  currency,
  restore,
  // The verb this press means. One engine, two intents: putting a change back
  // and putting a REVERSAL back are the same write, and the second one redoes
  // what the first undid — so the label is the caller's to name, and the two
  // must never read the same on adjacent rows.
  label = "history.undo.action",
}: Readonly<{
  entry: AuditHistoryEntry;
  kind: EntityKind;
  id: string;
  changes: readonly EntryFieldChange[];
  currency: string | null | undefined;
  restore: RecordRestore;
  label?: MessageKey;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const client = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  // What the server said when it refused the press. Held apart from the
  // advisory answer below because the two can differ: the read cannot hold a
  // lock, so a change that looked restorable a moment ago may not be one now.
  const [refused, setRefused] = useState<string | null>(null);

  const putBack = useMutation({
    mutationFn: async ({
      kind: pressedKind,
      id: pressedId,
      auditId,
      version,
    }: RestorePress) => {
      const { data, error } = await api.POST(
        "/records/{entity_type}/{id}/history/{audit_id}/restore",
        {
          params: {
            path: {
              entity_type: pressedKind,
              id: pressedId,
              audit_id: auditId,
            },
            ...ifMatch(version),
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setRefused(null);
      setConfirming(false);
      client.invalidateQueries({ queryKey: ["record-history", kind, id] });
      client.invalidateQueries({ queryKey: ["field-history", kind, id] });
      restore.onRestored();
    },
    onError: (error) => {
      const code = problemCodeOf(error);
      if (code === VERSION_SKEW_CODE) {
        // The record moved rather than the change being unrestorable. Re-read
        // both, so the reader decides against what the record says NOW.
        client.invalidateQueries({ queryKey: ["record-history", kind, id] });
        client.invalidateQueries({ queryKey: ["field-history", kind, id] });
        restore.onRestored();
        setRefused(t("history.undo.versionSkew"));
        setConfirming(false);
        return;
      }
      setRefused(refusalSentence(code, null, t) ?? problemMessageOf(error, t));
      setConfirming(false);
    },
  });

  const advisory = entry.undoable;
  const version = restore.version;
  // Nothing to offer: an installation whose read does not answer the question,
  // or a caller with no version to pin the write against.
  if (!advisory || version === undefined) {
    return null;
  }

  const upFront = advisory.undoable
    ? undefined
    : (refusalSentence(advisory.reason, advisory.detail, t) ??
      t("common.errorNoCause"));
  const press = () => {
    // Two presses get confirmed, for the same reason: the reader is told what
    // the write will do before it lands on somebody else's data.
    //
    // More than one field moves — they are told which, and what each goes back
    // to. And ANY link change, because reversing one removes or re-points a
    // connection between two records rather than editing a value on this one.
    // An edge entry carries no field changes at all, so the field count alone
    // would wave through the more consequential of the two.
    if (changes.length > 1 || entry.edge) {
      setConfirming(true);
      return;
    }
    putBack.mutate({ kind, id, auditId: entry.id, version });
  };

  return (
    <span className="entry-undo">
      <Button
        small
        variant="ghost"
        reason={upFront}
        pending={putBack.isPending}
        busyLabel={t("history.undo.busy")}
        onClick={press}
      >
        {t(label)}
      </Button>
      {refused && <span className="entry-undo-refusal">{refused}</span>}
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("history.undo.confirmTitle")}
        confirmLabel={t(label)}
        pending={putBack.isPending}
        onConfirm={() =>
          putBack.mutate({ kind, id, auditId: entry.id, version })
        }
      >
        <p>
          {entry.edge
            ? t("history.undo.confirmEdgeBody", {
                other: entry.edge.other_label ?? t("ref.nameLoadFailed"),
              })
            : t("history.undo.confirmBody", {
                count: formatNumber(changes.length, locale),
              })}
        </p>
        <ul className="entry-fields">
          {changes.map((change) => (
            <li key={change.field} className="entry-field">
              <span className="entry-field-name">
                {historyFieldLabel(change.field, t)}
              </span>
              <span>
                {historyValue(change.field, change.oldValue, {
                  currency,
                  locale,
                  zone: recordZone,
                }) ?? t("history.cleared")}
              </span>
            </li>
          ))}
        </ul>
      </ConfirmModal>
    </span>
  );
}

function HistoryEntryRow({
  entry,
  kind,
  id,
  locale,
  currency,
  restore,
  note,
  undoLabel,
}: Readonly<{
  entry: AuditHistoryEntry;
  kind: EntityKind;
  id: string;
  locale: ReturnType<typeof useLocale>["locale"];
  currency: string | null | undefined;
  restore: RecordRestore | undefined;
  // What this row is to the rows around it, when the sentence the server wrote
  // cannot say it: that a reversal undid something older than this page, or
  // that the change on this row has since been undone by the row above it.
  note?: string;
  undoLabel?: MessageKey;
}>) {
  const viewerId = useViewerId();
  const recordZone = useRecordZone();
  // An edge entry's images are the LINK's columns, not this record's, so it has
  // no field changes to draw or to name in a confirm — the sentence the read
  // wrote and the edge block below are the whole of what this row says.
  const edge = entry.edge;
  const changes = entryFieldChanges(entry);
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
          {note && <span className="entry-note">{note}</span>}
        </span>
        {edge ? (
          <HistoryEdgeDetail edge={edge} />
        ) : (
          <EntryFieldDetail changes={changes} currency={currency} />
        )}
        {restore && (
          <UndoButton
            entry={entry}
            kind={kind}
            id={id}
            changes={changes}
            currency={currency}
            restore={restore}
            label={undoLabel}
          />
        )}
      </span>
    </li>
  );
}

export function RecordHistory({
  kind,
  id,
  currency,
  restore,
}: Readonly<{
  kind: EntityKind;
  id: string;
  // The record's ISO currency, which is what gives a minor-unit column in the
  // detail below a row its scale.
  currency?: string | null;
  // What a change put back needs from the record. Absent on a surface that
  // reads the history without holding the record — no version to pin the
  // write with, so no button.
  restore?: RecordRestore;
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
          {historyRows(entries).map((row) =>
            row.kind === "pair" ? (
              <ReversalPairRow
                key={row.reversal.id}
                row={row}
                currency={currency}
              >
                <HistoryEntryRow
                  entry={row.reversal}
                  kind={kind}
                  id={id}
                  locale={locale}
                  currency={currency}
                  restore={restore}
                  undoLabel="history.undo.redo"
                />
                <HistoryEntryRow
                  entry={row.reversed}
                  kind={kind}
                  id={id}
                  locale={locale}
                  currency={currency}
                  restore={restore}
                  note={t("history.reversal.undoneBy", {
                    undoer: actorName(row.reversal.actor_name, t),
                  })}
                />
              </ReversalPairRow>
            ) : (
              <HistoryEntryRow
                key={row.entry.id}
                entry={row.entry}
                kind={kind}
                id={id}
                locale={locale}
                currency={currency}
                restore={restore}
                note={
                  row.kind === "unpairedReversal"
                    ? t("history.reversal.unpaired")
                    : undefined
                }
                undoLabel={
                  row.kind === "unpairedReversal"
                    ? "history.undo.redo"
                    : undefined
                }
              />
            ),
          )}
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
