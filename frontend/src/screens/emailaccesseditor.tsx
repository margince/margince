// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who reads this message, said in the drawer, and changed there.
//
// The timeline row already had an audience dialog. It could not do this one
// thing: start from the set that is already on the message. A list row carries
// a summary, not an access block, so the row's dialog names a NEW set every
// time it opens — and `selected_members` rides EmailAccess, which only the
// detail read fetches. Reading one per row would be a request per visible line.
//
// The drawer has the access block in hand, so it is the surface that can show
// a reader who is currently named and let them remove one person without
// retyping the other four.
//
// It also ends an inference. The timeline decides which of the two audience
// writes to offer by reading `captured_by` for a `connector:` prefix — display
// code holding a backend ownership rule, kept because deleting it before this
// editor existed would have taken the control off every row and given nothing
// back. Here the server says which write it would accept, as `change_mode`, and
// nothing guesses.

import { useState } from "react";

import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { emailDetailKey } from "../design-system/emaildetail";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  AUDIENCE_CHOICES,
  AUDIENCE_HINT,
  AUDIENCE_LABEL,
  AUDIENCE_REASON_LABEL,
  AudienceMembers,
  memberKey,
  useAudienceCandidates,
} from "./audiencemembers";
import { useMessageAudience, useThreadAudience } from "./audienceservice";
import { problemMessageOf } from "./common";
import "./emailaccesseditor.css";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailAccess = components["schemas"]["EmailAccess"];
type ActivityAudience = components["schemas"]["ActivityAudience"];
type AudienceMember = components["schemas"]["AudienceMember"];
type EmailAccessStatus = components["schemas"]["EmailAccessStatus"];

// The sentence version of the row's one-word badge. The row shows "Selected"
// because a badge that ran to a sentence would out-weigh the subject beside
// it; here there is room to say what it means for the person reading.
const STATUS_SENTENCE: Record<EmailAccessStatus, MessageKey> = {
  team: "email.access.sentence.team",
  participants: "email.access.sentence.participants",
  selected: "email.access.sentence.selected",
  withheld: "email.access.sentence.withheld",
};

/**
 * What the drawer says about who reads this message, and the control to
 * change it when this reader may.
 *
 * Always drawn, even for a reader who may change nothing: who may read a
 * message is a fact about it, like its date. A reader without standing sees
 * the sentence and no button — which is the honest rendering of "you can see
 * that this is limited, and it is not yours to widen".
 */
