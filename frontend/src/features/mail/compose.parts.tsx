import { Ellipsis, ShieldAlert, Trash2 } from "lucide-react";
import { type ReactNode, useState } from "react";
import { SendMark } from "../../components/sendpermission";
import { Button, Field } from "../../design-system/atoms";
import { DockedWindow, type DockState } from "../../design-system/dockedwindow";
import { EmailText } from "../../design-system/emailtext";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { RichText } from "../../design-system/richtext";
import { Select } from "../../design-system/select";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "../../lib/compose-context";
import { useRichTextLabels } from "../../lib/richtextlabels";
import type { CarriageViolation } from "./carriage";
import type { Activity, VoiceProfile } from "./compose";
import { AiDraftBand } from "./composeaiband";
import { CarriageNotice } from "./composeattachments";
import type { ComposeFields } from "./composefields";
import { FieldNeed } from "./composehead";
import type { useSavedDraft } from "./composesaveddraft";
import { momentLabel, momentOf, ScheduleMenu } from "./composeschedule";
import type { ComposeSend, MissingField } from "./composesend";
import { SignOffPreview } from "./composesignoff";
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

// The composer's window: docked beside the page, at its own size, grown or
// folded. Each opening starts at its own size.
export function ComposeFrame({
  title,
  onClose,
  closeDisabled,
  bodyId,
  openedFrom,
  children,
}: Readonly<{
  title: string;
  onClose: () => void;
  closeDisabled: boolean;
  openedFrom?: HTMLElement;
  /** The words' field, where focus lands as the window opens. */
  bodyId: string;
  children: ReactNode;
}>) {
  const [state, setState] = useState<DockState>("open");
  return (
    <DockedWindow
      title={title}
      state={state}
      onStateChange={setState}
      onClose={onClose}
      closeDisabled={closeDisabled}
      initialFocusTo={() => document.getElementById(bodyId)}
      openedFrom={openedFrom}
    >
      {children}
    </DockedWindow>
  );
}

