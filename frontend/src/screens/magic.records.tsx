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
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { Button } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { throwProblem } from "./common";
import { historyFieldLabel } from "./historyfieldlabels";
import { type MagicLine, magicKey } from "./magic.queries";
import { MagicUndoButton } from "./magic.undo";

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
      const { data, error } = await api.GET("/magic/lines/{id}/records", {
        params: {
          path: { id: lineId },
          query: { since, ...(pageParam ? { cursor: pageParam } : {}) },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}

/**
 * The line's About cell when it stands for more than one record: the summary,
 * and the control that opens the list.
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
      <Button onClick={() => setOpen(true)}>{summary}</Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        size="wide"
      >
        <Heading size="medium" as="h2" id={titleId}>
          {t("magic.records.title")}
        </Heading>
        {open && <LineRecordsList line={line} since={since} />}
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
  const href = isEntityKind(entity.type)
    ? routeHash(ENTITY[entity.type].route(entity.id))
    : undefined;
  return (
    <li className="magic-record">
      <div className="magic-record-head">
        {href ? <a href={href}>{label}</a> : <span>{label}</span>}
        <MagicUndoButton
          undo={record.undo}
          entityType={entity.type}
          entityId={entity.id}
        />
      </div>
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
      {changes.map((change) => (
        <div key={change.field}>
          <dt className="t-caption">{historyFieldLabel(change.field, t)}</dt>
          <dd>
            {t("magic.records.fromTo", {
              from: valueText(change.before, t),
              to: valueText(change.after, t),
            })}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// A stored value as text a reader recognises. Absent and empty read as "empty",
// a list as its items, anything structured as its JSON, all cut to a length.
function valueText(value: unknown, t: ReturnType<typeof useT>): string {
  if (value === null || value === undefined || value === "") {
    return t("magic.records.empty_value");
  }
  const text = Array.isArray(value)
    ? value.map((item) => String(item)).join(", ")
    : typeof value === "object"
      ? JSON.stringify(value)
      : String(value);
  return text.length > VALUE_CHARS ? `${text.slice(0, VALUE_CHARS)}…` : text;
}