export function EmailAccessEditor({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  const t = useT();
  const { access } = presentation;
  const reasonKey = AUDIENCE_REASON_LABEL[access.explanation ?? ""];
  return (
    <div className="emailaccess">
      <div className="emailaccess__verdict">
        <Badge tone={access.display_status === "withheld" ? "warn" : undefined}>
          {t(STATUS_LABEL[access.display_status])}
        </Badge>
        <span className="t-caption">
          {t(STATUS_SENTENCE[access.display_status])}
        </span>
      </div>
      {/* WHY, when the server gave a reason. A held message the reader can see
          the outline of is otherwise a limit with no author: the reason names
          what decided it, which is what a person disagreeing with the verdict
          needs before they can argue with it.

          `explanation` is a TOKEN, not a sentence — `readEmailAccess` fills it
          from the same `audience_reason` column the timeline row reads — so it
          is translated through the shared map. Printing it raw would put
          `pending_verdict` on the screen. A reason the server learned to give
          and the map has not draws nothing rather than the token. */}
      {reasonKey && <p className="t-caption">{t(reasonKey)}</p>}
      <NamedMembers access={access} />
      {/* The control only where the server says this caller's write would be
          taken. `can_change` and `change_mode` are decided by the authority
          that would execute the write, so a button drawn from them is a button
          the write accepts — nothing here re-derives it from the row. */}
      {access.can_change && access.change_mode !== "none" && (
        <ChangeAccess presentation={presentation} />
      )}
    </div>
  );
}

// The one-word label, shared with the row's badge so both say the same word
// about the same message.
const STATUS_LABEL: Record<EmailAccessStatus, MessageKey> = {
  team: "email.access.team",
  participants: "email.access.participants",
  selected: "email.access.selected",
  withheld: "email.access.withheld",
};

/**
 * Who is named, when the message is limited to a set and this reader may see
 * the set.
 *
 * `selected_members` comes back only to a caller who may also change it — a
 * reader with no standing to edit the set has no standing to enumerate it. So
 * an absent list here is not an empty audience, and nothing is drawn for it:
 * printing "nobody" would be a false statement about a message that is in fact
 * limited to four people this reader may not name.
 *
 * The write's vocabulary is ids, not names — `AudienceMember` is a subject type
 * and a uuid, which is the right shape for a write and unreadable in a list. So
 * the names come from the roster the picker below already reads, on the same
 * two cache entries: a reader who opens the editor pays no second fetch, and
 * one who only looks pays two reads the app holds anyway.
 */
function NamedMembers({ access }: Readonly<{ access: EmailAccess }>) {
  const t = useT();
  const members = access.selected_members;
  const named = useAudienceCandidates((members?.length ?? 0) > 0);
  if (!members || members.length === 0) {
    return null;
  }
  const nameOf = new Map(
    named.map((candidate) => [
      `${candidate.kind}:${candidate.id}`,
      candidate.name,
    ]),
  );
  return (
    <ul className="emailaccess__members">
      {members.map((member) => (
        <li key={memberKey(member)} className="emailaccess__member">
          {/* A member the roster has not answered for yet, or one it no longer
              carries. The id is not shown: a uuid tells a reader nothing, and
              a seat that has left the organization is still a real limit on
              the message. */}
          {nameOf.get(memberKey(member)) ?? t("email.access.unnamedMember")}
        </li>
      ))}
    </ul>
  );
}

/**
 * The change control, in whichever of the two writes the server named.
 *
 * `thread_contribution` is a captured message: its audience is derived from
 * what every importing mailbox asks for, so what this reader changes is their
 * own contribution to the thread, and the outcome reports how many other seats
 * still hold it. `message_audience` is a hand-logged one, which carries an
 * audience somebody set and takes a direct write.
 *
 * Two writes and one control, because the reader is answering one question.
 */
function ChangeAccess({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  return presentation.access.change_mode === "thread_contribution" ? (
    <ThreadContribution presentation={presentation} />
  ) : (
    <MessageAudience presentation={presentation} />
  );
}

function ThreadContribution({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  const t = useT();
  const [held, setHeld] = useState<number | null>(null);
  const shared = presentation.access.audience === "workspace";
  const threadKey = presentation.thread_key;
  const mutation = useThreadAudience({
    invalidate: () => [emailDetailKey(presentation.id)],
    onSettled: ({ outcome }) => {
      // A share that did not open the thread means a colleague still holds it.
      // Saying so is the difference between a control that looks broken and
      // one that reports what happened — and it is a COUNT, never a name,
      // because whose mail a person keeps private is itself private.
      setHeld(outcome && !outcome.shared ? outcome.held_by_others : null);
    },
  });
  if (!threadKey) {
    // The server offered the thread write for a message carrying no thread to
    // write against. Nothing to press rather than a button that would 404.
    return null;
  }
  return (
    <div className="emailaccess__change">
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
      {mutation.isError && (
        <p className="emailaccess__error">
          {problemMessageOf(mutation.error, t)}
        </p>
      )}
    </div>
  );
}

/**
 * The per-message editor, opened from the standing set rather than from empty.
 *
 * This is the whole reason the editor lives in the drawer. The timeline row's
 * dialog starts every `selected` audience blank, so a reader removing one
 * person from a set of five had to re-tick the other four and hope they
 * remembered them. Here `selected_members` is already loaded, so the checklist
 * opens with the real set ticked and the reader changes what they came to
 * change.
 */
function MessageAudience({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  const t = useT();
  const { access } = presentation;
  const current: ActivityAudience = access.audience ?? "workspace";
  const standing = access.selected_members ?? [];
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState<ActivityAudience>(current);
  const [members, setMembers] = useState<AudienceMember[]>(standing);
  const candidates = useAudienceCandidates(open && choice === "selected");
  const mutation = useMessageAudience({
    invalidate: () => [emailDetailKey(presentation.id)],
    onSettled: () => setOpen(false),
  });
  // Nothing to submit when neither the audience nor the set moved. Comparing
  // the SET too is what makes this editor different from the row's: a reader
  // who only removed somebody never changed `choice`, and a confirm gated on
  // the audience alone would sit disabled through the one edit this surface
  // exists for.
  const unchanged =
    choice === current && (choice !== "selected" || sameSet(members, standing));
  return (
    <div className="emailaccess__change">
      <Button
        small
        onClick={() => {
          setChoice(current);
          setMembers(standing);
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
            unchanged || (choice === "selected" && members.length === 0)
          }
          onConfirm={() =>
            mutation.mutate({
              activityId: presentation.id,
              version: presentation.version,
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
                audience is not a limit anybody meant, so the confirm above
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
    </div>
  );
}

/**
 * Whether two member sets name the same people, order disregarded.
 *
 * Order is not meaning here: the checklist appends in tick order and the server
 * returns its own, so comparing sequences would call every set changed the
 * moment a reader unticked somebody and ticked them back.
 */
function sameSet(
  a: readonly AudienceMember[],
  b: readonly AudienceMember[],
): boolean {
  if (a.length !== b.length) {
    return false;
  }
  const inB = new Set(b.map(memberKey));
  return a.every((member) => inB.has(memberKey(member)));
}
