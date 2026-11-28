import { RecordMeetingOutcome } from "../../components/meetingoutcome";
import { GeneratedText } from "../../design-system/generatedtext";
import { MeetingNotesText } from "../../design-system/timelinetext";
import { displayedBody, displayedSubject } from "../../lib/systemdisplay";
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode, useId, useState } from "react";
import type { components } from "../../api/schema";
import {
  useCustomFieldDisplay,
  useCustomFieldLabel,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EntityRef } from "../../components/entityref";
import { mostSpecificFirst, RecordLinks } from "../../components/recordlinks";
import { TaskDetailDialog, useTaskUpdate } from "../../components/taskactions";
import { writtenAsMarkdown } from "../../design-system/activitytimeline";
import { Badge, Button, Modal } from "../../design-system/atoms";
import { CommitmentReview } from "../../design-system/commitmentreview";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { NoteMarkdown } from "../../design-system/markdown";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import {
  AiAssistedFlag,
  TIMELINE_KIND_LABEL,
} from "../../design-system/timelineentry";
import { isTranscriptActivity } from "../../design-system/transcriptread";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatDue } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { activityFollowUpsKey, TASK_QUEUE_KEY } from "../../lib/activitykeys";
import { useActivity } from "../../lib/activityread";
import { callOutcomeLabel } from "../../lib/calloutcome";
import { useCanWrite } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { problemCodeOf } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { ActivityFiles } from "../records/activityfiles";
import { ActivityFollowUps } from "../records/activityfollowups";
import { worklistKey } from "../worklist/worklist.queries";
import {
  AttendeesLine,
  answersOf,
  colleaguesOf,
  PartyLine,
  ResponseBadge,
} from "./activitydrawer.parties";
import { WebFormSubmission } from "./webformsubmission";
import "../../design-system/emaildetail.css";

type Activity = components["schemas"]["Activity"];
type ActivityLink = components["schemas"]["ActivityLink"];

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
  activityIsLive = false,
  about,
}: Readonly<{
  activityId: string | null;
  /** The record's timezone, which the page owns. */
  zone: string;
  onClose: () => void;
  /** Initial row while the drawer obtains the current read. */
  activity?: Activity;
  /** The host keeps this row under a live read while the drawer is open. */
  activityIsLive?: boolean;
  /** What the host opened it for, drawn above the activity: a worklist row
   *  says what it asks of the reader. */
  about?: ReactNode;
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
      givenIsLive={activityIsLive}
      about={about}
    />
  );
}

