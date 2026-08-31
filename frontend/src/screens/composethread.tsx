// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The conversation a reply is answering, beside the reply.
 *
 * The composer used to say what it was answering in one sentence and leave the
 * messages themselves on the page behind — which the dialog was covering. A rep
 * checking what was actually promised had to send or discard first. This puts
 * the thread in the drawer, so the reply is written next to the words it
 * answers rather than over them.
 *
 * It renders through the timeline's own row: the conversation IS the record's
 * chronology narrowed to one thread, and a second message row would be a second
 * answer to "how does a message look" — including how a message nobody in this
 * audience may read looks, which the timeline already draws.
 */

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { activityTimeline } from "../design-system/activitytimeline";
import { PendingBody } from "../design-system/atoms";
import { TimelineRow } from "../design-system/composed";
import type { NameOf } from "../design-system/participants";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import "./composethread.css";

type Activity = components["schemas"]["Activity"];

// How much of a long conversation to carry. A reply is written against the
// recent turns; a thread that has run for a year does not need its first
// message in the drawer, and fetching all of it costs the composer's open.
const SHOWN = 12;

/**
 * The messages of the anchor's thread, newest first.
 *
 * A one-off mail carries no `thread_key` — capture assigns one only where a
 * provider reported a conversation — and that is not an empty thread: it is a
 * conversation of exactly one message, which is the anchor already in hand. So
 * the list read is skipped rather than run with an empty key, which would ask
 * the server for every activity that has no thread.
 */
export function useThreadMessages(anchor: Activity | undefined): {
  messages: Activity[];
  pending: boolean;
} {
  const threadKey = anchor?.thread_key ?? undefined;
  const query = useQuery({
    queryKey: ["compose-thread", threadKey],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: { query: { thread_key: threadKey, limit: SHOWN } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: Boolean(threadKey),
  });
  if (!anchor) {
    return { messages: [], pending: false };
  }
  if (!threadKey) {
    return { messages: [anchor], pending: false };
  }
  // A read that has not answered yet must not render as a thread of one: the
  // anchor alone looks like a settled answer ("this conversation is a single
  // message") rather than like a list still arriving.
  return {
    messages: query.data?.data ?? [],
    pending: query.isPending,
  };
}

/**
 * The pane. Newest first, matching the chronology it is drawn from — a reply
 * is written against the last thing said, and that belongs at the top where
 * the reader starts.
 */
export function ThreadPane({
  messages,
  pending,
  viewerUserId,
  nameOf,
}: Readonly<{
  messages: readonly Activity[];
  pending: boolean;
  viewerUserId?: string;
  nameOf: NameOf;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // No `renderActions`: every row's affordance here would be Reply, inside the
  // reply. The rows are the reading, not a place to act from.
  const entries = activityTimeline([...messages], viewerUserId, undefined, {
    nameOf,
    t,
    locale,
  });
  return (
    <section className="compose-thread" aria-labelledby="compose-thread-head">
      <h3 id="compose-thread-head" className="compose-thread-head t-eyebrow">
        {t("compose.threadHeading")}
      </h3>
      <div className="compose-thread-scroll">
        {pending ? (
          <PendingBody label={t("compose.threadPending")} lines={3} />
        ) : (
          <ul className="timeline compose-thread-list">
            {entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={zone} />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
