import { useContext, useEffect, useId, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { SendPermission } from "../../components/sendpermission";
import { useT } from "../../i18n";
import type { Transport } from "../../lib/contacttransports";
import { useVoiceProfile } from "../../lib/voice-profile";
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
import { ComposeBlockers } from "./composeblockers";
import { useDealChoice } from "./composedeal";
import { type ComposeDock, ComposeDockContext } from "./composedockcontext";
import { useComposeFields } from "./composefields";
import { useAccountRecipient, useComposeRecords } from "./composefiling";
import { groundingOf } from "./composelinks";
import type { ComposeProps } from "./composeprops";
import { useRecipientOffer } from "./composerecipientoffer";
import { useDraftRestore, useSavedDraft } from "./composesaveddraft";
import { savedFieldsOf } from "./composesaveddraftfields";

import { ScheduleDialog } from "./composeschedule";
import { useComposeSend } from "./composesend";
import { sendWording } from "./composesendhold";
import {
  useAnswerTarget,
  useConversationSwitch,
  useTargetFiles,
  useTransportDial,
} from "./composetarget";
import { ThreadBar } from "./composethreadbar";
import { useDraftOnOpen, useVoiceDraft } from "./composevoicedraft";
import { SendRefusalRemedies } from "./sendrefusal";
import "./compose.css";
import { ComposeHead } from "./compose.head";
import {
  ComposeAbout,
  ComposeBody,
  ComposeFoot,
  ComposeFrame,
  ReplyQuote,
} from "./compose.parts";

// The composer reviews replies, first messages and relinks before submitting.
export type Activity = components["schemas"]["Activity"];

export type VoiceProfile = components["schemas"]["VoiceProfile"];

// One frozen empty list for the default. A fresh array would remake the
// transport lookup on each keystroke's render.
const NO_TRANSPORTS: readonly Transport[] = [];

export type { ComposeProps } from "./composeprops";

/**
 * The composer, as every surface opens it. Inside the app's shell it hands
 * itself to the dock, so the message outlives the page that opened it.
 * Anywhere else (a story, a test) it draws in place.
 */
export function ComposeModal(props: ComposeProps) {
  const dock = useContext(ComposeDockContext);
  if (!dock) {
    return <ComposeWindow {...props} />;
  }
  return <DockedLauncher dock={dock} props={props} />;
}

// Opens and shuts this surface's composer in the dock. A surface that goes
// away with the composer open leaves it docked: that is what the dock is for.
function DockedLauncher({
  dock,
  props,
}: Readonly<{ dock: ComposeDock; props: ComposeProps }>) {
  const id = useId();
  const latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  });
  const { open } = props;
  useEffect(() => {
    if (open) {
      dock.show({ id, props: latest.current });
    } else {
      dock.hide(id);
    }
  }, [open, dock, id]);
  return null;
}

// The confirm-first composer for mail and for a captured channel. The rep's
// own click is the approval, so this path sends no approval token. A channel
// reply posts send-message and drops the subject, Cc and drafting.
export function ComposeWindow({
  initialMessage,
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
  // Whether the moment picker is up, apart from whether a moment is chosen.
  const [pickingMoment, setPickingMoment] = useState(false);
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
    setTo: fields.setTo,
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
      entityType,
      entityId: edit ? "" : entityId,
      isChannelReply,
    },
    open,
    fields: savedFieldsOf(fields, attached.files, {
      ...groundingOf({
        entityType,
        replying: !newMessage,
        contactIds: account.contactIds,
        dealId: fields.account.dealId,
        projectId,
      }),
      projectId: records.projectFiling.draftProjectId,
    }),
    replyTo: thread.anchorActivity,
    offeredRecipient: offer.standing.offeredAddress,
    initialIntent: askedIntent,
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
  const sendPending = send.send.isPending;
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
  const held = voice.rejectionInFlight || savedDraft.saving;
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
    anchorSubject: thread.anchorActivity?.subject,
    subject: fields.subject,
    recordName: records.recordName,
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
                rejectionInFlight={voice.rejectionInFlight}
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
                marksIn={markRow}
                attach={
                  <AttachAction
                    key={fileTarget}
                    entityType={entityType}
                    entityId={entityId}
                    chosen={attached.files}
                    onChange={attached.setFiles}
                    disabled={voice.rejectionInFlight}
                  />
                }
              />
            </div>
            <AttachedFiles
              chosen={attached.files}
              onChange={attached.setFiles}
              disabled={voice.rejectionInFlight}
            />
            <ComposeBlockers
              isChannelReply={isChannelReply}
              fields={fields}
              send={send}
              anchorActivity={thread.anchorActivity}
              dealChoice={dealChoice}
              bodyKey={wording.body}
              permission={(override, picksReason) => (
                <SendPermission
                  {...send.permission}
                  refusal={
                    send.permission.refusal && {
                      ...send.permission.refusal,
                      remedies: (
                        <SendRefusalRemedies
                          review={send.sendReview}
                          onDirectSent={send.directSent}
                          directOpen={send.directOpen}
                          onDirectOpenChange={send.setDirectOpen}
                        />
                      ),
                    }
                  }
                  override={override}
                  picksReason={picksReason}
                  namesRecipients
                  onRecheck={send.permission.recheck}
                />
              )}
            />
          </div>
          <ComposeFoot
            scheduling={scheduling}
            send={send}
            isChannelReply={isChannelReply}
            // The button stays live with fields outstanding, so a press can
            // name them. A rejection or a draft save in flight holds it.
            held={held}
            onPickMoment={() => setPickingMoment(true)}
            sendAt={fields.sendAt}
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

export { ChannelReplyAction } from "./compose.reply";
// Re-exported so writeto, recordemail, timelineactions, companyheader and
// composeattachments keep one import path.
export { RELINK_KINDS, type RelinkKind, RelinkModal } from "./composerelink";
