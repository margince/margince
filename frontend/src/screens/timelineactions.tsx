// The per-row action cluster the 360 timelines mount in each entry's action
// slot, and the audience control behind it.
//
// Lifted out of compose.tsx, which is 3,459 lines and was answering two
// unrelated questions: how a person writes a message, and what a reader may do
// to one already on a timeline. The writes go through the shared audience
// service, which is where the thread decision was already spelled a second
// time.
//
// An EMAIL's audience is not changed from here. It is changed in the drawer,
// which loads the access block and is told by the server which write it may
// perform — so the row no longer guesses that from `captured_by`.

import { type ReactNode, useState } from "react";

import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { entityTimelineKeys } from "./activitykeys";
import {
  AUDIENCE_CHOICES,
  AUDIENCE_HINT,
  AUDIENCE_LABEL,
  AudienceMembers,
  useAudienceCandidates,
} from "./audiencemembers";
import { useMessageAudience } from "./audienceservice";
import { problemMessageOf } from "./common";
// Reply and Relink stay in compose.tsx for now: they are the composer's own
// verbs, and moving them too would make a behaviour-preserving extraction into
// a much larger diff. What moved here is the cluster and the two audience
// controls, which are about reading a message rather than writing one.
import { ChannelReplyAction, type RelinkKind, RelinkModal } from "./compose";

type Activity = components["schemas"]["Activity"];
type ActivityAudience = components["schemas"]["ActivityAudience"];
type AudienceMember = components["schemas"]["AudienceMember"];

// The per-row action cluster the 360 timelines mount in each entry's action
// slot.
//
// Reply, because a send anchored to something that was never mail carries no
// RFC822 identity to thread against and simply starts a conversation — which is
// how the backend already reads it. Gating the composer on an email row instead
// makes a fresh workspace, whose only rows are logged notes, unable to send at
// all. A `message` row carries the opposite gate: it is withheld, not always
// offered, when the person behind it cannot be reached on the transport that
// carried it (see useChannelReachable).
//
// Relink, because an activity shown on a 360 timeline is by construction already
// linked to the entity whose timeline renders it, so re-associating it to the
// right record is always meaningful — and the Activity list payload carries no
// `links` to gate on regardless. It is offered unconditionally.
//
// It owns the two open states so the timeline mapper stays presentational.
//
// `extra` is how a surface adds a verb only it can serve — the person page's
// meeting brief opens a drawer this file cannot see. It renders before Relink
// so the row's own subject-matter verbs lead and the corrective ones follow.
export function TimelineActions({
  activity,
  entityType,
  entityId,
  personId,
  extra,
}: Readonly<{
  activity: Activity;
  entityType: RelinkKind;
  entityId: string;
  personId?: string;
  extra?: (activity: Activity) => ReactNode;
}>) {
  const t = useT();
  const [relink, setRelink] = useState(false);
  return (
    <>
      <ChannelReplyAction
        activityId={activity.id}
        kind={activity.kind}
        channelProvider={activity.channel_provider ?? undefined}
        entityType={entityType}
        entityId={entityId}
        personId={personId}
        contentWithheld={activity.content_state === "withheld"}
      />
      {extra?.(activity)}
      <Button small onClick={() => setRelink(true)}>
        {t("compose.relink")}
      </Button>
      {/* An EMAIL's audience is changed from the message, in the drawer, where
          the server states which write it would accept as `change_mode` and the
          editor opens on the set already standing. The row used to decide that
          here by testing `captured_by` for a `connector:` prefix — a backend
          ownership rule spelled in display code, in a second language, where
          the next change to what makes an audience derived would not find it.

          It could not simply be deleted: `change_mode` costs two per-row
          authorization queries (EnsureActivityWritable and callerIsSenderSeat
          in activities/emailaccess.go), which is why it rides the presentation
          and not the summary a twenty-row page ships. What removes the guess is
          having somewhere better to ask, and the drawer is that.

          Every other kind is offered it, and AudienceAction itself withholds
          it where the server says the audience was derived. That claim used to
          live here as "its audience is never derived", which was false for a
          connector-captured `message` and is the defect this comment now
          records rather than repeats. */}
      {activity.kind !== "email" && (
        <AudienceAction
          activity={activity}
          entityType={entityType}
          entityId={entityId}
        />
      )}
      {relink && (
        <RelinkModal
          activityId={activity.id}
          activityVersion={activity.version}
          threadKey={activity.thread_key}
          entityType={entityType}
          entityId={entityId}
          open={relink}
          onClose={() => setRelink(false)}
        />
      )}
    </>
  );
}