function ActivityDetail({
  activityId,
  open,
  zone,
  onClose,
  given,
  givenIsLive,
  about,
}: Readonly<{
  activityId: string;
  open: boolean;
  zone: string;
  onClose: () => void;
  given?: Activity;
  givenIsLive: boolean;
  about?: ReactNode;
}>) {
  const t = useT();
  const titleId = useId();
  // Read while closing too, so the panel keeps its words as it animates out.
  const read = useActivity(
    activityId,
    given === undefined || (given.kind === "meeting" && !givenIsLive),
  );
  const activity = activityForDrawer(given, read.data, read.isError);
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
        <AiAssistedFlag entry={{ aiAssisted: activity?.ai_assisted }} />
        {activity && (
          <Eyebrow>{t(labelFor(TIMELINE_KIND_LABEL, activity.kind))}</Eyebrow>
        )}
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {activity ? (
            <GeneratedText
              activityId={activity.id}
              display={activity.system_display}
              inline
            >
              {activityTitle(activity, t)}
            </GeneratedText>
          ) : (
            activityTitle(activity, t)
          )}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {activity ? (
          <ActivityBody activity={activity} zone={zone} about={about} />
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

function MeetingRecordDetails({ activity }: Readonly<{ activity: Activity }>) {
  if (activity.kind !== "meeting" || activity.content_state === "withheld")
    return null;
  return (
    <>
      {activity.meeting_notes && activity.meeting_notes !== activity.body && (
        <MeetingNotesText notes={activity.meeting_notes} />
      )}
      <div className="actions">
        <RecordMeetingOutcome
          activity={activity}
          invalidateKeys={[
            ["activity", activity.id],
            ["activities"],
            worklistKey,
          ]}
        />
      </div>
    </>
  );
}

function ActivityBody({
  activity,
  zone,
  about,
}: Readonly<{ activity: Activity; zone: string; about?: ReactNode }>) {
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
  // Guests no visible link shows: not a contact yet, past the link ceiling, or
  // a contact this reader may not open. The invitation's name stands in.
  const guests = attended ? (activity.guests ?? []) : [];
  const colleagues = colleaguesOf(activity);
  const answers = answersOf(activity);
  return (
    <div className="emaildetail__body">
      {about}
      <div className="emaildetail__parties">
        <PartyLine label={t("activity.detail.when")}>
          {formatActivityWhen(
            activity.occurred_at,
            activity.occurred_precision,
            locale,
            zone,
          )}
          {activity.kind === "task" &&
            activity.due_at &&
            ` · ${t("co.next.due", {
              when: formatDue(
                activity.due_at,
                activity.due_has_time,
                locale,
                zone,
                activity.due_intent,
              ),
            })}`}
          <CallOutcomeNote activity={activity} />{" "}
          {activity.is_done && (
            <Badge tone="success">{t("tasks.isDone")}</Badge>
          )}
        </PartyLine>
        <AttendeesLine
          attendees={attendees}
          guests={guests}
          answers={answers}
          leftOut={activity.guests_left_out}
        />
        {colleagues.length > 0 && (
          <PartyLine label={t("activity.detail.colleagues")}>
            {colleagues.map((id, index) => (
              <Fragment key={id}>
                {index > 0 && ", "}
                <EntityRef kind="user" id={id} />
                <ResponseBadge response={answers.get(`user:${id}`)} />
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
      {withheldOrBody(activity, filed, t)}
      <MeetingRecordDetails activity={activity} />
      {activity.content_state !== "withheld" && (
        <>
          <Panel title={t("files.title")}>
            <PanelBody>
              <MeetingNotesHint concern={activity.meeting_notes_connection} />
              <ActivityFiles
                activityId={activity.id}
                writable={activity.writable ?? false}
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
              // The follow-up's source is this activity, already open under it.
              openerFor={(ref) =>
                ref.id === activity.id ? () => setOpened(null) : undefined
              }
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
  const fieldLabel = useCustomFieldLabel();
  return fields.map((field) => {
    const shown = display(field, activity[field.column_name]);
    return shown === null ? null : (
      <PartyLine key={field.column_name} label={fieldLabel(field)}>
        {shown}
      </PartyLine>
    );
  });
}

function withheldOrBody(
  activity: Activity,
  filed: readonly ActivityLink[],
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
  // The body says the same as text; the fields read better as fields.
  if (activity.web_form) {
    return <WebFormSubmission sent={activity.web_form} filed={filed} />;
  }
  const body = displayedBody(activity)?.trim();
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
      <GeneratedText
        activityId={activity.id}
        display={activity.system_display}
        part="body"
        renderText={(text) => <NoteMarkdown source={text} />}
      >
        <NoteMarkdown source={body} />
      </GeneratedText>
    </div>
  ) : (
    <p className="t-body" style={{ whiteSpace: "pre-wrap" }}>
      <GeneratedText
        activityId={activity.id}
        display={activity.system_display}
        part="body"
      >
        {body}
      </GeneratedText>
    </p>
  );
}

function activityTitle(
  activity: Activity | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (!activity) return t("activity.detail.none");
  if (activity.content_state === "withheld") return t("timeline.withheld");
  return (
    displayedSubject(activity)?.trim() ||
    t(labelFor(TIMELINE_KIND_LABEL, activity.kind))
  );
}

/** How a call ended, after its time. Nothing on a call without an outcome. */
function CallOutcomeNote({
  activity,
}: Readonly<{ activity: Pick<Activity, "kind" | "call_outcome"> }>) {
  const t = useT();
  const label = callOutcomeLabel(activity, t);
  return label ? <> · {label}</> : null;
}

function MeetingNotesHint({
  concern,
}: Readonly<{ concern: Activity["meeting_notes_connection"] }>) {
  const t = useT();
  if (!concern) return null;
  const message =
    concern === "notes_account_mismatch"
      ? "activity.notes.otherAccount"
      : "activity.notes.connectHint";
  return (
    <div>
      <p>{t(message)}</p>
      <Button
        variant="link"
        onClick={() => navigate(settingsHref("connections"))}
      >
        {t("connectors.notes.connect")}
      </Button>
    </div>
  );
}

function activityForDrawer(
  given: Activity | undefined,
  fresh: Activity | undefined,
  failed: boolean,
): Activity | undefined {
  if (given?.kind === "meeting") return failed ? undefined : (fresh ?? given);
  return given ?? fresh;
}
