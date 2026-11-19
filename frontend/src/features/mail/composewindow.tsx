// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { useT } from "../../i18n";
import type { Transport } from "../../lib/contacttransports";
import { useVoiceProfile } from "../../lib/voice-profile";
import type { Activity } from "./compose";
import { ChannelDraftNotice } from "./compose.notices";
import {
  useComposeThread,
  useConversationNames,
  windowTitle,
} from "./composeanchor";
import {
  AttachAction,
  AttachedFiles,
  useCarriageBlocks,
} from "./composeattachments";
import { useKeptDraft } from "./composeautosave";
import {
  ComposeDeal,
  type DealChoiceState,
  useDealChoice,
} from "./composedeal";
import { type ComposeFields, useComposeFields } from "./composefields";
import { useAccountRecipient, useComposeRecords } from "./composefiling";
import { useInlineReview, useOverrideExplanation } from "./composeinlinereview";
import { groundingOf } from "./composelinks";
import type { ComposeProps } from "./composeprops";
import { useRecipientOffer } from "./composerecipientoffer";
import { useDraftRestore, useSavedDraft } from "./composesaveddraft";
import { savedFieldsOf } from "./composesaveddraftfields";
import { ScheduleDialogOnDemand } from "./composeschedule";
import { type ComposeSend, useComposeSend } from "./composesend";
import { InlineBlockers, markedRecipients } from "./composesendanyway";
import { sendWording } from "./composesendhold";
import {
  useAnswerTarget,
  useConversationSwitch,
  useTargetFiles,
  useTransportDial,
} from "./composetarget";
import { ThreadBar } from "./composethreadbar";
import { useDraftOnOpen, useVoiceDraft } from "./composevoicedraft";
import { ComposeWhy } from "./composewhy";
import { pickReason } from "./reasonanchor";
import "./compose.css";
import { ComposeHead } from "./compose.head";
import {
  ComposeAbout,
  ComposeBody,
  ComposeFoot,
  ComposeFrame,
  MailSendNotices,
  ReplyQuote,
} from "./compose.parts";

// One frozen empty list for the default. A fresh array would remake the
// transport lookup on each keystroke's render.
const NO_TRANSPORTS: readonly Transport[] = [];

