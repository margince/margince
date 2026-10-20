import { SendMark } from "../../components/sendpermission";
import { Button, Field } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { RichText } from "../../design-system/richtext";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useRichTextLabels } from "../../lib/richtextlabels";
import type { CarriageViolation } from "./carriage";
import type { Activity } from "./compose";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "./compose-context";
import type { ComposeThread } from "./composeanchor";
import {
  AttachAction,
  AttachedFiles,
  CarriageNotice,
} from "./composeattachments";
import { ConversationFold } from "./composeconversation";
import { RewriteRow } from "./composedraftband";
import type { ComposeFields } from "./composefields";
import { FieldNeed } from "./composehead";
import type { RelinkKind } from "./composerelink";
import type { useSavedDraft } from "./composesaveddraft";
import type { ComposeSend, MissingField } from "./composesend";
import { SignOffPreview } from "./composesignoff";
import type { useTargetFiles } from "./composetarget";
import { ConversationChoices, ThreadPane } from "./composethread";
import type { VoiceDraft } from "./composevoicedraft";

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

// The footer's lead: the permission mark a rep checks before pressing
// anything, then discarding the draft and saving it.
export function ComposeActionsLead({
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
export function ComposeConversation({
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

// The words the reader writes, with what travels with them: the sign-off,
// the attached files, and the rewrite offer over a model's untouched draft.
export function ComposeBody({
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
