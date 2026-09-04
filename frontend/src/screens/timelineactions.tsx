// The per-row action cluster the 360 timelines mount in each entry's action
// slot, and the two audience controls behind it.
//
// Lifted out of compose.tsx, which is 3,459 lines and was answering two
// unrelated questions: how a person writes a message, and what a reader may do
// to one already on a timeline. Nothing here changed in the move — the writes
// now go through the shared audience service, which is where the thread
// decision was already spelled a second time.

import { type ReactNode, useState } from "react";

import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { entityTimelineKeys } from "./activitykeys";
import {
  AUDIENCE_CHOICES,
  AUDIENCE_HINT,
  AUDIENCE_LABEL,
  AudienceMembers,
  useAudienceCandidates,
} from "./audiencemembers";
import { useMessageAudience, useThreadAudience } from "./audienceservice";
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
      {/* Captured mail's audience is derived, never a direct write, and
          ThreadAudienceAction's own endpoint refuses a thread_key with no
          capture_import row behind it (capture/threadverdict.go). A hand-typed
          REPLY carries a thread_key too — outboundmessage.go stamps every send
          with the RFC822 thread it answers, imported or not — so thread_key
          alone would route a rep's own threaded reply to the one control that
          404s on it. `audience_reason` is no better a signal: it is null on an
          untouched captured row (the wrong per-message dialog offered first)
          and non-null on a narrowed hand-typed one, whose missing
          capture_import row makes ThreadAudienceAction disappear for good.
          captured_by's own prefix is what actually says "capture wrote this"
          — `connector:<name>:<uuid>`, never `human:<uuid>` or `agent:<id>` —
          so it is the one signal correct in every direction.

          This inference is display code holding a backend ownership rule. The
          server answers the same question properly as `change_mode`, but on
          the email PRESENTATION rather than on the summary a list row carries
          — so the honest place to spend it is the drawer's own access editor,
          where the presentation is already loaded. Deleting the inference here
          before that editor exists would take the audience control off every
          timeline row and give nothing back (margince#3811). */}
      {(activity.captured_by ?? "").startsWith("connector:") ? (
        <ThreadAudienceAction
          activity={activity}
          entityType={entityType}
          entityId={entityId}
        />
      ) : (
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

// Why a captured message is held, in the reader's words. A reason the server
// learned to give and this map has not falls back to nothing rather than to the
// raw token: a badge reading `financial_corporate` beside a customer's mail is
// worse than no badge at all.
const audienceReasonLabel: Record<string, MessageKey> = {
  posture: "compose.reason.posture",
  workspace_floor: "compose.reason.workspaceFloor",
  no_record: "compose.reason.noRecord",
  pending_verdict: "compose.reason.pendingVerdict",
  manual: "compose.reason.manual",
};

// ThreadAudienceAction shares or keeps back a whole THREAD, for a message that
// came from a mailbox.
//
// Captured mail does not take the per-message dialog: its audience is derived
// from what every importing mailbox asks for, so a direct write is refused
// (`audience_is_derived`) and pointed here. The unit is the thread rather than
// the message because that is what a person decides about — nobody shares the
// third reply and keeps the fourth.
//
// The decision releases only the CALLER's hold. A thread two colleagues
// imported opens when both allow it, so the outcome reports how many other
// seats still hold it — a count and never a name, because whose mail a person
// keeps private is itself private.
function ThreadAudienceAction({
  activity,
  entityType,
  entityId,
}: Readonly<{
  activity: Activity;
  entityType: RelinkKind;
  entityId: string;
}>) {
  const t = useT();
  const [held, setHeld] = useState<number | null>(null);
  const shared = activity.audience === "workspace";
  const threadKey = activity.thread_key;
  const mutation = useThreadAudience({
    invalidate: () => entityTimelineKeys(entityType, entityId),
    onSettled: ({ outcome }) => {
      // A share that did not open the thread means somebody else still holds
      // it. Saying so is the difference between a control that looks broken
      // and one that reports what actually happened.
      setHeld(outcome && !outcome.shared ? outcome.held_by_others : null);
    },
  });
  // Withheld content carries no reason either, so there is nothing to draw and
  // no standing to change it.
  if (activity.content_state === "withheld" || !threadKey) {
    return null;
  }
  const reasonKey = audienceReasonLabel[activity.audience_reason ?? ""];
  return (
    <>
      {!shared && reasonKey && <Badge tone="warn">{t(reasonKey)}</Badge>}
      <Button
        small
        pending={mutation.isPending}
        onClick={() => {
          setHeld(null);
          mutation.mutate({ threadKey, share: !shared });
        }}
      >
        {shared ? t("compose.threadKeepPrivate") : t("compose.threadShare")}
      </Button>
      {held !== null && (
        <span className="t-caption">
          {t("compose.threadStillHeld").replace("{count}", String(held))}
        </span>
      )}
    </>
  );
}

// AudienceAction limits (or re-opens) who may read ONE message's content. Per
// message on purpose: a thread is not a unit of trust, and the contact stays
// visible to everyone either way. Absent on a row the reader cannot read in
// full — somebody who is not in the audience has no standing to change it.
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
  // It starts EMPTY even on a message that is already limited, and that is a
  // boundary rather than an oversight: `selected_members` rides EmailAccess,
  // gated on `can_change`, and a timeline row carries no access block — reading
  // one per row would be a detail fetch per visible line. So this dialog names
  // a new set rather than editing the standing one, and the confirm below
  // refuses an empty set so nobody narrows a message to nobody by pressing it
  // twice. Editing an existing set in place belongs in the drawer, which
  // already holds the access block it would start from.
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
