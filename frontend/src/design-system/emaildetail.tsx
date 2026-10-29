// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useId, useRef, useState } from "react";

import type { components } from "../api/schema";
import type { ActivityWhenFormatter } from "../format/activitywhen";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { Badge, Modal } from "./atoms";
import { DrawerBody, DrawerHead } from "./drawerbands";
import { moveLabel } from "./emailentry";
import { emailDetailKey, readEmailPresentation } from "./emailmessage";
import { EmailThread, threadOrder } from "./emailthread";
import { Heading } from "./heading";
import type { DrawerSpan } from "./modal";
import { SurfaceState } from "./surfacestate";
import "./emaildetail.css";

// The drawer keeps the originating record visible and fetches only when opened.

type EmailPresentation = components["schemas"]["EmailPresentation"];

/**
 * One email's thread, read whole. The drawer makes its own read: what it may
 * show is an authorization result only the server's answer knows.
 *
 * What needs another API call arrives as a render prop: record names, the
 * audience editor and each message's actions.
 */
export function EmailDetail({
  activityId,
  open = true,
  onClose,
  formatWhen,
  renderAccess,
  renderRecords,
  renderActions,
  renderRelated,
  notice,
}: Readonly<{
  activityId: string;
  /**
   * Whether the drawer is showing. It stays mounted while shut, so `Modal` can
   * animate it out and the next open asks again. Defaults to open.
   */
  open?: boolean;
  onClose: () => void;
  /** The caller owns the reader's timezone, so it owns the formatting. */
  formatWhen: ActivityWhenFormatter;
  /**
   * Who reads this thread, and the control to change it, under the subject.
   *
   * Passed in because the editor performs writes. A design-system component
   * importing the audience service would turn the catalog into a layer that
   * talks to the API. Absent means no region, never an empty one.
   */
  renderAccess?: (presentation: EmailPresentation) => ReactNode;
  /**
   * The records a message is filed against, NAMED. Drawn on the header line
   * and on each message's envelope. Passed in because `links` carries ids and
   * no names, so naming one is a record read per link.
   */
  renderRecords?: (presentation: EmailPresentation) => ReactNode;
  /**
   * The actions under each expanded message: reply, forward and the rest.
   * Passed in because each one is a write the app performs.
   */
  renderActions?: (presentation: EmailPresentation) => ReactNode;
  renderRelated?: () => ReactNode;
  /** A status above the thread, such as confirmation that a reply was added. */
  notice?: string;
}>) {
  const t = useT();
  // Generated rather than fixed: two drawers mounted at once would otherwise
  // share an id.
  const titleId = useId();
  const [span, setSpan] = useState<DrawerSpan>("intent");
  // A FRESH KEY PER OPEN, which is what makes each open a fresh ask.
  const opens = useRef(0);
  const wasOpen = useRef(false);
  if (open && !wasOpen.current) {
    opens.current += 1;
  }
  wasOpen.current = open;
  const read = useQuery({
    queryKey: [...emailDetailKey(activityId), opens.current],
    // A message's content is an authorization result, so ask on every open
    // and keep nothing. The per-open segment stops the next open painting this
    // answer; it is last, so `emailDetailKey`'s prefix still invalidates it.
    enabled: open,
    staleTime: 0,
    gcTime: 0,
    queryFn: () => readEmailPresentation(activityId),
  });

  // The status decides before the subject, so a withheld message never shows
  // a subject a careless response still carried.
  const title = !read.data
    ? t("email.detail.none")
    : read.data.access.content_state === "withheld"
      ? t("email.withheldSubject")
      : read.data.summary.subject?.trim() || t("email.noSubject");

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
      span={span}
      onSpanChange={setSpan}
    >
      <DrawerHead className="emaildetail__head">
        <Heading size="large" id={titleId} className="emaildetail__title">
          {title}
        </Heading>
        {read.data && (
          <ThreadFacts presentation={read.data} renderRecords={renderRecords} />
        )}
        {/* Who may read the thread, under its subject: a reader wants a limit
            before they read, not after. */}
        {read.data && renderAccess?.(read.data)}
      </DrawerHead>
      <DrawerBody>
        <ThreadState
          read={read}
          formatWhen={formatWhen}
          renderRecords={renderRecords}
          renderActions={renderActions}
          related={renderRelated}
          notice={notice}
        />
      </DrawerBody>
    </Modal>
  );
}

// The drawer's body: the thread once it is read, or the state of the read.
function ThreadState({
  read,
  formatWhen,
  renderRecords,
  renderActions,
  related,
  notice,
}: Readonly<{
  read: Readonly<{
    data: EmailPresentation | undefined;
    isPending: boolean;
    isError: boolean;
    refetch: () => unknown;
  }>;
  formatWhen: ActivityWhenFormatter;
  renderRecords?: (presentation: EmailPresentation) => ReactNode;
  renderActions?: (presentation: EmailPresentation) => ReactNode;
  related?: () => ReactNode;
  notice?: string;
}>) {
  const t = useT();
  const labels = {
    emptyLabel: t("email.detail.none"),
    loadingLabel: t("email.detail.loading"),
  };
  if (read.isPending) {
    return (
      <SurfaceState state="loading" {...labels}>
        {null}
      </SurfaceState>
    );
  }
  if (read.isError || !read.data) {
    // `failed` with a retry: a failure with nothing to press is the same as
    // being told the message is not there.
    return (
      <SurfaceState
        state="failed"
        {...labels}
        detail={{ onRetry: () => void read.refetch() }}
      >
        {null}
      </SurfaceState>
    );
  }
  if (read.data.access.content_state === "withheld") {
    // The row stays, its words do not, and the reason does not either: why a
    // message is limited describes what it is about.
    return (
      <SurfaceState
        state="withheld"
        {...labels}
        detail={{ withheldReason: t("email.detail.withheldReason") }}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <EmailThread
      key={read.data.id}
      presentation={read.data}
      formatWhen={formatWhen}
      renderRecords={renderRecords}
      renderActions={renderActions}
      related={related}
      notice={notice}
    />
  );
}

/**
 * The line under the subject: whether the thread waits on the reader, how
 * many messages it holds, and what it is filed under. A withheld message
 * shares none of it.
 */
function ThreadFacts({
  presentation,
  renderRecords,
}: Readonly<{
  presentation: EmailPresentation;
  renderRecords?: (presentation: EmailPresentation) => ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (presentation.access.content_state === "withheld") {
    return null;
  }
  const shown = threadOrder(presentation).length;
  const count = formatNumber(shown, locale);
  const move = moveLabel(presentation.summary, false);
  // A thread with more pages than the read carried is counted as what is shown.
  const counted = plural(
    presentation.thread?.next_cursor
      ? "email.thread.countLatest"
      : "email.thread.count",
    shown,
    { count },
  );
  return (
    <p className="emaildetail__facts t-caption">
      {move && (
        <Badge
          tone={
            presentation.summary.move === "needs_reply" ? "warning" : undefined
          }
        >
          {t(move)}
        </Badge>
      )}
      <span>{counted}</span>
      {renderRecords && presentation.links.length > 0 && (
        <span className="emaildetail__filed">
          {t("email.detail.filedUnder")} {renderRecords(presentation)}
        </span>
      )}
    </p>
  );
}
