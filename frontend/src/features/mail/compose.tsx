import { useCallback, useId, useState } from "react";
import type { components } from "../../api/schema";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Transport } from "../../lib/contacttransports";
import { useVoiceProfile } from "../../lib/voice-profile";
import {
  colleagueMailboxesOf,
  useComposeThread,
  useConversationNames,
} from "./composeanchor";
import { useCarriageBlocks } from "./composeattachments";
import { SendChecks } from "./composechecks";
import { useComposeFields } from "./composefields";
import { useComposeRecords } from "./composefiling";
import { useRecipientOffer } from "./composerecipientoffer";
import type { RelinkKind } from "./composerelink";
import { type SavedDraftFields, useSavedDraft } from "./composesaveddraft";
import { ScheduleDialog, ScheduleMenu } from "./composeschedule";
import { useComposeSend } from "./composesend";
import {
  useAnswerTarget,
  useConversationSwitch,
  useTargetFiles,
  useTransportDial,
} from "./composetarget";
import { useVoiceDraft } from "./composevoicedraft";
import type { EditSeed } from "./schedulededit";
import "./compose.css";
import { ComposeHead, ProjectFiling } from "./compose.head";
import {
  ComposeActionsLead,
  ComposeBody,
  ComposeConversation,
  ComposeOutcome,
  ComposeWhy,
} from "./compose.parts";

// The composer reviews replies, first messages and relinks before submitting.
export type Activity = components["schemas"]["Activity"];

export type VoiceProfile = components["schemas"]["VoiceProfile"];

// One frozen empty list for the default. A fresh array would remake the
// transport lookup on each keystroke's render.
const NO_TRANSPORTS: readonly Transport[] = [];

// What the dialog promises. A scheduled message waits and can be withdrawn,
// so its title and body say so; a channel reply has no send-later field.
function sendWording(
  isChannelReply: boolean,
  scheduling: boolean,
): Readonly<{ title: MessageKey; body: MessageKey }> {
  if (isChannelReply) {
    return {
      title: "compose.sendMessageConfirmTitle",
      body: "compose.sendMessageBody",
    };
  }
  if (scheduling) {
    return {
      title: "compose.scheduleConfirmTitle",
      body: "compose.scheduleBody",
    };
  }
  return { title: "compose.sendConfirmTitle", body: "compose.sendBody" };
}

// The dialog's head. A reply is named for the thread it answers. A new message
// is named for its subject as typed, until it has one. Both file under a
// project, while a channel reply keeps the dialog's own title and files nothing.
function dialogHead(
  input: Readonly<{
    isChannelReply: boolean;
    wordingTitle: string;
    answering: string | undefined;
    anchorSubject: string | null | undefined;
    subject: string;
    t: ReturnType<typeof useT>;
  }>,
): { eyebrow?: string; title: string; anchorSubject: string; filed: boolean } {
  const { t } = input;
  const anchorSubject = input.anchorSubject?.trim() || t("email.noSubject");
  if (input.isChannelReply) {
    return { title: input.wordingTitle, anchorSubject, filed: false };
  }
  if (input.answering) {
    return {
      eyebrow: t("compose.reply"),
      title: anchorSubject,
      anchorSubject,
      filed: true,
    };
  }
  return {
    eyebrow: t("compose.newEmail"),
    title: input.subject.trim() || t("compose.newThread"),
    anchorSubject,
    filed: true,
  };
}

// What a model's draft is written from, said under the ask for one.
function aiSourceLine(
  anchorSubject: string,
  answering: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  return answering
    ? t("compose.aiSourceReply", { subject: anchorSubject })
    : t("compose.aiSourceNew");
}

