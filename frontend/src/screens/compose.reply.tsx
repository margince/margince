import { useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { ComposeModal } from "./compose";
import { useChannelReachable } from "./composereachability";
import type { RelinkKind } from "./composerelink";

type Activity = components["schemas"]["Activity"];

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
  channelProvider,
  entityType,
  entityId,
  contactId,
  contentWithheld,
  onSent,
}: Readonly<{
  activityId: string;
  kind: Activity["kind"];
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
}>) {
  const t = useT();
  // `null` until the verb is first pressed; the drawer stays mounted from then
  // on, which is what lets it animate out rather than vanish. `seq` counts the
  // opens and keys the composer, so each press gets a composer of its own — a
  // drawer that outlives its close would otherwise reopen on the draft the
  // reader abandoned, and offer to send it again.
  const [reply, setReply] = useState<{ seq: number; open: boolean } | null>(
    null,
  );
  const reachable = useChannelReachable(
    kind === "message",
    contactId,
    channelProvider,
  );
  if (!reachable) {
    return null;
  }
  return (
    <>
      <Button
        small
        onClick={() =>
          setReply((prior) => ({ seq: (prior?.seq ?? 0) + 1, open: true }))
        }
      >
        {contentWithheld ? t("compose.writeEmail") : t("compose.reply")}
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
          activityId={contentWithheld ? undefined : activityId}
          entityType={entityType}
          entityId={entityId}
          contactId={contactId}
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
          kind={contentWithheld ? "email" : kind}
          open={reply.open}
          onClose={() => setReply({ ...reply, open: false })}
          onSent={onSent}
        />
      )}
    </>
  );
}
