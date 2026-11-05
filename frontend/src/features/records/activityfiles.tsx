// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { FileChip } from "../../design-system/filechip";
import { Heading } from "../../design-system/heading";
import { LoadMoreButton } from "../../design-system/querystates";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatBytes } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { activityFilesKey } from "../../lib/activitykeys";
import { useCanWriteRecord } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { AddDocumentDialog } from "../voice-knowledge/adddocument";
import "./activityfiles.css";

// The files filed on one activity: what a rep handed over in a meeting or
// attached when logging a call. The upload is the record libraries' own
// dialog, anchored on the activity, so limits and refusals read the same.

type Activity = components["schemas"]["Activity"];

const PAGE_LIMIT = 20;

/**
 * ActivityFiles lists the activity's files, each one opening as a FileChip
 * does, and offers the upload. The server scopes the list through the
 * activity, so a reader outside its audience is refused rather than shown
 * an empty list.
 */
export function ActivityFiles({
  activityId,
  writable,
  archived = false,
}: Readonly<{
  activityId: string;
  // The server's per-row answer from the activity read; absent reads as no.
  writable: boolean;
  // An archived activity takes no new file; the server refuses one.
  archived?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [adding, setAdding] = useState(false);
  const permitted = useCanWriteRecord("activity", { writable });
  const answered = useMe().data?.authorization !== undefined;
  const query = useInfiniteQuery({
    queryKey: activityFilesKey(activityId),
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/attachments", {
          params: {
            query: {
              entity_type: "activity",
              entity_id: activityId,
              limit: PAGE_LIMIT,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const files = query.data?.pages.flatMap((page) => page.data) ?? [];
  const hasMore = query.data?.pages.at(-1)?.page.has_more ?? false;
  let state: SectionState;
  if (query.isPending) {
    state = "loading";
  } else if (files.length === 0) {
    state = query.isError ? "failed" : "empty";
  } else {
    state = hasMore ? "partial" : "ready";
  }
  return (
    <div className="activity-files">
      <SurfaceState
        loadingLabel={t("files.title")}
        state={state}
        emptyLabel={t("activity.files.empty")}
        detail={
          state === "failed"
            ? { onRetry: () => void query.refetch() }
            : undefined
        }
      >
        <div className="activity-files-items">
          {files.map((file) => (
            <FileChip
              key={file.id}
              href={`/v1/attachments/${file.id}`}
              filename={file.filename}
              size={
                file.byte_size == null
                  ? undefined
                  : formatBytes(file.byte_size, locale)
              }
            />
          ))}
        </div>
      </SurfaceState>
      <LoadMoreButton query={query} />
      {!archived && (
        <>
          <Button
            variant="ghost"
            // Quiet until /me answers, so no refusal is claimed before the server decides.
            disabled={!answered}
            reason={
              answered && !permitted ? t("docs.add.errRefused") : undefined
            }
            onClick={() => setAdding(true)}
          >
            {t("docs.add.action")}
          </Button>
          <AddDocumentDialog
            anchor={{ record: "activity", id: activityId }}
            open={adding}
            onClose={() => setAdding(false)}
          />
        </>
      )}
    </div>
  );
}

// Captured mail and channel messages carry their files in their own drawer.
const CAPTURED_KINDS: ReadonlySet<string> = new Set(["email", "message"]);

/**
 * ActivityFilesAction is the Files verb on a history row. The list is read
 * only while the dialog is open, so a page of rows costs no request per row.
 */
export function ActivityFilesAction({
  activity,
}: Readonly<{ activity: Activity }>) {
  const t = useT();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  if (
    CAPTURED_KINDS.has(activity.kind) ||
    activity.content_state === "withheld"
  ) {
    return null;
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("files.title")}</Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent="form"
      >
        <Heading size="large" id={titleId} className="modal-title">
          {t("activity.files.title", {
            subject: activity.subject?.trim() || t("files.title"),
          })}
        </Heading>
        {open && (
          <ActivityFiles
            activityId={activity.id}
            writable={activity.writable ?? false}
            archived={Boolean(activity.archived_at)}
          />
        )}
      </Modal>
    </>
  );
}
