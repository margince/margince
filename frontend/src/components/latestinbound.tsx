// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { FileText, Mail, MessageCircle } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { markdownLines } from "../design-system/markdown-parse";
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
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import "./latestinbound.css";

type Activity = components["schemas"]["Activity"];

// The newest form among the recent rows is what is looked for. A record
// rarely has more than a page of activity in a week.
const RECENT_LIMIT = 50;
// Enough waiting threads to pass over the ones a reader set aside.
const WAITING_LIMIT = 10;
const DAY_MS = 86_400_000;

/** The item as a page holds it: `notice` sets it aside on this browser. */
export type JustIn = LatestInbound & Readonly<{ notice?: () => void }>;

function noticedKey(activityId: string) {
  return { family: STORAGE_KEYS.latestInboundNoticed, member: activityId };
}

/**
 * The newest thing they sent on this record that nobody has answered, from
 * the last `JUST_IN_DAYS`, unless the reader pressed "Noticed" on it. Keyed
 * under the record's timeline key, so every write that refreshes the timeline
 * (a reply sent, a call logged) refreshes this too.
 */
export function useLatestInbound(
  entityType: EntityKind,
  entityId: string,
): JustIn | undefined {
  // The ids set aside since mount; storage alone would not draw the page again.
  const [noticed, setNoticed] = useState<ReadonlySet<string>>(new Set());
  // Fixed for the mount: a window that moved every render would be a new
  // query key every render. `now` ages rows out of it while the page stays open.
  const now = useNow(60_000);
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
              limit: WAITING_LIMIT,
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
  const cutoff = now - JUST_IN_DAYS * DAY_MS;
  const latest = latestInbound(
    waiting.data.data ?? [],
    recent.data.data ?? [],
    (row) =>
      Date.parse(row.occurred_at) < cutoff ||
      noticed.has(row.id) ||
      readStored(noticedKey(row.id)) !== null,
  );
  if (!latest) {
    return undefined;
  }
  const id = latest.activity.id;
  const notice = () => {
    writeStored(noticedKey(id), "1");
    setNoticed((ids) => new Set(ids).add(id));
  };
  return { ...latest, notice };
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
  const excerpt = summary?.preview ?? excerptOf(activity.kind, raw);
  return { subject, excerpt: excerpt || undefined };
}

// A form's note is markdown with one answer per line, read the way the
// timeline renders it so its quote marks never show as text. A chat is plain
// text, where a leading ">" is the sender's own character.
function excerptOf(kind: Activity["kind"], text: string): string {
  if (kind === "email") return emailSummaryText(text);
  const lines =
    kind === "note"
      ? markdownLines(text)
      : text.split("\n").map((line) => line.trim());
  return lines.filter(Boolean).join(" · ");
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
 * "Just in": the newest unanswered thing they sent, as one row at the top of
 * a record's Overview. It is laid out like the Next bar under it. `actions` draws
 * the verbs the record's timeline offers on that row, so the two agree.
 */
export function LatestInboundPanel({
  latest,
  actions,
}: Readonly<{
  latest: JustIn | undefined;
  actions?: (activity: Activity) => ReactNode;
}>) {
  const t = useT();
  if (!latest) {
    return null;
  }
  const { activity, kind, notice } = latest;
  const { subject, excerpt } = itemWords(activity);
  const sender = activity.email_summary?.counterparty?.trim();
  const words = excerpt !== subject ? excerpt : undefined;
  return (
    <Panel tone="accent">
      <div className="latest-row">
        <span className="latest-label">
          <span className="latest-kind">
            {KIND_ICON[kind]}
            {kindLabel(kind, t)}
          </span>
          <When activity={activity} />
        </span>
        <span className="latest-rule" aria-hidden="true" />
        <div className="latest-body">
          {subject && <span className="latest-subject">{subject}</span>}
          {(sender || words) && (
            <span className="latest-excerpt">
              {sender && <span className="latest-sender">{sender}</span>}
              {sender && words ? " · " : null}
              {words}
            </span>
          )}
        </div>
        <div className="latest-verbs">
          {actions?.(activity)}
          {notice && (
            <Button variant="ghost" onClick={notice}>
              {t("latest.noticed")}
            </Button>
          )}
        </div>
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
  latest: JustIn | undefined;
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
