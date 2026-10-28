// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { FileText, Mail, MessageCircle } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar } from "../design-system/atoms";
import { Panel } from "../design-system/panel";
import { activityAgo } from "../format/activitywhen";
import { emailSummaryText } from "../format/emailtext";
import { useNow } from "../format/now";
import { type Translator, useLocale, useT } from "../i18n";
import { timelineBaseKey } from "../lib/activitykeys";
import type { EntityKind } from "../lib/entity";
import {
  JUST_IN_DAYS,
  type LatestInbound,
  type LatestInboundKind,
  latestInbound,
} from "../lib/latestinbound";
import { unwrap } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import "./latestinbound.css";

type Activity = components["schemas"]["Activity"];

// The newest form among the recent rows is what is looked for. A record
// rarely has more than a page of activity in a week.
const RECENT_LIMIT = 50;
const DAY_MS = 86_400_000;

/**
 * The newest thing they sent on this record that nobody has answered, from
 * the last `JUST_IN_DAYS`. Keyed under the record's timeline key, so every
 * write that refreshes the timeline (a reply sent, a call logged) refreshes
 * this too.
 */
export function useLatestInbound(
  entityType: EntityKind,
  entityId: string,
): LatestInbound | undefined {
  // Fixed for the mount: a window that moved every render would be a new
  // query key every render.
  const [since] = useState(() =>
    new Date(Date.now() - JUST_IN_DAYS * DAY_MS).toISOString(),
  );
  const base = timelineBaseKey(entityType, entityId);
  const scope = { entity_type: entityType, entity_id: entityId };
  const waiting = useQuery({
    queryKey: [...base, "latest-inbound", "waiting", since],
    queryFn: async () =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              ...scope,
              waiting_reply: true,
              occurred_after: since,
              limit: 1,
            },
          },
        }),
      ),
  });
  const recent = useQuery({
    queryKey: [...base, "latest-inbound", "recent", since],
    queryFn: async () =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: { ...scope, occurred_after: since, limit: RECENT_LIMIT },
          },
        }),
      ),
  });
  // Nothing until both reads agree: a form drawn before the read that would
  // show it answered is a panel that appears and vanishes.
  if (!waiting.isSuccess || !recent.isSuccess) {
    return undefined;
  }
  return latestInbound(waiting.data.data?.[0], recent.data.data ?? []);
}

const KIND_ICON: Record<LatestInboundKind, ReactNode> = {
  email: <Mail aria-hidden="true" />,
  message: <MessageCircle aria-hidden="true" />,
  web_form: <FileText aria-hidden="true" />,
};

function kindLabel(kind: LatestInboundKind, t: Translator): string {
  switch (kind) {
    case "email":
      return t("latest.kind.email");
    case "message":
      return t("latest.kind.message");
    case "web_form":
      return t("latest.kind.webForm");
  }
}

// The words a reader is shown for the item: what it is about, and the
// sender's own text. A withheld row has neither, and says only what it is.
function itemWords(activity: Activity): {
  subject?: string;
  excerpt?: string;
} {
  if (activity.content_state === "withheld") {
    return {};
  }
  const summary = activity.email_summary;
  const subject = (summary?.subject ?? activity.subject)?.trim() || undefined;
  const raw = activity.body ?? "";
  const excerpt =
    summary?.preview ??
    (activity.kind === "email" ? emailSummaryText(raw) : raw.trim());
  return { subject, excerpt: excerpt || undefined };
}

function When({ activity }: Readonly<{ activity: Activity }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  // A minute is the finest unit the label shows.
  const now = useNow(60_000);
  const moment = {
    at: activity.occurred_at,
    precision: activity.occurred_precision,
  };
  return (
    <time dateTime={activity.occurred_at} className="latest-when">
      {activityAgo(moment, now, t, locale, zone)}
    </time>
  );
}

/**
 * "Just in": the newest unanswered thing they sent, as the lead panel at the
 * top of a record's Overview. `actions` draws the verbs for it, the same ones
 * the record's timeline offers on that row. So the panel and the row cannot
 * disagree about what may be done with it.
 */
export function LatestInboundPanel({
  latest,
  actions,
}: Readonly<{
  latest: LatestInbound | undefined;
  actions?: (activity: Activity) => ReactNode;
}>) {
  const t = useT();
  if (!latest) {
    return null;
  }
  const { activity, kind } = latest;
  const { subject, excerpt } = itemWords(activity);
  const sender = activity.email_summary?.counterparty?.trim();
  const senderId = activity.email_summary?.counterparty_contact_id;
  return (
    <Panel
      tone="accent"
      title={
        <span className="latest-title">
          {KIND_ICON[kind]}
          {kindLabel(kind, t)}
          <When activity={activity} />
        </span>
      }
      actions={actions?.(activity)}
    >
      <div className="latest-body">
        {sender && (
          <span className="latest-sender">
            <Avatar name={sender} identity={senderId ?? sender} />
            {sender}
          </span>
        )}
        {subject && <p className="latest-subject">{subject}</p>}
        {excerpt && excerpt !== subject && (
          <p
            className={
              kind === "web_form"
                ? "latest-excerpt latest-form"
                : "latest-excerpt"
            }
          >
            {excerpt}
          </p>
        )}
      </div>
    </Panel>
  );
}

/**
 * The same item as one line in the record's head, for the tabs that do not
 * draw the panel. Pressing it goes back to the Overview, where the panel is.
 */
export function LatestInboundLine({
  latest,
  onOpen,
}: Readonly<{
  latest: LatestInbound | undefined;
  onOpen: () => void;
}>) {
  const t = useT();
  if (!latest) {
    return null;
  }
  const { activity, kind } = latest;
  const { subject, excerpt } = itemWords(activity);
  return (
    <button
      type="button"
      className="latest-line"
      onClick={onOpen}
      aria-label={t("latest.line.open", { kind: kindLabel(kind, t) })}
    >
      <span className="latest-line-mark">{KIND_ICON[kind]}</span>
      <span className="latest-line-text">
        <span className="latest-line-kind">
          {kindLabel(kind, t)} · <When activity={activity} />
        </span>
        {(subject ?? excerpt) && (
          <span className="latest-line-subject">{subject ?? excerpt}</span>
        )}
      </span>
    </button>
  );
}
