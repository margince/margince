import { useCallback, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import {
  type PickableProject,
  ProjectPicker,
} from "../design-system/projectpicker";
import { RichText } from "../design-system/richtext";
import { Select } from "../design-system/select";
import type { TokenSuggestion } from "../design-system/tokeninput";
import { INTL_LOCALE } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { CarriageViolation } from "./carriage";
import { StaleThreadNotice } from "./compose.notices";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "./compose-context";
import {
  type ComposeThread,
  colleagueMailboxesOf,
  useComposeThread,
  useConversationNames,
  useThreadProject,
} from "./composeanchor";
import {
  AttachAction,
  AttachedFiles,
  CarriageNotice,
  useCarriageBlocks,
} from "./composeattachments";
import { ConversationFold } from "./composeconversation";
import { DraftBand, RewriteRow } from "./composedraftband";
import type { DraftProvenance, DraftUnavailable } from "./composedraftcall";
import {
  AccountDraftContext,
  DraftOffer,
  type PendingAction,
} from "./composedraftcontext";
import { type ComposeFields, useComposeFields } from "./composefields";
import { type ComposeRecords, useComposeRecords } from "./composefiling";
import {
  AddressBlock,
  FieldNeed,
  SubjectRow,
  TransportRow,
} from "./composehead";
import { useRecipientOffer } from "./composerecipientoffer";
import { RELINK_KINDS, type RelinkKind, RelinkModal } from "./composerelink";
import {
  type SavedDraftFields,
  SavedDraftNotices,
  useSavedDraft,
} from "./composesaveddraft";
import {
  momentLabel,
  momentOf,
  ScheduleDialog,
  ScheduleMenu,
} from "./composeschedule";
import {
  type ComposeSend,
  type MissingField,
  useComposeSend,
} from "./composesend";
import { SignOffPreview } from "./composesignoff";
import {
  useAnswerTarget,
  useConversationSwitch,
  useTargetFiles,
  useTransportDial,
} from "./composetarget";
import { ConversationChoices, ThreadPane } from "./composethread";
import { useVoiceDraft, type VoiceDraft } from "./composevoicedraft";
import type { Transport } from "./contacttransports";
import { useProjectRecord } from "./projectrecord";
import { useRichTextLabels } from "./richtextlabels";
import { SendMark, SendPermission } from "./sendpermission";
import { SendRefusal } from "./sendrefusal";
import { useVoiceProfile } from "./voice-profile";
import "./compose.css";

// The composer reviews replies, first messages and relinks before submitting.
type Activity = components["schemas"]["Activity"];
type VoiceProfile = components["schemas"]["VoiceProfile"];

/**
 * The one control that says which project a message belongs to — on a reply
 * and on a message started from an account alike.
 *
 * It carries no explanation of its own. The tag it puts in the Subject field
 * is visible there, and a rep who does not want it deletes it like any other
 * text; a sentence saying so would restate what the field already shows.
 *
 * Choosing None takes the tag out. Choosing a project puts it in. Nothing
 * renders when the record reaches no live project, because a list whose only
 * entry is None asks a question with one answer.
 */
function ProjectFiling({
  projects,
  projectId,
  onChange,
}: Readonly<{
  projects: readonly PickableProject[];
  projectId: string;
  onChange: (next: string) => void;
}>) {
  return (
    <ProjectPicker
      projects={projects}
      projectId={projectId}
      onChange={onChange}
    />
  );
}

/**
 * Where a CHANNEL reply will be filed, said rather than chosen.
 *
 * A channel send carries the words and the consent purpose and nothing else
 * (SendMessageRequest has no links and no subject), so the server files the
 * reply under the links of the conversation it answers. A picker here would
 * collect an answer nothing could carry: mail keeps its choice in the subject
 * tag, and a channel has no subject line to keep it in. So this states the
 * filing rather than asking for one.
 *
 * It reads the ANCHOR's own project link rather than the picker's list, because
 * that link is what the send inherits — a channel conversation hangs off a
 * contact, whose timeline reaches no project list at all, and it is filed all
 * the same.
 *
 * Nothing renders while a read is unanswered, and nothing renders when the
 * conversation names no project: an unsettled read is not "no project", and
 * naming a project the send then contradicts is worse than the silence.
 */
function ChannelReplyFiling({ activityId }: Readonly<{ activityId?: string }>) {
  const t = useT();
  const thread = useThreadProject(activityId);
  const { project, settled } = useProjectRecord(thread.projectId);
  if (!thread.settled || !settled || !project) {
    return null;
  }
  return (
    <p>
      {t("compose.channelFiling", {
        // The same shape the picker labels an option with, so the project a rep
        // reads here and the one they read on a mail reply are one name.
        project: project.key
          ? `${project.key} · ${project.name}`
          : project.name,
      })}
    </p>
  );
}

// sharedUnsubscribeAhead predicts the sharedUnsubscribe refusal (refusalOf).
// A marketing send's unsubscribe link is one addressee's consent record, so a
// second addressee is refused. The server rule stays the authority, keyed on
// the marketing category, which a reply never carries.
function sharedUnsubscribeAhead(
  to: string[],
  cc: string[],
  context: CommunicationContext | undefined,
): boolean {
  if (context !== "marketing") {
    return false;
  }
  const addressees = new Set(
    [...to, ...cc].map((address) => address.trim().toLowerCase()),
  );
  return addressees.size > 1;
}

// The mail-only half of the composer: AI drafting (there is no draft-message
// endpoint for a channel) plus the recipient/subject inputs a channel reply's
// request shape has no room for. Kept as its own component so a channel
// reply — which renders none of this — doesn't inherit its branching.
function MailOnlyFields({
  replying,
  intent,
  onIntentChange,
  draft,
  draftUnavailable,
  provenance,
  voiceMaturity,
  reasons,
  to,
  onToChange,
  cc,
  onCcChange,
  bcc,
  onBccChange,
  bccOpen,
  onOpenBcc,
  recipients,
  onToEditing,
  subject,
  onSubjectChange,
  rejectionInFlight,
  answering,
  flagged,
  deadRecipients,
}: Readonly<{
  replying: boolean;
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  draftUnavailable: DraftUnavailable | null;
  provenance: DraftProvenance | null;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  to: string[];
  onToChange: (next: string[]) => void;
  cc: string[];
  onCcChange: (next: string[]) => void;
  bcc: string[];
  onBccChange: (next: string[]) => void;
  /** Whether the blind-copy row is drawn. A button until it is asked for. */
  bccOpen: boolean;
  onOpenBcc: () => void;
  /** The addresses this record already knows, offered as the reader types. */
  recipients: readonly TokenSuggestion[];
  /** The reader has started typing a recipient, before any is committed. */
  onToEditing: () => void;
  subject: string;
  onSubjectChange: (next: string) => void;
  /** The fields a pressed Send is still waiting for. Empty until it is pressed. */
  flagged: ReadonlySet<MissingField>;
  rejectionInFlight: boolean;
  /** The reply target or the current loading/error state. */
  answering: string;
  /** The recipients on this draft that are known not to arrive. */
  deadRecipients: readonly string[];
}>) {
  const t = useT();
  const offer = (
    <DraftOffer
      replying={replying}
      intent={intent}
      onIntentChange={onIntentChange}
      draft={draft}
      unavailable={draftUnavailable}
    />
  );
  return (
    <>
      {provenance ? (
        <DraftBand
          provenance={provenance}
          maturity={voiceMaturity}
          reasons={reasons}
        >
          {offer}
        </DraftBand>
      ) : (
        offer
      )}
      <p className="t-caption mw-answering">{answering}</p>
      <AddressBlock
        to={to}
        onToChange={onToChange}
        cc={cc}
        onCcChange={onCcChange}
        bcc={bcc}
        onBccChange={onBccChange}
        bccOpen={bccOpen}
        onOpenBcc={onOpenBcc}
        suggestions={recipients}
        onToEditing={onToEditing}
        invalidTo={flagged.has("to")}
        needTo={t("compose.emptyRecipients")}
        deadRecipients={deadRecipients}
        disabled={rejectionInFlight}
      />
      <SubjectRow
        subject={subject}
        onChange={onSubjectChange}
        invalid={flagged.has("subject")}
        need={t("compose.missingSubject")}
        disabled={rejectionInFlight}
      />
    </>
  );
}

// The mail-only send-time notice: a shared-unsubscribe-token risk. The concept
// does not exist on a channel reply — there is no addressee list to warn
// about — so this renders nothing there.
//
// The empty-recipient line used to live here too, at the foot of the drawer,
// and now belongs to the To field: said in both places it read as a stutter,
// and the copy at the bottom was the one furthest from the thing to fix.
function MailSendNotices({
  to,
  cc,
  context,
}: Readonly<{
  to: string[];
  cc: string[];
  context: CommunicationContext | undefined;
}>) {
  const t = useT();
  return (
    <>
      {sharedUnsubscribeAhead(to, cc, context) && (
        <ErrorLine standing>{t("compose.multiRecipientWarning")}</ErrorLine>
      )}
    </>
  );
}

export { ChannelReplyAction } from "./compose.reply";
// Re-exported so writeto, recordemail, timelineactions, companyheader and
// composeattachments keep one import path.
export { RELINK_KINDS, type RelinkKind, RelinkModal };

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

// The footer's lead: the permission mark a rep checks before pressing
// anything, then discarding the draft and saving it.
function ComposeActionsLead({
  isChannelReply,
  permission,
  discardControl,
  savedDraft,
  busy,
}: Readonly<{
  isChannelReply: boolean;
  permission: ComposeSend["permission"];
  discardControl: VoiceDraft["discardControl"];
  savedDraft: ReturnType<typeof useSavedDraft>;
  /** A send or a rejection is in flight. */
  busy: boolean;
}>) {
  const t = useT();
  return (
    <>
      {!isChannelReply && (
        <SendMark
          preview={permission.preview}
          asking={permission.asking}
          unanswered={permission.unanswered}
        />
      )}
      {discardControl && (
        <Button
          onClick={discardControl.run}
          disabled={discardControl.disabled}
          // Discarding tells the voice profile this draft missed; it is not
          // an undo for clearing the box.
          title={t("compose.discardDraftHint")}
        >
          {t("compose.discardDraft")}
        </Button>
      )}
      {savedDraft.enabled && (
        <Button
          onClick={savedDraft.save}
          pending={savedDraft.saving}
          disabled={busy}
        >
          {t("compose.saveDraft")}
        </Button>
      )}
    </>
  );
}

// The column beside the form: the conversations on offer, or the thread
// being answered.
function ComposeConversation({
  thread,
  answering,
  viewerId,
  nameOf,
  onSelect,
  disabled,
}: Readonly<{
  thread: ComposeThread;
  answering: string | undefined;
  viewerId: string | undefined;
  nameOf: (linkType: string, linkId: string) => string | undefined;
  onSelect: (id: string | null) => void;
  disabled: boolean;
}>) {
  if (!thread.splitColumns) {
    return null;
  }
  const { recent, conversation, anchorRead } = thread;
  return (
    <ConversationFold choosing={thread.showChoices}>
      {thread.showChoices && (
        <ConversationChoices
          conversations={recent.conversations}
          pending={recent.pending}
          failed={recent.failed}
          onRetry={recent.retry}
          onChoose={onSelect}
        />
      )}
      {thread.showConversation && (
        <ThreadPane
          messages={conversation.messages}
          pending={conversation.pending || thread.anchorUnresolved}
          failed={conversation.failed || anchorRead.failed}
          onRetry={anchorRead.failed ? anchorRead.retry : conversation.retry}
          viewerUserId={viewerId}
          nameOf={nameOf}
          named
          onLeave={() => onSelect(null)}
          selectedId={answering}
          onSelect={onSelect}
          disabled={disabled}
        />
      )}
    </ConversationFold>
  );
}

// Whose conversation this is, when colleagues' mailboxes took it. Covering
// for a colleague is ordinary work, so it states the fact without a warning
// or a live region.
function ColleagueMailboxNotice({
  seats,
  nameOf,
}: Readonly<{
  seats: readonly string[];
  nameOf: (linkType: string, linkId: string) => string | undefined;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (seats.length === 0) {
    return null;
  }
  return (
    <Callout kind="standing" title={t("compose.colleagueMailboxTitle")}>
      {plural("compose.colleagueMailbox", seats.length, {
        names: new Intl.ListFormat(INTL_LOCALE[locale], {
          style: "long",
          type: "conjunction",
        }).format(
          seats.map(
            (seat) => nameOf("user", seat) ?? t("compose.colleagueUnnamed"),
          ),
        ),
      })}
    </Callout>
  );
}

// The head the reader scans: how the message goes, whose conversation it is,
// where it files, and the mail-only drafting and address fields.
function ComposeHead({
  fields,
  savedDraft,
  dial,
  transports,
  onTransportChange,
  staleThread,
  colleagueSeats,
  nameOf,
  activityId,
  records,
  accountCompanyId,
  answeringLine,
  replying,
  flagged,
  voice,
  voiceMaturity,
  onToEditing,
}: Readonly<{
  fields: ComposeFields;
  savedDraft: ReturnType<typeof useSavedDraft>;
  dial: ReturnType<typeof useTransportDial>;
  transports: readonly Transport[];
  onTransportChange: (next: string) => void;
  staleThread: boolean;
  colleagueSeats: readonly string[];
  nameOf: (linkType: string, linkId: string) => string | undefined;
  activityId?: string;
  records: ComposeRecords;
  /** The company an account-started message is grounded in, if any. */
  accountCompanyId: string | undefined;
  answeringLine: string;
  replying: boolean;
  flagged: ReadonlySet<MissingField>;
  voice: VoiceDraft;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
  onToEditing: () => void;
}>) {
  const t = useT();
  const { account } = fields;
  return (
    <>
      <SavedDraftNotices draft={savedDraft} />
      {fields.draftKept && <p role="status">{t("compose.draftKept")}</p>}
      {/* How this goes comes first: a channel carries no subject and names
          no addressee, so the choice governs the fields below it. */}
      <TransportRow
        transports={transports}
        selected={dial.transport}
        onChange={onTransportChange}
      />
      <StaleThreadNotice stale={staleThread} />
      <ColleagueMailboxNotice seats={colleagueSeats} nameOf={nameOf} />
      {/* Mail asks where it files, in the subject tag. A channel send carries
          no filing field, so it is told what the conversation inherits. */}
      {dial.isChannelReply ? (
        <ChannelReplyFiling activityId={activityId} />
      ) : (
        <ProjectFiling
          projects={records.reachableProjects}
          projectId={records.projectFiling.projectId}
          onChange={records.projectFiling.setProjectId}
        />
      )}
      {/* The account path's recipient and deal sit with the rows that say
          what the message is. */}
      {accountCompanyId !== undefined && (
        <AccountDraftContext
          companyId={accountCompanyId}
          recipientId={account.recipientId}
          onRecipientChange={account.setRecipientId}
          dealId={account.dealId}
          onDealChange={account.setDealId}
        />
      )}
      {/* Drafting and addresses are mail-only: there is no draft-message
          endpoint, and a channel resolves its recipient on the server. */}
      {!dial.isChannelReply && (
        <MailOnlyFields
          // The pane answers what is being replied to; the sentence serves
          // the composer that has no pane.
          answering={answeringLine}
          replying={replying}
          flagged={flagged}
          intent={fields.intent}
          onIntentChange={fields.setIntent}
          draft={voice.control}
          draftUnavailable={fields.draftUnavailable}
          provenance={fields.provenance}
          voiceMaturity={voiceMaturity}
          reasons={account.reasoning}
          to={fields.to}
          onToChange={fields.setTo}
          cc={fields.cc}
          onCcChange={fields.setCc}
          bcc={fields.bcc}
          onBccChange={fields.setBcc}
          bccOpen={fields.bccOpen}
          onOpenBcc={fields.openBcc}
          recipients={records.recipients}
          onToEditing={onToEditing}
          subject={fields.subject}
          onSubjectChange={fields.setSubject}
          rejectionInFlight={voice.rejectionInFlight}
          deadRecipients={records.deadRecipients}
        />
      )}
    </>
  );
}

// The words the reader writes, with what travels with them: the sign-off,
// the attached files, and the rewrite offer over a model's untouched draft.
function ComposeBody({
  bodyId,
  fields,
  attached,
  fileTarget,
  entityType,
  entityId,
  signs,
  channelLabel,
  carriageBlocks,
  flagged,
  voice,
}: Readonly<{
  bodyId: string;
  fields: ComposeFields;
  attached: ReturnType<typeof useTargetFiles>;
  fileTarget: string;
  entityType: RelinkKind;
  entityId: string;
  /** Whether the send appends a sign-off; a channel message carries none. */
  signs: boolean;
  channelLabel: string | undefined;
  carriageBlocks: readonly CarriageViolation[];
  flagged: ReadonlySet<MissingField>;
  voice: VoiceDraft;
}>) {
  const t = useT();
  const richTextLabels = useRichTextLabels();
  const frozen = voice.rejectionInFlight;
  return (
    <>
      {/* Both renderings travel. A text client, a screen reader and a spam
          filter read the plain part. */}
      <RichText
        id={bodyId}
        value={fields.html}
        onChange={fields.editBody}
        label={t("compose.body")}
        placeholder={t("compose.bodyPlaceholder")}
        labels={richTextLabels}
        hint={t("compose.bodyHint")}
        actions={
          <AttachAction
            key={fileTarget}
            entityType={entityType}
            entityId={entityId}
            chosen={attached.files}
            onChange={attached.setFiles}
            disabled={frozen}
          />
        }
        rows={10}
        disabled={frozen}
      />
      <FieldNeed show={flagged.has("body")} need={t("compose.missingBody")} />
      {signs && <SignOffPreview body={fields.body} subject={fields.subject} />}
      {/* Attachments sit under the words they belong to, as in a mail client. */}
      <AttachedFiles
        chosen={attached.files}
        onChange={attached.setFiles}
        disabled={frozen}
      />
      <CarriageNotice channel={channelLabel} blocks={carriageBlocks} />
      {/* A rewrite replaces the body, so it is offered over the model's own
          untouched words. A draft in flight blocks it too. */}
      {fields.body !== "" && fields.body === fields.servedBody && (
        <RewriteRow
          disabled={voice.control.pending || voice.control.disabled}
          onRewrite={voice.rewrite}
        />
      )}
    </>
  );
}

// Why the reader is writing, asked where the record does not answer it. A
// reply derives its category from the thread it answers.
function ComposeWhy({
  anchorActivity,
  flagged,
  context,
  onContextChange,
}: Readonly<{
  anchorActivity: Activity | undefined;
  flagged: ReadonlySet<MissingField>;
  context: CommunicationContext | "";
  onContextChange: (next: CommunicationContext | "") => void;
}>) {
  const t = useT();
  if (!asksWhy(anchorActivity)) {
    return <p>{t("compose.derivedReply")}</p>;
  }
  return (
    <Field
      label={t("compose.why")}
      hint={t("compose.whyHint")}
      error={flagged.has("context") ? t("compose.missingWhy") : undefined}
    >
      {(control) => (
        <Select
          {...control}
          options={contextOptions(t)}
          value={context}
          onChange={(value) =>
            onContextChange(value as CommunicationContext | "")
          }
        />
      )}
    </Field>
  );
}

// The foot: what the engine and the server said about sending, then what
// pressing the button will do.
function ComposeFoot({
  isChannelReply,
  fields,
  send,
  discardError,
  contactId,
  bodyKey,
}: Readonly<{
  isChannelReply: boolean;
  fields: ComposeFields;
  send: ComposeSend;
  discardError: string | null | undefined;
  contactId?: string;
  bodyKey: MessageKey;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const scheduled = momentOf(fields.sendAt);
  return (
    <>
      {!isChannelReply && (
        <>
          <MailSendNotices
            to={fields.to}
            cc={fields.cc}
            context={send.claimedContext}
          />
          {/* No override is offered: the engine grants nothing on what a
              sender types about their own send. */}
          <SendPermission
            preview={send.permission.preview}
            asking={send.permission.asking}
            unanswered={send.permission.unanswered}
          />
        </>
      )}
      {fields.sendUnavailable && <p>{t("compose.sendUnavailable")}</p>}
      {/* A failed rejection leaves the judgment open over the same words. */}
      {discardError && <ErrorLine>{discardError}</ErrorLine>}
      <SendRefusal
        refusal={send.refusal}
        contactId={contactId}
        review={send.sendReview}
      />
      <p>{t(bodyKey)}</p>
      {/* The chosen moment in words, under the control that carries it. */}
      {scheduled && (
        <p className="t-caption">
          {t("compose.willGoOut", {
            when: momentLabel(scheduled, locale, viewerZone()),
          })}
        </p>
      )}
    </>
  );
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
  recordAddress,
  kind,
  transports = NO_TRANSPORTS,
  initialTransportId,
  staleThread = false,
  intent: askedIntent,
  open,
  onClose,
  onSent,
}: Readonly<{
  // Absent means this is an ACCOUNT-STARTED send: a new conversation with no
  // prior message to anchor to (ADR-0087 §1). It is the same send either way —
  // only the origin differs — so the drafting, consent, refusal and provenance
  // behaviour below is shared rather than forked. A reply keeps threading and
  // inherits its links; an account-started message roots its own thread and
  // files itself under the record it was started from.
  initialMessage?: Readonly<{ subject: string; body: string }>;
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  contactId?: string;
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
  /** The conversation the caller named cannot be answered any more: a channel
   *  disconnected, an address removed, a message off the end of the window. The
   *  composer opens on the default AND says so, because falling back silently
   *  would have the reader writing into a conversation they did not choose. */
  staleThread?: boolean;
  /** What the caller wants the draft to be ABOUT. A moment action knows why it
   *  fired — "their reply is overdue", "the meeting needs an agenda" — and that
   *  reason shaped nothing until it reached the steer field. */
  intent?: string;
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
  const fields = useComposeFields({ initialMessage, askedIntent, contactId });
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
    where: { answering, entityType, entityId, isChannelReply },
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
        title={t(wording.title)}
        tier="confirm"
        // A reading drawer for mail keeps the record in view beside it.
        intent={dial.asDrawer ? "drawer-reading" : "form"}
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
            permission={send.permission}
            discardControl={voice.discardControl}
            savedDraft={savedDraft}
            busy={busy}
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
              flagged={send.flagged}
              voice={voice}
              voiceMaturity={voiceProfile.data?.maturity}
              onToEditing={settle}
            />
            <ComposeBody
              bodyId={bodyId}
              fields={fields}
              attached={attached}
              fileTarget={fileTarget}
              entityType={entityType}
              entityId={entityId}
              signs={open && !isChannelReply}
              channelLabel={dial.channel?.label}
              carriageBlocks={carriageBlocks}
              flagged={send.flagged}
              voice={voice}
            />
            <ComposeWhy
              anchorActivity={thread.anchorActivity}
              flagged={send.flagged}
              context={fields.context}
              onContextChange={fields.setContext}
            />
            <ComposeFoot
              isChannelReply={isChannelReply}
              fields={fields}
              send={send}
              discardError={voice.discardControl?.error}
              contactId={contactId}
              bodyKey={wording.body}
            />
          </div>
        </div>
      </ConfirmModal>
    </>
  );
}
