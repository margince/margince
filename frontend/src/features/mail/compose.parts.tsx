import { type ReactNode, useState } from "react";
import { Button, Field } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { RichText } from "../../design-system/richtext";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "../../lib/compose-context";
import { useRichTextLabels } from "../../lib/richtextlabels";
import type { CarriageViolation } from "./carriage";
import type { Activity, VoiceProfile } from "./compose";
import { AiDraftBand, AiDraftToggle } from "./composeaiband";
import type { ComposeThread } from "./composeanchor";
import {
  AttachAction,
  AttachedFiles,
  CarriageNotice,
} from "./composeattachments";
import { ConversationFold } from "./composeconversation";
import type { ComposeFields } from "./composefields";
import { FieldNeed } from "./composehead";
import { ConversationRail } from "./composerail";
import type { RelinkKind } from "./composerelink";
import type { useSavedDraft } from "./composesaveddraft";
import type { ComposeSend, MissingField } from "./composesend";
import { SignOffPreview } from "./composesignoff";
import type { useTargetFiles } from "./composetarget";
import type { VoiceDraft } from "./composevoicedraft";
import { SendRefusal } from "./sendrefusal";

// sharedUnsubscribeAhead predicts the sharedUnsubscribe refusal (refusalOf).
// A marketing send's unsubscribe link is one addressee's consent record, so a
// second addressee is refused. The server rule stays the authority, keyed on
// the marketing category, which a reply never carries.
export function sharedUnsubscribeAhead(
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

// The mail-only send-time notice: a shared-unsubscribe-token risk. A channel
// reply has no addressee list to warn about, so this renders nothing there.
// The empty-recipient line belongs to the To field, not the foot of the drawer,
// because that is closer to the thing to fix.
export function MailSendNotices({
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

// The footer's lead: saving the draft, then the checks a send meets.
export function ComposeActionsLead({
  isChannelReply,
  savedDraft,
  checks,
  busy,
}: Readonly<{
  /** A channel reply meets no checks a list could hold. */
  isChannelReply: boolean;
  savedDraft: ReturnType<typeof useSavedDraft>;
  checks: ReactNode;
  /** A send or a rejection is in flight. */
  busy: boolean;
}>) {
  const t = useT();
  return (
    <>
      {savedDraft.enabled && (
        <Button
          onClick={savedDraft.save}
          pending={savedDraft.saving}
          disabled={busy}
        >
          {t("compose.saveDraft")}
        </Button>
      )}
      {!isChannelReply && checks}
    </>
  );
}

// What the send came back with, under the words it is about. That is a send
// the server cannot make yet, or a refusal with what to do about it. A channel
// reply states its one promise here too, having no checks to state it in.
export function ComposeOutcome({
  isChannelReply,
  bodyKey,
  sendUnavailable,
  send,
  contactId,
}: Readonly<{
  isChannelReply: boolean;
  bodyKey: MessageKey;
  sendUnavailable: boolean;
  send: ComposeSend;
  contactId: string | undefined;
}>) {
  const t = useT();
  return (
    <>
      {isChannelReply && <p>{t(bodyKey)}</p>}
      {sendUnavailable && <p>{t("compose.sendUnavailable")}</p>}
      <SendRefusal
        refusal={send.refusal}
        contactId={contactId}
        review={send.sendReview}
      />
    </>
  );
}

// The column beside the form: the record's conversations, the one being
// answered marked and open to read.
export function ComposeConversation({
  thread,
  answering,
  recordName,
  viewerId,
  nameOf,
  onSelect,
  disabled,
}: Readonly<{
  thread: ComposeThread;
  answering: string | undefined;
  recordName: string | undefined;
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
    <ConversationFold choosing={!answering}>
      <ConversationRail
        recordName={recordName}
        conversations={recent.conversations}
        pending={recent.pending}
        failed={recent.failed}
        onRetry={recent.retry}
        answering={answering}
        anchor={thread.anchorActivity}
        anchorPending={thread.anchorUnresolved}
        anchorFailed={anchorRead.failed}
        onRetryAnchor={anchorRead.retry}
        thread={conversation}
        viewerUserId={viewerId}
        nameOf={nameOf}
        onChoose={onSelect}
        onLeave={() => onSelect(null)}
        disabled={disabled}
      />
    </ConversationFold>
  );
}

// The words the reader writes, with what travels with them: the model's
// draft above them, the sign-off under them, and the attached files.
export function ComposeBody({
  bodyId,
  fields,
  attached,
  fileTarget,
  entityType,
  entityId,
  mail,
  replying,
  aiSource,
  voiceMaturity,
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
  /** Mail drafts and signs; a channel message does neither. */
  mail: boolean;
  replying: boolean;
  /** What a model's draft is written from, said under the ask. */
  aiSource: string;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
  channelLabel: string | undefined;
  carriageBlocks: readonly CarriageViolation[];
  flagged: ReadonlySet<MissingField>;
  voice: VoiceDraft;
}>) {
  const t = useT();
  const richTextLabels = useRichTextLabels();
  const frozen = voice.rejectionInFlight;
  // A caller that knows what the draft is about opens the ask with it filled.
  const [asking, setAsking] = useState(() => fields.intent !== "");
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
        bar="head"
        lead={
          mail && (
            <AiDraftToggle
              open={asking}
              onToggle={() => setAsking((was) => !was)}
            />
          )
        }
        band={
          mail && (
            <AiDraftBand
              open={asking}
              replying={replying}
              source={aiSource}
              fields={fields}
              voice={voice}
              voiceMaturity={voiceMaturity}
            />
          )
        }
        foot={
          mail && <SignOffPreview body={fields.body} subject={fields.subject} />
        }
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
      {/* Attachments sit under the words they belong to, as in a mail client. */}
      <AttachedFiles
        chosen={attached.files}
        onChange={attached.setFiles}
        disabled={frozen}
      />
      <CarriageNotice channel={channelLabel} blocks={carriageBlocks} />
    </>
  );
}

// Why the reader is writing, asked where the record does not answer it. A
// reply derives its category from the thread it answers, which the checks say.
export function ComposeWhy({
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
    return null;
  }
  return (
    <Field
      className="compose-address-field"
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
