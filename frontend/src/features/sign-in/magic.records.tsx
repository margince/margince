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
import { Button } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { searchActivityRoute, searchEmailRoute } from "../../lib/activityroute";
import { ENTITY, isEntityKind } from "../../lib/entity";
import { FACT_FIELD_LABELS } from "../../lib/factview";
import { historyFieldLabel } from "../../lib/historyfieldlabels";
import {
  type HistoryValueCtx,
  historyValue,
  isMinorUnitField,
} from "../../lib/historyvalues";
import { useMemberNames } from "../../lib/membernames";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate, routeHash } from "../../lib/router";
import { isUuid } from "../../lib/uuid";
import { type MagicLine, magicKey } from "./magic.queries";
import { MagicUndoButton } from "./magic.undo";
import "../brief/brief.css";

type MagicLineRecord = components["schemas"]["MagicLineRecord"];
type MagicFieldChange = components["schemas"]["MagicFieldChange"];
const factFieldLabels = new Map(Object.entries(FACT_FIELD_LABELS));

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
          {open && (
            <LineRecordsList
              line={line}
              since={since}
              onOpenRecord={() => setOpen(false)}
            />
          )}
        </DrawerBody>
      </Modal>
    </>
  );
}

function LineRecordsList({
  line,
  since,
  onOpenRecord,
}: Readonly<{ line: MagicLine; since: string; onOpenRecord: () => void }>) {
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
          <RecordRow
            key={record.audit_id}
            record={record}
            onOpen={onOpenRecord}
          />
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

function RecordRow({
  record,
  onOpen,
}: Readonly<{ record: MagicLineRecord; onOpen: () => void }>) {
  const t = useT();
  const entity = record.entity;
  const label = entity.label ?? t("magic.noRecord");
  const href = isEntityKind(entity.type)
    ? routeHash(ENTITY[entity.type].route(entity.id))
    : undefined;
  return (
    <li className="magic-record">
      <div className="magic-record-head">
        {href ? (
          <a href={href} onClick={onOpen}>
            {label}
          </a>
        ) : entity.type === "activity" ? (
          <ActivityRecordLink id={entity.id} label={label} onOpen={onOpen} />
        ) : (
          <span>{label}</span>
        )}
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
  const { locale } = useLocale();
  const zone = useRecordZone();
  const members = useMemberNames(
    changes
      .filter(
        (change) =>
          change.field === "owner_id" || change.field === "assignee_id",
      )
      .flatMap((change) => [change.before, change.after])
      .filter(
        (value): value is string => typeof value === "string" && isUuid(value),
      ),
  );
  const currency = changes.find((change) => change.field === "currency");
  const values = (value: unknown): HistoryValueCtx => ({
    currency: typeof value === "string" ? value : undefined,
    locale,
    zone,
    nameOf: (id) => members.get(id) ?? t("approval.preview.nameUnavailable"),
  });
  if (changes.length === 0) {
    return null;
  }
  return (
    <dl className="magic-changes">
      {changes.map((change) => (
        <div key={JSON.stringify([change.field, change.before, change.after])}>
          <dt className="t-caption">
            {change.field === "archived_at" && change.after
              ? t("record.archived")
              : fieldLabel(change.field, t)}
          </dt>
          <dd>
            {change.field === "archived_at" && change.after
              ? valueText(
                  change.field,
                  change.after,
                  values(currency?.after),
                  t,
                )
              : t("magic.records.fromTo", {
                  from: valueText(
                    change.field,
                    change.before,
                    values(currency?.before),
                    t,
                  ),
                  to: valueText(
                    change.field,
                    change.after,
                    values(currency?.after),
                    t,
                  ),
                })}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function valueText(
  field: string,
  value: unknown,
  context: HistoryValueCtx,
  t: ReturnType<typeof useT>,
): string {
  if (value === null || value === undefined || value === "")
    return t("magic.records.empty_value");
  const text =
    typeof value === "object"
      ? JSON.stringify(
          value,
          (_key, entry: unknown) =>
            typeof entry === "string" && isUuid(entry)
              ? context.nameOf?.(entry)
              : entry,
          2,
        )
      : String(value);
  if (
    isMinorUnitField(field) &&
    !context.currency &&
    Number.isInteger(Number(text))
  ) {
    return t("magic.records.minorUnitsUnknown", {
      amount: formatNumber(Number(text), context.locale),
    });
  }
  return historyValue(field, text, context) ?? t("magic.records.empty_value");
}

function fieldLabel(field: string, t: ReturnType<typeof useT>): string {
  const key = factFieldLabels.get(field);
  return key ? t(key) : historyFieldLabel(field, t);
}

function ActivityRecordLink({
  id,
  label,
  onOpen,
}: Readonly<{ id: string; label: string; onOpen: () => void }>) {
  const t = useT();
  const read = useActivity(id, false);
  const [error, setError] = useState<string | null>(null);
  const [opening, setOpening] = useState(false);
  async function open() {
    setOpening(true);
    setError(null);
    const result = await read.refetch();
    setOpening(false);
    if (result.error) {
      setError(problemMessageOf(result.error, t));
      return;
    }
    if (!result.data) return;
    const route =
      result.data.kind === "email" ? searchEmailRoute : searchActivityRoute;
    onOpen();
    navigate(route(result.data.subject?.trim() || label, id));
  }
  return (
    <div>
      <Button variant="link" disabled={opening} onClick={() => void open()}>
        {label}
      </Button>
      {error && <p role="status">{error}</p>}
    </div>
  );
}