// The confirm-first composer for mail and for a captured channel. The rep's
// own click is the approval, so this path sends no approval token. A channel
// reply posts send-message and drops the subject, Cc and drafting.
export function ComposeWindow({
  initialMessage,
  forwarding,
  initialReason,
  initialDealId,
  activityId,
  sourceBlocked: openingBlocked,
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
  draftOnOpen,
  edit,
  open,
  onClose,
  onSent,
  openedFrom,
}: ComposeProps) {
  const t = useT();
  // A shut composer asks for nothing it does not share with the page behind it.
  const voiceProfile = useVoiceProfile(open);
  const bodyId = useId();
  const dial = useTransportDial({ open, transports, initialTransportId, kind });
  const { isChannelReply } = dial;
  const reason = useOverrideExplanation(open);
  const fields = useComposeFields({
    initialMessage,
    initialReason,
    askedIntent,
    edit,
  });
  const { answering, choose, sourceBlocked } = useAnswerTarget({
    open,
    channelAnchorId: dial.channel?.anchorId,
    openingBlocked,
    activityId,
  });
  const fileTarget = answering ?? "";
  const attached = useTargetFiles(fileTarget, edit?.files);
  // Warn about carriage bounds before staging can refuse the message.
  const carriageBlocks = useCarriageBlocks(
    dial.channel?.id,
    attached.files,
    fields.body,
    open,
  );
  const momentPicker = useMomentPicker();
  // The foot's place for the editor's marks, so they sit beside Send.
  const [markRow, setMarkRow] = useState<HTMLDivElement | null>(null);
  // The account-started path is keyed on the resolved anchor: a record with
  // earlier mail answers it rather than asking for a recipient.
  const newMessage = !answering && !isChannelReply;
  const groundable = newMessage && entityType === "company";
  const accountCompanyId = groundable ? entityId : undefined;
  const account = useAccountRecipient(accountCompanyId, fields, {
    contactId,
    recordAddress,
  });
  const offer = useRecipientOffer({
    open,
    isChannelReply,
    answering,
    recordAddress: account.offeredAddress,
    to: fields.to,
    setTo: fields.setTo,
    copies: [...fields.cc, ...fields.bcc],
  });
  const names = useConversationNames(open, entityType, entityId);
  const thread = useComposeThread({
    open,
    answering,
    rowKind: answering === activityId ? kind : undefined,
    isChannelReply,
    entityType,
    entityId,
    nameOf: names.nameOf,
    setSubject: fields.setSubject,
  });
  const records = useComposeRecords({
    open,
    isChannelReply,
    entityType,
    entityId,
    recipientId: account.recipientId,
    answering,
    subject: fields.subject,
    setSubject: fields.setSubject,
    addressed: [...fields.to, ...fields.cc, ...fields.bcc],
    initialProjectId: edit?.projectId,
  });
  const projectId = records.projectFiling.projectId;
  const { settle } = offer;
  const { restoreSaved } = fields;
  const { adoptSavedSubject } = records.projectFiling;
  const restoreFields = useDraftRestore({
    settle,
    restoreSaved,
    adoptSavedSubject,
    setFiles: attached.setFiles,
    account: {
      setDealId: fields.account.setDealId,
      setProjectId: records.projectFiling.setProjectId,
    },
    setOverrideExplanation: reason.setText,
  });
  const dealChoice = useDealChoice({
    open,
    entityType,
    entityId,
    replying: !newMessage,
    context: fields.context,
    dealId: fields.account.dealId,
  });
  const grounding = groundingOf({
    entityType,
    replying: !newMessage,
    contactIds: account.contactIds,
    dealId: dealChoice.dealId,
    projectId,
  });
  const savedDraft = useSavedDraft({
    where: {
      answering,
      forwarding,
      prefilled: initialMessage !== undefined,
      entityType,
      entityId: edit ? "" : entityId,
      isChannelReply,
    },
    open,
    fields: savedFieldsOf(
      fields,
      attached.files,
      {
        ...groundingOf({
          entityType,
          replying: !newMessage,
          contactIds: account.contactIds,
          dealId: fields.account.dealId,
          projectId,
        }),
        projectId: records.projectFiling.draftProjectId,
      },
      reason.text,
    ),
    replyTo: thread.anchorActivity,
    offeredRecipient: offer.standing.offeredAddress,
    initialIntent: askedIntent,
    initialMessage,
    onRestore: restoreFields,
    onClose,
  });
  const send = useComposeSend({
    open,
    isChannelReply,
    answering,
    entityType,
    entityId,
    dealId: initialDealId,
    fields,
    files: attached.files,
    carriageBlocked: carriageBlocks.length > 0,
    sourceBlocked,
    anchorActivity: thread.anchorActivity,
    grounding,
    projectId,
    needsDeal: dealChoice.needsChoice,
    savedDraft,
    onSent,
    onClose,
  });
  const inlineReview = useInlineReview(send);
  const sendPending = inlineReview.sending;
  const voice = useVoiceDraft({
    open,
    fields,
    answering,
    entityType,
    entityId,
    followUp,
    projectId,
    recipient: account,
    groundable,
    thread,
    bodyId,
    voiceProfileId: voiceProfile.data?.id ?? null,
    sendPending,
    sourceBlocked,
  });
  useDraftOnOpen({
    open,
    asked: draftOnOpen,
    mail: !isChannelReply,
    settled: savedDraft.settled,
    held: savedDraft.held !== null,
    body: fields.body,
    control: voice.control,
  });
  const busy = sendPending || voice.rejectionInFlight;
  const locked = [voice.rejectionInFlight, inlineReview.pending].includes(true);
  const held = locked || savedDraft.saving;
  // Every hold on the Send button, so Cmd/Ctrl+Enter waits for the same ones.
  const sendBlocked = [
    held,
    sendPending,
    fields.addressRefused,
    send.hold.blocks,
  ].includes(true);
  useKeptDraft(savedDraft.keeper, sendPending);
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
  const title = windowTitle({
    isChannelReply,
    wordingTitle: t(wording.title),
    replying: thread.replying,
    anchor: thread.anchorActivity,
    subject: fields.subject,
    recordName: records.recordName,
    t,
  });
  return (
    <>
      <MomentDialog picker={momentPicker} fields={fields} />
      {open && (
        <ComposeFrame
          title={title}
          aside={
            <ComposeAbout
              kind={entityType}
              id={entityId}
              name={records.recordName}
            />
          }
          onClose={savedDraft.requestClose}
          closeDisabled={sendPending}
          bodyId={bodyId}
          recipientFirst={startsInTo(newMessage, account, fields.to)}
          openedFrom={openedFrom}
        >
          <ChannelDraftNotice
            requested={draftOnOpen}
            channel={isChannelReply}
          />
          {!isChannelReply && (
            <ThreadBar
              thread={thread}
              answering={answering}
              recordName={records.recordName}
              viewerId={names.viewerId}
              nameOf={names.nameOf}
              onSelect={selectMessage}
              disabled={busy}
            />
          )}
          <div ref={send.fieldsRef} className="compose-window">
            <div className="compose-sheet">
              <ComposeHead
                fields={fields}
                savedDraft={savedDraft}
                dial={dial}
                transports={transports}
                onTransportChange={changeTransport}
                staleThread={staleThread}
                answering={thread.replying ? answering : undefined}
                viewerId={names.viewerId}
                nameOf={names.nameOf}
                activityId={activityId}
                records={records}
                accountCompanyId={accountCompanyId}
                flagged={send.flagged}
                marked={markedRecipients(inlineReview.review, fields, t)}
                rejectionInFlight={locked}
                onToEditing={settle}
              />
              <ComposeBody
                bodyId={bodyId}
                fields={fields}
                mail={!isChannelReply}
                replying={thread.replying}
                earlier={
                  answering && <ReplyQuote message={thread.anchorActivity} />
                }
                voiceMaturity={voiceProfile.data?.maturity}
                channelLabel={dial.channel?.label}
                carriageBlocks={carriageBlocks}
                send={send}
                sendBlocked={sendBlocked}
                voice={voice}
                locked={locked}
                marksIn={markRow}
                attach={
                  <AttachAction
                    key={fileTarget}
                    entityType={entityType}
                    entityId={entityId}
                    chosen={attached.files}
                    onChange={attached.setFiles}
                    disabled={locked}
                  />
                }
              />
            </div>
            <AttachedFiles
              chosen={attached.files}
              onChange={attached.setFiles}
              disabled={locked}
            />
            <InlineBlockers
              isChannelReply={isChannelReply}
              refusal={inlineReview.refusal}
              sendUnavailable={fields.sendUnavailable}
              bodyKey={wording.body}
              state={inlineReview}
              explanation={reason.text}
              onExplanationChange={reason.setText}
            >
              <ComposeQuestions
                isChannelReply={isChannelReply}
                fields={fields}
                send={send}
                anchorActivity={thread.anchorActivity}
                dealChoice={dealChoice}
              />
            </InlineBlockers>
          </div>
          <ComposeFoot
            scheduling={scheduling}
            send={send}
            isChannelReply={isChannelReply}
            // The button stays live with fields outstanding, so a press can
            // name them. A rejection or a draft save in flight holds it.
            held={held}
            onPickMoment={momentPicker.pick}
            {...fields.scheduleControl}
            savedDraft={savedDraft}
            busy={busy}
            addressRefused={fields.addressRefused}
            marksRef={setMarkRow}
          />
        </ComposeFrame>
      )}
    </>
  );
}

