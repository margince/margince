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
import { ChangeList } from "../../components/changelist";
import { ReceiptReview } from "../../components/receiptreview";
import { Button } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { ENTITY, isEntityKind } from "../../lib/entity";
import { type Receipt, useHandledForYou } from "../../lib/handled";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { type MagicLine, magicKey } from "./magic.queries";
import { MagicUndoButton } from "./magic.undo";
import "../brief/brief.css";

type MagicLineRecord = components["schemas"]["MagicLineRecord"];

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
  const { locale } = useLocale();
  const entity = record.entity;
  const label = entity.label ?? t("magic.noRecord");
  const href = isEntityKind(entity.type)
    ? routeHash(ENTITY[entity.type].route(entity.id))
    : undefined;
  return (
    <li className="magic-record">
      <div className="magic-record-head">
        {href ? <a href={href}>{label}</a> : <span>{label}</span>}
        <RecordActions record={record} />
      </div>
      <time className="t-caption" dateTime={record.occurred_at}>
        {formatDateTime(record.occurred_at, locale, viewerZone())}
      </time>
      <ChangeList changes={record.changes} />
    </li>
  );
}

// A grouped change keeps the verbs its receipt has on its own. A reviewed deal
// change offers Accept or Keep date beside Undo; any other offers its restore.
function RecordActions({ record }: Readonly<{ record: MagicLineRecord }>) {
  const reviewed = useReviewedReceipt(record.receipt_id);
  if (reviewed) {
    return <ReceiptReview receipt={reviewed} />;
  }
  return (
    <MagicUndoButton
      undo={record.undo}
      entityType={record.entity.type}
      entityId={record.entity.id}
    />
  );
}

// Only off a read that answered now. A refetch that failed keeps the old list
// cached, and its review verbs may no longer be this reader's to press.
function useReviewedReceipt(id: string | undefined): Receipt | undefined {
  const handled = useHandledForYou();
  if (!id || !handled.isSuccess) {
    return undefined;
  }
  return handled.data.receipts?.find(
    (receipt) => receipt.id === id && receipt.review !== undefined,
  );
}
