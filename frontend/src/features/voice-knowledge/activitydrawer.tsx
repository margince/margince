// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode, useId, useState } from "react";
import type { components } from "../../api/schema";
import {
  useCustomFieldDisplay,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EntityRef } from "../../components/entityref";
import { mostSpecificFirst, RecordLinks } from "../../components/recordlinks";
import { TaskDetailDialog, useTaskUpdate } from "../../components/taskactions";
import { writtenAsMarkdown } from "../../design-system/activitytimeline";
import { Badge, Modal } from "../../design-system/atoms";
import { CommitmentReview } from "../../design-system/commitmentreview";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { Markdown } from "../../design-system/markdown";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { TIMELINE_KIND_LABEL } from "../../design-system/timelineentry";
import { isTranscriptActivity } from "../../design-system/transcriptread";
import { formatDate, formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { activityFollowUpsKey, TASK_QUEUE_KEY } from "../../lib/activitykeys";
import { useActivity } from "../../lib/activityread";
import { useCanWrite } from "../../lib/capability";
import { problemCodeOf } from "../../lib/problem";
import { ActivityFiles } from "../records/activityfiles";
import { ActivityFollowUps } from "../records/activityfollowups";
import { worklistKey } from "../worklist/worklist.queries";
import "../../design-system/emaildetail.css";

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
  activity,
}: Readonly<{
  activityId: string | null;
  /** The record's timezone, which the page owns. */
  zone: string;
  onClose: () => void;
  /** The answer of a live read the host already holds, so the drawer does not
   *  read the activity a second time. Absent, the drawer reads it. */
  activity?: Activity;
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
      given={activity?.id === shown ? activity : undefined}
    />
  );
}

function ActivityDetail({
  activityId,
  open,
  zone,
  onClose,
  given,
}: Readonly<{
  activityId: string;
  open: boolean;
  zone: string;
  onClose: () => void;
  given?: Activity;
}>) {
  const t = useT();
  const titleId = useId();
  // Read while closing too, so the panel keeps its words as it animates out.
  const read = useActivity(activityId, given === undefined);
  const activity = given ?? read.data;
  const unavailable = ["not_found", "permission_denied"].includes(
    problemCodeOf(read.error) ?? "",
  );
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
    >
      <DrawerHead>
        {activity?.archived_at && <Badge>{t("record.archived")}</Badge>}
        {activity && <Eyebrow>{t(TIMELINE_KIND_LABEL[activity.kind])}</Eyebrow>}
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {activityTitle(activity, t)}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {activity ? (
          <ActivityBody activity={activity} zone={zone} />
        ) : (
          <SurfaceState
            state={
              unavailable ? "withheld" : read.isError ? "failed" : "loading"
            }
            emptyLabel={t("activity.detail.none")}
            loadingLabel={t("activity.detail.loading")}
            detail={
              unavailable
                ? { withheldReason: t("activity.detail.withheldReason") }
                : { onRetry: () => void read.refetch() }
            }
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
  const [opened, setOpened] = useState<string | null>(null);
  const canUpdate = useCanWrite("activity", "update");
  const update = useTaskUpdate([
    worklistKey,
    TASK_QUEUE_KEY,
    activityFollowUpsKey(activity.id),
  ]);
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
        <CustomFieldLines activity={activity} />
      </div>
      {withheldOrBody(activity, t)}
      {activity.content_state !== "withheld" && (
        <>
          <Panel title={t("files.title")}>
            <PanelBody>
              <ActivityFiles
                activityId={activity.id}
                archived={Boolean(activity.archived_at)}
              />
            </PanelBody>
          </Panel>
          {["meeting", "call", "note"].includes(activity.kind) && (
            <ActivityFollowUps
              sourceId={activity.id}
              archived={Boolean(activity.archived_at)}
              onOpen={setOpened}
            />
          )}
          {opened && (
            <TaskDetailDialog
              activityId={opened}
              readOnly={!canUpdate || Boolean(activity.archived_at)}
              update={update}
              onClose={() => setOpened(null)}
              onOpenSource={() => setOpened(null)}
            />
          )}
        </>
      )}
      {activity.content_state !== "withheld" && (
        <CommitmentReview
          decisions={activity.commitment_admissions}
          zone={zone}
        />
      )}
    </div>
  );
}

// The workspace's own fields, one line each, for the ones this activity holds a
// value in. A withheld activity arrives with none, so it shows none.
function CustomFieldLines({ activity }: Readonly<{ activity: Activity }>) {
  const { fields } = useObjectCustomFields("activity", "record");
  const display = useCustomFieldDisplay();
  return fields.map((field) => {
    const shown = display(field, activity[field.column_name]);
    return shown === null ? null : (
      <PartyLine key={field.column_name} label={field.label}>
        {shown}
      </PartyLine>
    );
  });
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

function activityTitle(
  activity: Activity | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (!activity) return t("activity.detail.none");
  if (activity.content_state === "withheld") return t("timeline.withheld");
  return activity.subject?.trim() || t(TIMELINE_KIND_LABEL[activity.kind]);
}