// Whether the moment picker is up, apart from whether a moment is chosen.
// Null until it first opens: its code loads then.
function useMomentPicker() {
  const [open, setOpen] = useState<boolean | null>(null);
  return { open, pick: () => setOpen(true), close: () => setOpen(false) };
}

function MomentDialog({
  picker,
  fields,
}: Readonly<{
  picker: ReturnType<typeof useMomentPicker>;
  fields: ComposeFields;
}>) {
  const now = new Date();
  return (
    <ScheduleDialogOnDemand
      open={picker.open}
      onClose={picker.close}
      {...fields.scheduleControl}
      onChoose={fields.setSendAt}
      now={now}
      sampledAt={now.getTime()}
    />
  );
}

// The cursor starts in To on a new message with no address on its way there.
function startsInTo(
  newMessage: boolean,
  account: Readonly<{ nothingOffered: boolean }>,
  to: readonly string[],
): boolean {
  return newMessage && account.nothingOffered && to.length === 0;
}

// What the message must say before Send: why it is written, and the deal a
// follow-up is about. Nothing is checked until Send is pressed.
function ComposeQuestions({
  isChannelReply,
  fields,
  send,
  anchorActivity,
  dealChoice,
}: Readonly<{
  isChannelReply: boolean;
  fields: ComposeFields;
  send: ComposeSend;
  anchorActivity: Activity | undefined;
  dealChoice: DealChoiceState;
}>) {
  return (
    <>
      <ComposeWhy
        anchorActivity={anchorActivity}
        needsReason={fields.context !== ""}
        flagged={send.flagged}
        context={fields.context}
        onContextChange={pickReason(fields)}
      />
      <ComposeDeal
        choice={dealChoice}
        onDealChange={fields.account.setDealId}
        flagged={send.flagged}
      />
      {!isChannelReply && (
        <MailSendNotices
          to={fields.to}
          cc={fields.cc}
          context={send.claimedContext}
          newContacts={[]}
        />
      )}
    </>
  );
}