// The footer: files and the draft's state on the left. The send and its other
// moments sit at the end, where the hand ends up.
export function ComposeFoot({
  confirmLabel,
  send,
  isChannelReply,
  held,
  onPickMoment,
  sendAt,
  savedDraft,
  busy,
  marksRef,
}: Readonly<{
  confirmLabel: string;
  send: ComposeSend;
  /** A channel reply goes now, and has no engine answer to mark. */
  isChannelReply: boolean;
  /** A write about these words is in flight, so the send waits for it. */
  held: boolean;
  onPickMoment: () => void;
  sendAt: string;
  savedDraft: ReturnType<typeof useSavedDraft>;
  /** A send or a rejection is in flight. */
  busy: boolean;
  /** The place the body's formatting marks draw, beside Send. */
  marksRef: (node: HTMLDivElement | null) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const scheduled = momentOf(sendAt);
  const error = send.sendError ?? savedDraft.error;
  return (
    <div className="compose-foot">
      {error && <ErrorLine>{error}</ErrorLine>}
      <div className="compose-foot-row">
        <span className="actions-split">
          <Button
            variant="primary"
            onClick={send.confirm}
            pending={send.send.isPending}
            disabled={held}
          >
            {confirmLabel}
          </Button>
          {!isChannelReply && <ScheduleMenu onOpen={onPickMoment} />}
        </span>
        {!isChannelReply && <SendMark {...send.permission} />}
        <div className="compose-foot-marks" ref={marksRef} />
        <span className="t-caption compose-foot-status" role="status">
          {scheduled
            ? t("compose.willGoOut", {
                when: momentLabel(scheduled, locale, viewerZone()),
              })
            : savedDraftStatus(savedDraft, t)}
        </span>
        {savedDraft.enabled && (
          <IconAction
            label={t("compose.discardMessage")}
            icon={<Trash2 aria-hidden="true" />}
            onClick={savedDraft.discard}
            disabled={busy}
            pending={savedDraft.removing}
          />
        )}
      </div>
    </div>
  );
}

// What became of the draft as the reader writes: being kept, or kept.
function savedDraftStatus(
  savedDraft: ReturnType<typeof useSavedDraft>,
  t: ReturnType<typeof useT>,
): string {
  if (savedDraft.saving) return t("compose.draftSaving");
  if (savedDraft.kept) return t("compose.savedDraftSaved");
  return "";
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

// The words the reader writes: the ask for a model's draft above them, the
// sign-off and the thread they answer under them.
export function ComposeBody({
  bodyId,
  fields,
  mail,
  replying,
  earlier,
  voiceMaturity,
  channelLabel,
  carriageBlocks,
  flagged,
  voice,
  marksIn,
  attach,
}: Readonly<{
  bodyId: string;
  fields: ComposeFields;
  /** Mail drafts and signs; a channel message does neither. */
  mail: boolean;
  replying: boolean;
  /** The thread's earlier messages, folded under the words of a reply. */
  earlier?: ReactNode;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
  channelLabel: string | undefined;
  carriageBlocks: readonly CarriageViolation[];
  flagged: ReadonlySet<MissingField>;
  voice: VoiceDraft;
  /** Where the editor's marks draw, in the foot; null until it mounts. */
  marksIn: Element | null;
  /** The paperclip, among the editor's own marks. */
  attach: ReactNode;
}>) {
  const t = useT();
  const richTextLabels = useRichTextLabels();
  const frozen = voice.rejectionInFlight;
  return (
    <>
      {mail && (
        <AiDraftBand
          replying={replying}
          fields={fields}
          voice={voice}
          voiceMaturity={voiceMaturity}
        />
      )}
      {/* Both renderings travel. A text client, a screen reader and a spam
          filter read the plain part. */}
      <RichText
        id={bodyId}
        value={fields.html}
        onChange={fields.editBody}
        label={t("compose.body")}
        placeholder={t("compose.bodyPlaceholder")}
        labels={richTextLabels}
        rows={8}
        grow
        disabled={frozen}
        actions={attach}
        footIn={marksIn}
      />
      <FieldNeed show={flagged.has("body")} need={t("compose.missingBody")} />
      {mail && <SignOffPreview body={fields.body} subject={fields.subject} />}
      {earlier}
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
  const missing = context === "";
  // One strip in the warning band: what is missing and why it matters, with
  // the pick that fixes it at the end. Once picked it stays, without the warning.
  return (
    <div
      className={
        missing ? "compose-strip compose-strip-warning" : "compose-strip"
      }
    >
      {missing && <ShieldAlert aria-hidden="true" />}
      <p className="compose-strip-text">
        {missing ? (
          <>
            <strong>{t("compose.whyMissing")}</strong>{" "}
            {t("compose.whyMissingText")}
          </>
        ) : (
          t("compose.why")
        )}
      </p>
      <Field
        className="compose-strip-field"
        label={t("compose.why")}
        labelHidden
        error={flagged.has("context") ? t("compose.missingWhy") : undefined}
      >
        {(control) => (
          <Select
            {...control}
            options={contextOptions(t).filter((o) => o.value !== "")}
            value={context}
            placeholder={t("compose.whyPick")}
            onChange={(value) =>
              onContextChange(value as CommunicationContext | "")
            }
          />
        )}
      </Field>
    </div>
  );
}

// The thread a reply answers, folded under its words as quoted history. One
// small control says how much is there, and opens it.
// It is the reader's to read; the reply carries only what they wrote.
export function EarlierMessages({
  messages,
}: Readonly<{ messages: readonly Activity[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const readable = messages.filter(
    (message) => message.content_state !== "withheld" && message.body,
  );
  if (readable.length === 0) {
    return null;
  }
  return (
    <div className="compose-earlier">
      <IconAction
        label={
          open
            ? t("compose.earlierHide")
            : plural("compose.earlierShow", readable.length, {
                count: formatNumber(readable.length, locale),
              })
        }
        icon={
          <span className="compose-earlier-pill">
            <Ellipsis aria-hidden="true" />
          </span>
        }
        variant="ghost"
        pressed={open}
        onClick={() => setOpen((was) => !was)}
      />
      {open &&
        readable.map((message) => (
          <figure key={message.id} className="compose-quoted">
            <figcaption className="t-caption">
              {t("compose.quotedHeader", {
                when: formatActivityWhen(
                  message.occurred_at,
                  message.occurred_precision,
                  locale,
                  viewerZone(),
                ),
                who:
                  message.direction === "outbound"
                    ? t("compose.quotedYou")
                    : message.email_summary?.counterparty?.trim() ||
                      t("email.received"),
              })}
            </figcaption>
            <blockquote>
              <EmailText body={message.body ?? ""} />
            </blockquote>
          </figure>
        ))}
    </div>
  );
}
