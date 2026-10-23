// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A done line, opened: every record it stands for, what changed on each, and
// an undo per record.
//
// "Changed industry, offer summary — GEM and 150 more" is the right summary and
// the wrong place to stop. A reader who disagrees with the machine has to see
// which records it changed, from what to what, and put back the ones it got
// wrong without opening 150 record pages.

import { useInfiniteQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Disclosure } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { ENTITY, isEntityKind } from "../../lib/entity";
import { historyFieldLabel } from "../../lib/historyfieldlabels";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { type MagicLine, magicKey } from "./magic.queries";
import { LineReview, useReceiptFor } from "./magic.review";
import { MagicUndoButton } from "./magic.undo";
import "../brief/brief.css";

type MagicLineRecord = components["schemas"]["MagicLineRecord"];
type MagicFieldChange = components["schemas"]["MagicFieldChange"];

// The longest value drawn in full. A company's offer summary can run to a
// paragraph, and the list is for recognising a change, not reading it.
const VALUE_CHARS = 140;

function useLineRecords(lineId: string, since: string, open: boolean) {
  return useInfiniteQuery({
    queryKey: [...magicKey, "line", lineId, since],
    enabled: open,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/magic/lines/{id}/records", {
          params: {
            path: { id: lineId },
            query: { since, ...(pageParam ? { cursor: pageParam } : {}) },
          },
        }),
      );
    },
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}

/**
 * A line's record count, as the control that opens every record it stands
 * for. A link rather than a boxed button: it is a way in, standing in the
 * line's text, and the one boxed verb a line carries is Undo.
 */
export function LineRecordsOpener({
  line,
  since,
  summary,
}: Readonly<{ line: MagicLine; since: string; summary: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  return (
    <>
      <Button variant="link" onClick={() => setOpen(true)}>
        {summary}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent="drawer"
      >
        <DrawerHead>
          <Heading size="large" id={titleId} className="t-h2 modal-title">
            {t("magic.records.title")}
          </Heading>
        </DrawerHead>
        <DrawerBody>
          {open && <LineRecordsList line={line} since={since} />}
        </DrawerBody>
      </Modal>
    </>
  );
}

function LineRecordsList({
  line,
  since,
}: Readonly<{ line: MagicLine; since: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const read = useLineRecords(line.id, since, true);
  const records = read.data?.pages.flatMap((page) => page.data) ?? [];
  const total = read.data?.pages[0]?.page.total;
  const state = read.isPending
    ? "loading"
    : read.isError
      ? "failed"
      : records.length === 0
        ? "empty"
        : ("ready" as const);
  return (
    <SurfaceState
      state={state}
      emptyLabel={t("magic.records.empty")}
      loadingLabel={t("magic.loading")}
      detail={{ onRetry: () => void read.refetch() }}
    >
      {total !== undefined && (
        <p className="t-caption">
          {plural("magic.records.count", total, {
            count: formatNumber(total, locale),
          })}
        </p>
      )}
      <ul className="magic-records">
        {records.map((record) => (
          <RecordRow key={record.audit_id} record={record} />
        ))}
      </ul>
      {read.hasNextPage && (
        <Button
          disabled={read.isFetchingNextPage}
          onClick={() => void read.fetchNextPage()}
        >
          {t("magic.records.more")}
        </Button>
      )}
    </SurfaceState>
  );
}

function RecordRow({ record }: Readonly<{ record: MagicLineRecord }>) {
  const t = useT();
  const entity = record.entity;
  const label = entity.label ?? t("magic.noRecord");
  // Each grouped change keeps its own native review, joined by its receipt.
  const review = useReceiptFor(
    record.receipt_id ? [record.receipt_id] : undefined,
    entity.type,
  );
  const href = isEntityKind(entity.type)
    ? routeHash(ENTITY[entity.type].route(entity.id))
    : undefined;
  return (
    <li className="magic-record">
      <div className="magic-record-head">
        {href ? <a href={href}>{label}</a> : <span>{label}</span>}
        {review.receipt ? (
          <LineReview receipt={review.receipt} />
        ) : (
          <MagicUndoButton
            undo={record.undo}
            entityType={entity.type}
            entityId={entity.id}
          />
        )}
      </div>
      {review.unavailable && (
        <p className="t-caption">{t("magic.review.unavailable")}</p>
      )}
      <ChangeList changes={record.changes} />
    </li>
  );
}

/** Every field the change moved, old value to new. */
export function ChangeList({
  changes,
}: Readonly<{ changes: readonly MagicFieldChange[] }>) {
  const t = useT();
  if (changes.length === 0) {
    return null;
  }
  return (
    <dl className="magic-changes">
      {changes.map((change) => {
        const from = valueText(change.before, t);
        const to = valueText(change.after, t);
        return (
          <div key={change.field}>
            <dt className="t-caption">{historyFieldLabel(change.field, t)}</dt>
            <dd>
              {t("magic.records.fromTo", {
                from: compact(from),
                to: compact(to),
              })}
              {/* Two values may differ only past the cut; the reader can
                  still read both in full before pressing Undo. */}
              {(from.length > VALUE_CHARS || to.length > VALUE_CHARS) && (
                <Disclosure summary={t("magic.records.fullValues")}>
                  <p>{t("magic.records.fromTo", { from, to })}</p>
                </Disclosure>
              )}
            </dd>
          </div>
        );
      })}
    </dl>
  );
}

// A stored value as text a reader recognises. Absent and empty read as "empty",
// a list as its items, anything structured as its JSON.
function valueText(value: unknown, t: ReturnType<typeof useT>): string {
  if (value === null || value === undefined || value === "") {
    return t("magic.records.empty_value");
  }
  const text = Array.isArray(value)
    ? value.map((item) => String(item)).join(", ")
    : typeof value === "object"
      ? JSON.stringify(value)
      : String(value);
  return text;
}

// A value cut to the row's length; the full one sits behind the row.
function compact(text: string): string {
  return text.length > VALUE_CHARS ? `${text.slice(0, VALUE_CHARS)}…` : text;
}