// The confirm-first composer for mail and for a captured channel. The rep's
// own click is the approval, so this path sends no approval token. A channel
// reply posts send-message and drops the subject, Cc and drafting.
export function ComposeModal({
  initialMessage,
  activityId,
  entityType,
  entityId,
  contactId,
  followUp,
  recordAddress,
  kind,
  transports = NO_TRANSPORTS,
  initialTransportId,
  staleThread = false,
  intent: askedIntent,
  edit,
  open,
  onClose,
  onSent,
}: Readonly<{
  // Absent means an ACCOUNT-STARTED send: a new conversation (ADR-0087).
  // Drafting, consent, refusal and provenance are shared with a reply. A
  // reply keeps threading and inherits its links. A new message files itself
  // under the record it started from.
  initialMessage?: Readonly<{ subject: string; body: string }>;
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  contactId?: string;
  /** A meeting follow-up: further recipients and the meeting it follows up. */
  followUp?: Parameters<typeof useVoiceDraft>[0]["followUp"];
  /**
   * The record's own email address, for a FIRST message to it — a lead or a
   * contact nobody has written to yet, where there is no thread to resolve a
   * counterparty from.
   *
   * The caller's, because the record is already on screen behind this drawer
   * and its address came with it: a lookup here would ask the server for a
   * field the page is holding. Offered only when nothing is being answered,
   * and offered the same way a thread's address is — into an empty field,
   * once, and never over what the reader typed.
   */
  recordAddress?: string;
  // Undefined (or any non-channel kind) keeps the mail behaviour this modal
  // already had before a channel existed — every mail test renders this
  // component without ever naming a kind.
  kind?: Activity["kind"];
  /**
   * The ways this record can be written to, when there is more than one.
   *
   * The CALLER's, because reachability is a fact about a contact and the record
   * page is already holding the payload it is read from — asking again here
   * would let the drawer and the verb that opened it disagree about what
   * pressing it does. A company, a deal and a lead pass none and get the mail
   * composer they have always had; only a contact reachable on a chat channel
   * has a choice to offer.
   */
  transports?: readonly Transport[];
  /** Which of them to open on — the transport the conversation a caller named
   *  is ON, rather than the record's own lead. The reader may still change it. */
  initialTransportId?: string;
  /** The named conversation cannot be answered any more. The composer opens
   *  on the default AND says so, rather than fall back silently. */
  staleThread?: boolean;
  /** What the caller wants the draft to be ABOUT. A moment action knows why it
   *  fired — "their reply is overdue", "the meeting needs an agenda" — and that
   *  reason shaped nothing until it reached the steer field. */
  intent?: string;
  /** A scheduled message written again. Read when the composer mounts, so the
   *  caller keys the modal on the message. It keeps no saved draft: the
   *  words are the scheduled message's, not a draft's. */
  edit?: EditSeed;
  open: boolean;
  onClose: () => void;
  // A message left this composer — sent now or scheduled. Called before the
  // close, and only then: a caller whose own reading of the record depends on
  // what was sent re-asks on this, and not on a cancel that changed nothing.
  onSent?: () => void;
}>) {
  const t = useT();
  // A shut composer asks for nothing it does not share with the page behind it.
  const voiceProfile = useVoiceProfile(open);
  const bodyId = useId();
  const dial = useTransportDial({ open, transports, initialTransportId, kind });
  const { isChannelReply } = dial;
  const fields = useComposeFields({
    initialMessage,
    askedIntent,
    contactId,
    edit,
  });
  const { answering, choose } = useAnswerTarget({
    open,
    channelAnchorId: dial.channel?.anchorId,
    activityId,
  });
  const fileTarget = answering ?? "";
  const attached = useTargetFiles(fileTarget);
  // Warn about carriage bounds before staging can refuse the message.
  const carriageBlocks = useCarriageBlocks(
    dial.channel?.id,
    attached.files,
    fields.body,
    open,
  );
  // Whether the moment picker is up, apart from whether a moment is chosen.
  const [pickingMoment, setPickingMoment] = useState(false);
  const offer = useRecipientOffer({
    open,
    isChannelReply,
    answering,
    recordAddress,
    setTo: fields.setTo,
  });
  const names = useConversationNames(open, entityType, entityId);
  const thread = useComposeThread({
    open,
    answering,
    isChannelReply,
    entityType,
    entityId,
    nameOf: names.nameOf,
    setSubject: fields.setSubject,
  });
  // The account-started path is keyed on the resolved anchor: a record with
  // earlier mail answers it rather than asking for a recipient.
  const groundable = !answering && entityType === "company" && !isChannelReply;
  const records = useComposeRecords({
    open,
    isChannelReply,
    entityType,
    entityId,
    recipientId: fields.account.recipientId,
    answering,
    subject: fields.subject,
    setSubject: fields.setSubject,
    addressed: [...fields.to, ...fields.cc, ...fields.bcc],
  });
  const projectId = records.projectFiling.projectId;
  const { settle } = offer;
  const { restoreSaved } = fields;
  const restoreFields = useCallback(
    (saved: SavedDraftFields) => {
      settle();
      restoreSaved(saved);
    },
    [settle, restoreSaved],
  );
  const savedDraft = useSavedDraft({
    where: {
      answering,
      entityType,
      entityId: edit ? "" : entityId,
      isChannelReply,
    },
    open,
    fields: {
      to: fields.to,
      cc: fields.cc,
      bcc: fields.bcc,
      subject: fields.subject,
      body: fields.body,
      html: fields.html,
    },
    replyTo: thread.anchorActivity,
    offeredRecipient: offer.standing.offeredAddress,
    onRestore: restoreFields,
    onClose,
  });
  const send = useComposeSend({
    open,
    isChannelReply,
    answering,
    entityType,
    entityId,
    fields,
    files: attached.files,
    carriageBlocked: carriageBlocks.length > 0,
    anchorActivity: thread.anchorActivity,
    grounding: groundable ? { ...fields.account.grounding, projectId } : null,
    projectId,
    savedDraft,
    onSent,
    onClose,
  });
  const sendPending = send.send.isPending;
  const voice = useVoiceDraft({
    open,
    fields,
    answering,
    entityType,
    entityId,
    followUp,
    projectId,
    groundable,
    thread,
    bodyId,
    voiceProfileId: voiceProfile.data?.id ?? null,
    sendPending,
  });
  const busy = sendPending || voice.rejectionInFlight;
  const selectMessage = useConversationSwitch({
    answering,
    choose,
    fields,
    offer,
    busy,
    resetRequests: () => {
      voice.draft.reset();
      fields.setDraftKept(false);
      send.send.reset();
      send.clearAttempt();
    },
  });
  // A new transport is a new conversation, and the offer is per conversation.
  const changeTransport = (next: string) => {
    fields.clearForTransport();
    dial.turn(next);
    offer.rearm();
  };
  const scheduling = !isChannelReply && fields.sendAt !== "";
  const wording = sendWording(isChannelReply, scheduling);
  // Whether the checks are open, as the reader leaves them.
  const [checking, setChecking] = useState(false);
  const head = dialogHead({
    isChannelReply,
    wordingTitle: t(wording.title),
    answering,
    anchorSubject: thread.anchorActivity?.subject,
    subject: fields.subject,
    t,
  });
  return (
    <>
      <ScheduleDialog
        open={pickingMoment}
        onClose={() => setPickingMoment(false)}
        sendAt={fields.sendAt}
        onChoose={fields.setSendAt}
        now={new Date()}
      />
      <ConfirmModal
        initialFocusTo={() => document.getElementById(bodyId)}
        open={open}
        onClose={savedDraft.requestClose}
        title={head.title}
        eyebrow={head.eyebrow}
        headAside={<ProjectFiling filed={head.filed} records={records} />}
        tier="confirm"
        // Mail is worked in at length with its conversations beside it.
        intent={dial.asWorkspace ? "workspace" : "form"}
        confirmLabel={t(scheduling ? "compose.schedule" : "compose.send")}
        // The button stays live with fields outstanding, so a press can name
        // them. A rejection or a draft save in flight disables it.
        confirmDisabled={voice.rejectionInFlight || savedDraft.saving}
        onConfirm={send.confirm}
        pending={sendPending}
        error={send.sendError ?? savedDraft.error}
        actionsLead={
          <ComposeActionsLead
            isChannelReply={isChannelReply}
            savedDraft={savedDraft}
            busy={busy}
            checks={
              <SendChecks
                fields={fields}
                send={send}
                anchorActivity={thread.anchorActivity}
                contactOf={records.contactOf}
                bodyKey={wording.body}
                open={checking}
                onOpenChange={setChecking}
              />
            }
          />
        }
        confirmMenu={
          isChannelReply ? undefined : (
            <ScheduleMenu onOpen={() => setPickingMoment(true)} />
          )
        }
      >
        {/* The form uses the full width when no conversation is available. */}
        <div
          ref={send.fieldsRef}
          className={thread.splitColumns ? "compose-split" : undefined}
        >
          <ComposeConversation
            thread={thread}
            answering={answering}
            recordName={records.recordName}
            viewerId={names.viewerId}
            nameOf={names.nameOf}
            onSelect={selectMessage}
            disabled={busy}
          />
          <div className="compose-fields">
            <ComposeHead
              fields={fields}
              savedDraft={savedDraft}
              dial={dial}
              transports={transports}
              onTransportChange={changeTransport}
              staleThread={staleThread}
              colleagueSeats={colleagueMailboxesOf(
                offer.mailboxes,
                names.viewerId,
              )}
              nameOf={names.nameOf}
              activityId={activityId}
              records={records}
              accountCompanyId={groundable ? entityId : undefined}
              answeringLine={thread.answeringLine}
              replying={Boolean(answering)}
              railShown={thread.splitColumns}
              onStartNew={() => selectMessage(null)}
              flagged={send.flagged}
              rejectionInFlight={voice.rejectionInFlight}
              onToEditing={settle}
            />
            <ComposeWhy
              anchorActivity={thread.anchorActivity}
              flagged={send.flagged}
              context={fields.context}
              onContextChange={fields.setContext}
            />
            <ComposeBody
              bodyId={bodyId}
              fields={fields}
              attached={attached}
              fileTarget={fileTarget}
              entityType={entityType}
              entityId={entityId}
              mail={open && !isChannelReply}
              replying={Boolean(answering)}
              aiSource={aiSourceLine(head.anchorSubject, answering, t)}
              voiceMaturity={voiceProfile.data?.maturity}
              channelLabel={dial.channel?.label}
              carriageBlocks={carriageBlocks}
              flagged={send.flagged}
              voice={voice}
            />
            <ComposeOutcome
              isChannelReply={isChannelReply}
              bodyKey={wording.body}
              sendUnavailable={fields.sendUnavailable}
              send={send}
              contactId={contactId}
            />
          </div>
        </div>
      </ConfirmModal>
    </>
  );
}

export { ChannelReplyAction } from "./compose.reply";
// Re-exported so writeto, recordemail, timelineactions, companyheader and
// composeattachments keep one import path.
export { RELINK_KINDS, type RelinkKind, RelinkModal } from "./composerelink";
