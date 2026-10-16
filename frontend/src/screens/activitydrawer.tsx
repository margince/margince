// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { EntityRef } from "../components/entityref";
import { writtenAsMarkdown } from "../design-system/activitytimeline";
import { Badge, Modal } from "../design-system/atoms";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Eyebrow } from "../design-system/eyebrow";
import { Heading } from "../design-system/heading";
import { Markdown } from "../design-system/markdown";
import { SurfaceState } from "../design-system/surfacestate";
import { TIMELINE_KIND_LABEL } from "../design-system/timelineentry";
import { isTranscriptActivity } from "../design-system/transcriptread";
import { formatDate, formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useActivity } from "../lib/activityread";
import { mostSpecificFirst, RecordLinks } from "./recordlinks";
import "../design-system/emaildetail.css";

type Activity = components["schemas"]["Activity"];

// The kinds whose linked contacts were in the room. On a note or a task a
// linked contact is what the entry is about, so it is filed, not attending.
const ATTENDED_KINDS = new Set(["meeting", "call"]);

/**
 * One meeting, call, note or task, opened for reading where it was found.
 *
 * The email hit's drawer reads the message presentation; this one reads the
 * plain activity, under the same row scope and audience as the timeline. Like
 * `OpenEmailDrawer`, it stays mounted after the first open so it can animate
 * out, keeping the id it was last opened with.
 */
export function OpenActivityDrawer({
  activityId,
  zone,
  onClose,
}: Readonly<{
  activityId: string | null;
  /** The record's timezone, which the page owns. */
  zone: string;
  onClose: () => void;
}>) {
  const [shown, setShown] = useState<string | null>(activityId);
  if (activityId !== null && activityId !== shown) {
    setShown(activityId);
  }
  if (shown === null) {
    return null;
  }
  return (
    <ActivityDetail
      activityId={shown}
      open={activityId !== null}
      zone={zone}
      onClose={onClose}
    />
  );
}

function ActivityDetail({
  activityId,
  open,
  zone,
  onClose,
}: Readonly<{
  activityId: string;
  open: boolean;
  zone: string;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  // Read while closing too, so the panel keeps its words as it animates out.
  const read = useActivity(activityId);
  const activity = read.data;
  const withheld = activity?.content_state === "withheld";
  const title = !activity
    ? t("activity.detail.none")
    : withheld
      ? t("timeline.withheld")
      : activity.subject?.trim() || t(TIMELINE_KIND_LABEL[activity.kind]);
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
    >
      <DrawerHead>
        {activity && <Eyebrow>{t(TIMELINE_KIND_LABEL[activity.kind])}</Eyebrow>}
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {title}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {activity ? (
          <ActivityBody activity={activity} zone={zone} />
        ) : (
          <SurfaceState
            state={read.isError ? "failed" : "loading"}
            emptyLabel={t("activity.detail.none")}
            loadingLabel={t("activity.detail.loading")}
            detail={{ onRetry: () => void read.refetch() }}
          >
            {null}
          </SurfaceState>
        )}
      </DrawerBody>
    </Modal>
  );
}

function ActivityBody({
  activity,
  zone,
}: Readonly<{ activity: Activity; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const links = mostSpecificFirst(activity.links ?? []);
  const attended = ATTENDED_KINDS.has(activity.kind);
  const attendees = attended
    ? links.filter((link) => link.entity_type === "contact")
    : [];
  const filed = attended
    ? links.filter((link) => link.entity_type !== "contact")
    : links;
  const colleagues = colleaguesOf(activity);
  return (
    <div className="emaildetail__body">
      <div className="emaildetail__parties">
        <PartyLine label={t("activity.detail.when")}>
          {formatDateTime(activity.occurred_at, locale, zone)}
          {activity.kind === "task" && activity.due_at && (
            <>
              {" "}
              ·{" "}
              {t("co.next.due", {
                when: formatDate(activity.due_at, locale, zone),
              })}
            </>
          )}{" "}
          {activity.is_done && (
            <Badge tone="success">{t("tasks.isDone")}</Badge>
          )}
        </PartyLine>
        {attendees.length > 0 && (
          <PartyLine label={t("activity.detail.attendees")}>
            <RecordLinks links={attendees} newTab />
          </PartyLine>
        )}
        {colleagues.length > 0 && (
          <PartyLine label={t("activity.detail.colleagues")}>
            {colleagues.map((id, index) => (
              <Fragment key={id}>
                {index > 0 && ", "}
                <EntityRef kind="user" id={id} />
              </Fragment>
            ))}
          </PartyLine>
        )}
        {filed.length > 0 && (
          <PartyLine label={t("activity.detail.filedUnder")}>
            <RecordLinks links={filed} newTab />
          </PartyLine>
        )}
      </div>
      {withheldOrBody(activity, t)}
    </div>
  );
}

// The host first, then every other colleague the read names, each once.
function colleaguesOf(activity: Activity): string[] {
  const seats = [activity.host_user_id, ...(activity.colleague_ids ?? [])];
  return [...new Set(seats.filter((id): id is string => Boolean(id)))];
}

function withheldOrBody(
  activity: Activity,
  t: ReturnType<typeof useT>,
): ReactNode {
  if (activity.content_state === "withheld") {
    return (
      <SurfaceState
        state="withheld"
        emptyLabel={t("activity.detail.none")}
        loadingLabel={t("activity.detail.loading")}
        detail={{ withheldReason: t("activity.detail.withheldReason") }}
      >
        {null}
      </SurfaceState>
    );
  }
  const body = activity.body?.trim();
  if (!body) {
    return null;
  }
  // Read the way the timeline row reads it: a rep's own words as markdown, a
  // transcript or a chat as the text it arrived as.
  const markdown = writtenAsMarkdown({
    kind: activity.kind,
    transcript: isTranscriptActivity(activity),
  });
  return markdown ? (
    <div className="t-body">
      <Markdown source={body} autolink />
    </div>
  ) : (
    <p className="t-body" style={{ whiteSpace: "pre-wrap" }}>
      {body}
    </p>
  );
}

function PartyLine({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">{label}</span>
      {children}
    </p>
  );
}