// AudienceAction limits (or re-opens) who may read ONE row's content — a note,
// a call, a meeting, a channel message. Per row on purpose: a thread is not a
// unit of trust, and the contact stays visible to everyone either way. Absent
// on a row the reader cannot read in full: somebody who is not in the audience
// has no standing to change it.
//
// An email does not come here. Its audience may be derived from what every
// importing mailbox asked for, which is a different write, and the drawer is
// where the server states which one it would accept.
export function AudienceAction({
  activity,
  entityType,
  entityId,
}: Readonly<{
  activity: Activity;
  entityType: RelinkKind;
  entityId: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const current: ActivityAudience = activity.audience ?? "workspace";
  const [choice, setChoice] = useState<ActivityAudience>(current);
  // The set the reader is building. Read only when `selected` is the choice,
  // and submitted as the FULL replacement set, which is what the write takes.
  //
  // It starts EMPTY even on a row that is already limited, and that is a
  // boundary rather than an oversight: who is named rides the access block,
  // which only an email has and only the drawer's read fetches. A note carries
  // no such read, so this dialog names a NEW set rather than editing the
  // standing one, and the confirm below refuses an empty set so nobody narrows
  // a row to nobody by pressing it twice.
  const [members, setMembers] = useState<AudienceMember[]>([]);
  // Fetched only while the dialog is open: the roster is two reads, and a
  // timeline of twenty rows would otherwise fire them per row on mount.
  const candidates = useAudienceCandidates(open && choice === "selected");
  const mutation = useMessageAudience({
    invalidate: () => entityTimelineKeys(entityType, entityId),
    onSettled: () => setOpen(false),
  });
  if (activity.content_state === "withheld") {
    return null;
  }
  // A DERIVED audience is not this dialog's to change, and offering it anyway
  // is what made a member press a control that opened, took their answer and
  // then refused it — naming a thread-level control the page does not have.
  //
  // Asked of `audience_reason`, which is the server saying so, rather than
  // inferred from the kind. The dispatch below used to read `kind !== "email"`
  // on the belief that only mail derives an audience; a connector-captured
  // `message` derives one exactly the same way, and the next kind that does
  // would have been missed the same way again.
  //
  // `manual` is a human's own answer and stays editable. Null is a row nothing
  // derived. The field is withheld with the content, which is why this sits
  // BELOW the withheld guard — above it, a held row would read as editable.
  if (audienceIsDerived(activity)) {
    return null;
  }
  return (
    <>
      <Button
        small
        onClick={() => {
          setChoice(current);
          setOpen(true);
        }}
      >
        {t("compose.audience")}
      </Button>
      {open && (
        <ConfirmModal
          open={open}
          onClose={() => setOpen(false)}
          title={t("compose.audienceTitle")}
          confirmLabel={t("compose.audienceConfirm")}
          confirmDisabled={
            choice === current ||
            (choice === "selected" && members.length === 0)
          }
          onConfirm={() =>
            mutation.mutate({
              activityId: activity.id,
              version: activity.version,
              audience: choice,
              members: choice === "selected" ? members : undefined,
            })
          }
          pending={mutation.isPending}
          error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        >
          <div className="compose-fields">
            <ChoiceList
              legend={t("compose.audienceLegend")}
              value={choice}
              onChange={setChoice}
              choices={AUDIENCE_CHOICES.map((value) => ({
                value,
                label: t(AUDIENCE_LABEL[value]),
                description: t(AUDIENCE_HINT[value]),
              }))}
            />
            {/* The picker only where the choice needs one. A limited-to-nobody
                audience is not a limit anybody meant, so the confirm below
                refuses an empty set rather than writing it. */}
            {choice === "selected" && (
              <AudienceMembers
                candidates={candidates}
                chosen={members}
                onChange={setMembers}
              />
            )}
            <p className="t-caption">{t("compose.audienceNote")}</p>
          </div>
        </ConfirmModal>
      )}
    </>
  );
}

// audienceIsDerived reports whether the SYSTEM decided who may read this row.
//
// `audience_reason` is the server's own word for it, and reading it is what
// stops this being guessed from the kind again: `posture` (a mailbox asked for
// it), `workspace_floor`, `no_record`, `pending_verdict`. `manual` is a human's
// answer and is still theirs to change; null is a row nothing derived.
//
// Null is also what a WITHHELD row reports, because the reason describes what
// the message is about — so callers must fail closed on withheld first, or a
// held row reads as editable here.
export function audienceIsDerived(activity: Activity): boolean {
  const reason = activity.audience_reason;
  return reason != null && reason !== "manual";
}
