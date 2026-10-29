// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { FileText, ListChecks, Mail, MessageCircle } from "lucide-react";
import {
  type ComponentProps,
  createContext,
  type ReactNode,
  useContext,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { NextBar } from "../design-system/nextbar";
import { activityAgo } from "../format/activitywhen";
import { emailSummaryText } from "../format/emailtext";
import { useNow } from "../format/now";
import { type Translator, useLocale, useT } from "../i18n";
import { taskWriteKeys, timelineBaseKey } from "../lib/activitykeys";
import type { EntityKind } from "../lib/entity";
import { useSourceOpener } from "../lib/opensource";
import { unwrap } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import "./latestinbound.css";

type Activity = components["schemas"]["Activity"];
type LatestInbound = components["schemas"]["LatestInbound"];
type LatestInboundKind = LatestInbound["kind"];

/** The item as a page holds it: `notice` sets it aside on this browser. */
export type JustIn = LatestInbound & Readonly<{ notice?: () => void }>;

function noticedKey(activityId: string) {
  return { family: STORAGE_KEYS.latestInboundNoticed, member: activityId };
}

/** The read every surface that shows the record's newest unanswered item shares. */
export function latestInboundKey(
  entityType: EntityKind,
  entityId: string,
): QueryKey {
  return [...timelineBaseKey(entityType, entityId), "latest-inbound"];
}

/**
 * The newest thing they sent on this record that nobody has answered, unless
 * the reader pressed "Noticed" on it. The server judges what is unanswered and
 * leaves out what a task already stands for. Keyed under the record's timeline
 * key, so every write that refreshes the timeline refreshes this too.
 */
export function useLatestInbound(
  entityType: EntityKind,
  entityId: string,
): JustIn | undefined {
  // The ids set aside since mount; storage alone would not draw the page again.
  const [noticed, setNoticed] = useState<ReadonlySet<string>>(new Set());
  const query = useQuery({
    queryKey: latestInboundKey(entityType, entityId),
    queryFn: async () =>
      unwrap(
        await api.GET("/activities/latest-inbound", {
          params: { query: { entity_type: entityType, entity_id: entityId } },
        }),
      ),
  });
  const latest = query.data?.data.find(
    ({ activity }) =>
      !noticed.has(activity.id) && readStored(noticedKey(activity.id)) === null,
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

/** What a record page hands its Next bar about the item. */
type JustInScopeValue = Readonly<{
  latest: JustIn | undefined;
  entityType: EntityKind;
  entityId: string;
  // The verbs the record's timeline offers on that row, so the two agree.
  actions?: (activity: Activity) => ReactNode;
  // The record's own reads a new task changes, beside its timeline.
  refreshKeys?: readonly QueryKey[];
  // The page's own message drawer, where it has one; else the shell's.
  onOpenEmail?: (activityId: string) => void;
}>;

const JustInContext = createContext<JustInScopeValue | null>(null);

/** Makes the record's item known to the Next bar inside `children`. */
export function JustInScope({
  children,
  ...value
}: JustInScopeValue & Readonly<{ children: ReactNode }>) {
  return (
    <JustInContext.Provider value={value}>{children}</JustInContext.Provider>
  );
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
    (activity.kind === "email" ? emailSummaryText(raw) : oneLine(raw));
  return { subject, excerpt: excerpt || undefined };
}

// A form's note is one answer per line; the row reads them as one line.
function oneLine(text: string): string {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .join(" · ");
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

function readLabel(kind: LatestInboundKind, t: Translator): string {
  switch (kind) {
    case "email":
      return t("latest.read.email");
    case "message":
      return t("latest.read.message");
    case "web_form":
      return t("latest.read.webForm");
  }
}

/** Opens the item in the shell's drawer, or undefined when it cannot open. */
function useOpenItem(scope: JustInScopeValue | null): (() => void) | undefined {
  const opener = useSourceOpener(scope?.onOpenEmail);
  const latest = scope?.latest;
  return latest
    ? opener({
        id: latest.activity.id,
        kind: latest.activity.kind,
        contentState: latest.activity.content_state,
      })
    : undefined;
}

type NextBarProps = ComponentProps<typeof NextBar>;

/**
 * A record's Next bar that knows what just came in. The item takes the bar
 * when nothing more urgent is next. So the page never says there is nothing to
 * do under a mail that asks for something. Otherwise it is one line above the
 * bar. `urgent` defaults to the bar saying it is late. A bar whose move cites
 * the item (`about`) already says it, so it draws alone.
 */
export function RecordNextBar({
  urgent,
  about = [],
  ...bar
}: NextBarProps &
  Readonly<{
    urgent?: boolean;
    about?: readonly (string | null | undefined)[];
  }>) {
  const t = useT();
  const scope = useContext(JustInContext);
  const latest = scope?.latest;
  const open = useOpenItem(scope);
  if (!scope || !latest || about.includes(latest.activity.id)) {
    return <NextBar {...bar} />;
  }
  if (urgent ?? bar.late?.tone === "danger") {
    return (
      <>
        <LatestInboundLine
          latest={latest}
          onOpen={open}
          pressLabel={readLabel(latest.kind, t)}
        />
        <NextBar {...bar} />
      </>
    );
  }
  return (
    <JustInNextBar
      scope={scope}
      latest={latest}
      open={open}
      more={bar.more}
      onward={bar.onward}
    />
  );
}

function JustInNextBar({
  scope,
  latest,
  open,
  more,
  onward,
}: Readonly<{
  scope: JustInScopeValue;
  latest: JustIn;
  open?: () => void;
  more?: NextBarProps["more"];
  onward?: ReactNode;
}>) {
  const t = useT();
  const { activity, kind, notice } = latest;
  const { subject, excerpt } = itemWords(activity);
  const sender = activity.email_summary?.counterparty?.trim();
  return (
    <NextBar
      title={subject ?? kindLabel(kind, t)}
      late={{ label: kindLabel(kind, t), tone: "success" }}
      detail={excerpt !== subject ? excerpt : undefined}
      sub={
        <span className="latest-sub">
          {sender && <span className="latest-sender">{sender} · </span>}
          <When activity={activity} />
        </span>
      }
      insight={
        open && (
          <Button variant="link" onClick={open}>
            {readLabel(kind, t)}
          </Button>
        )
      }
      action={
        <>
          {scope.actions?.(activity)}
          <TaskFromItem scope={scope} latest={latest} />
          {notice && (
            <Button variant="ghost" onClick={notice}>
              {t("latest.noticed")}
            </Button>
          )}
        </>
      }
      more={more}
      onward={onward}
    />
  );
}

/**
 * Makes the item the reader's task. A request is accepted, so completing the
 * task settles it; anything else is a task made from the item. Either way the
 * server leaves the item out of the read once the task stands.
 */
function TaskFromItem({
  scope,
  latest,
}: Readonly<{ scope: JustInScopeValue; latest: JustIn }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const { activity, kind, accepts_request } = latest;
  const create = useMutation({
    mutationFn: async () => {
      const about = itemWords(activity).subject ?? kindLabel(kind, t);
      return unwrap(
        await api.POST("/tasks", {
          body: {
            subject: t(
              kind === "web_form" ? "latest.task.form" : "latest.task.reply",
              { subject: about },
            ),
            source: "manual",
            ...(accepts_request
              ? { request_activity_id: activity.id }
              : { source_activity_id: activity.id }),
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const key of [
        ...taskWriteKeys(scope.entityType, scope.entityId),
        ...(scope.refreshKeys ?? []),
      ]) {
        queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
  // The task reads the item's content, which a withheld row refuses.
  if (activity.content_state === "withheld") {
    return null;
  }
  return (
    <>
      <Button pending={create.isPending} onClick={() => create.mutate()}>
        <ListChecks aria-hidden />
        {t("latest.makeTask")}
      </Button>
      <ErrorLine error={create.error} inline />
    </>
  );
}

/**
 * The item as one line. It shows in the record's head on the other tabs, and
 * above a Next bar that has something more urgent. With no
 * `onOpen` it only says what came in.
 */
export function LatestInboundLine({
  latest,
  onOpen,
  pressLabel,
}: Readonly<{
  latest: JustIn | undefined;
  onOpen?: () => void;
  // What pressing it does, for a screen reader; going to the Overview by default.
  pressLabel?: string;
}>) {
  const t = useT();
  if (!latest) {
    return null;
  }
  const { activity, kind } = latest;
  const { subject, excerpt } = itemWords(activity);
  const label = kindLabel(kind, t);
  const content = (
    <>
      <span className="latest-line-mark">{KIND_ICON[kind]}</span>
      <span className="latest-line-text">
        <span className="latest-line-kind">
          {label} · <When activity={activity} />
        </span>
        {(subject ?? excerpt) && (
          <span className="latest-line-subject">{subject ?? excerpt}</span>
        )}
      </span>
    </>
  );
  if (!onOpen) {
    return <div className="latest-line">{content}</div>;
  }
  return (
    <button
      type="button"
      className="latest-line latest-line-button"
      onClick={onOpen}
      aria-label={pressLabel ?? t("latest.line.open", { kind: label })}
    >
      {content}
    </button>
  );
}
