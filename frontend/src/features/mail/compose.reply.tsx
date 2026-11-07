import { useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { ComposeModal } from "./compose";
import { useChannelReachable } from "./composereachability";
import type { RelinkKind } from "./composerelink";
import { rowAnswer } from "./rowanswer";

type Activity = components["schemas"]["Activity"];
type EmailTarget = components["schemas"]["ActivityEmailTarget"];

// The reply affordance for ONE captured conversation, on its own so the two
// surfaces that offer it cannot come to offer different things.
//
// It exists as a separate export because the conversation-memory card wants
// exactly this and nothing else: Relink below is a raw-ledger act — "this
// activity is filed against the wrong record" — and a summary card is not where
// a reader re-files anything.
//
// The gate is the same one TimelineActions applies, and that sameness is the
// point of the extraction rather than a happy accident: a `message` row is
// withheld when the contact behind it cannot be reached on the transport that
// carried it, and a rep offered a reply on one surface and refused it on the
// other would have no way to tell which answer was true.
export function ChannelReplyAction({
  activityId,
  kind,
  emailTarget,
  channelProvider,
  entityType,
  entityId,
  contactId,
  contentWithheld,
  prepared,
  onSent,
}: Readonly<{
  activityId: string;
  kind: Activity["kind"];
  /** Where a new email about this row goes (`email_target`). */
  emailTarget?: EmailTarget;
  channelProvider?: string;
  entityType: RelinkKind;
  entityId: string;
  contactId?: string;
  // Told when the message actually went, for a caller whose own view the send
  // changes. ComposeModal invalidates the RECORD timelines it knows about; a
  // surface listing the unanswered — the worklist's waiting lane — is not one
  // of them, so without this its row keeps saying nobody has replied and keeps
  // offering to reply again.
  onSent?: () => void;
  // The row's content is not this reader's to see. The verb still works —
  // writing to the contact is not reading their mail — but it is not a REPLY,
  // and calling it one claims access to the message being answered.
  contentWithheld?: boolean;
  /**
   * This reply is the move a rule PREPARED. The button is indigo because the
   * press has the model write: the composer drafts on open from `intent`.
   * `label` is the caller's, since the word belongs to the ranking surface.
   *
   * Absent on the timelines, which mount the plain reply: nothing proposed
   * those, a reader simply opened a conversation and chose to answer it.
   */
  prepared?: { label: string; intent: string };
}>) {
  const t = useT();
  // `null` until the verb is first pressed; the drawer stays mounted from then
  // on, which is what lets it animate out rather than vanish. `seq` counts the
  // opens and keys the composer, so each press gets a composer of its own — a
  // drawer that outlives its close would otherwise reopen on the draft the
  // reader abandoned, and offer to send it again.
  const [reply, setReply] = useState<{
    seq: number;
    open: boolean;
    from: HTMLElement;
  } | null>(null);
  const reachable = useChannelReachable(
    kind === "message",
    contactId,
    channelProvider,
  );
  const answer = rowAnswer(kind);
  // A written-about row is withheld through `email_target`, which the server
  // leaves out for it, so only mail reaches the withheld branch below.
  if (!reachable || (answer === "about" && !offered(emailTarget))) {
    return null;
  }
  const company = answer === "about" ? companyOf(emailTarget) : undefined;
  const withheld = answer === "reply" && contentWithheld;
  // WITHHELD CONTENT IS NOT A PREPARED REPLY. `Write email` is an
  // account-started send with no prior message to answer, so a mark claiming a
  // rule read this thread and worked out an answer would claim exactly the
  // access the row has just been refused.
  const proposed = withheld ? undefined : prepared;
  return (
    <>
      <Button
        variant={proposed ? "ai" : undefined}
        onClick={(event) => {
          const from = event.currentTarget;
          setReply((prior) => ({
            seq: (prior?.seq ?? 0) + 1,
            open: true,
            from,
          }));
        }}
      >
        {proposed?.label ??
          t(
            withheld || answer === "about"
              ? "compose.writeEmail"
              : "compose.reply",
          )}
      </Button>
      {reply !== null && (
        <ComposeModal
          key={reply.seq}
          // No anchor when the content is withheld, which is what makes the
          // dialog match the button. `Write email` is an ACCOUNT-STARTED send
          // — ComposeModal's own word for one with no prior message to anchor
          // to — and handing it the withheld activity made it behave like a
          // reply that could not read what it was replying to: `THIS
          // CONVERSATION` re-rendered the row the reader had just been told
          // was not theirs, above a To field useReplyRecipient could resolve
          // nobody into.
          {...composerRecord({
            activityId: withheld ? undefined : activityId,
            entityType,
            entityId,
            contactId,
            company,
          })}
          // `email`, not the withheld row's own kind. ComposeModal reads
          // `message` as a CHANNEL reply and posts to send-message, which
          // needs the conversation it answers — and the anchor is exactly
          // what is withheld here. Passing the original kind left the
          // button's own Send throwing "a channel reply needs the
          // conversation it answers": the dialog opened and could not send.
          //
          // Email is not a fallback, it is what the button says. `Write
          // email` is an account-started send, the same shape the composer
          // uses when there is no prior message at all.
          kind={withheld || company ? "email" : kind}
          // WHY THIS ONE IS BEING ANSWERED, where the caller knows. The drawer
          // reads the thread for itself; what it cannot read is the judgement
          // that put this message in front of a rep, and that judgement is what
          // the steer field is for.
          intent={proposed?.intent}
          draftOnOpen={proposed !== undefined}
          open={reply.open}
          onClose={() => setReply({ ...reply, open: false })}
          onSent={onSent}
          openedFrom={reply.from}
        />
      )}
    </>
  );
}

/** A target the server names and does not refuse. */
function offered(target: EmailTarget | undefined): boolean {
  return target !== undefined && target.refusal === undefined;
}

/** The company a written-about row's email goes to, when that is the target. */
function companyOf(target: EmailTarget | undefined): string | undefined {
  return target?.entity_type === "company" ? target.entity_id : undefined;
}

// A company has no one address, so the composer opens on the company itself
// and asks which of its contacts the mail is for. Every other target answers
// on the row.
function composerRecord({
  activityId,
  entityType,
  entityId,
  contactId,
  company,
}: Readonly<{
  activityId: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  contactId: string | undefined;
  company: string | undefined;
}>) {
  if (company !== undefined) {
    return {
      activityId: undefined,
      entityType: "company" as const,
      entityId: company,
      contactId: undefined,
    };
  }
  return { activityId, entityType, entityId, contactId };
}
