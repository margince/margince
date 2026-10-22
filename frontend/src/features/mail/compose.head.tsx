import { Reply, SquarePen } from "lucide-react";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ProjectPicker } from "../../design-system/projectpicker";
import type { TokenSuggestion } from "../../design-system/tokeninput";
import { INTL_LOCALE } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { Transport } from "../../lib/contacttransports";
import { useProjectRecord } from "../projects/projectrecord";
import { StaleThreadNotice } from "./compose.notices";
import { useThreadProject } from "./composeanchor";
import { AccountDraftContext } from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { ComposeRecords } from "./composefiling";
import { AddressBlock, SubjectRow, TransportRow } from "./composehead";
import { SavedDraftNotices, type useSavedDraft } from "./composesaveddraft";
import type { MissingField } from "./composesend";
import type { useTransportDial } from "./composetarget";

/**
 * The one control that says which project a message belongs to, in the
 * composer's head. A reply and a message started from an account share it.
 *
 * It carries no explanation of its own. The tag it puts in the Subject field
 * shows there, and a rep who does not want it deletes it like any other text.
 * Nothing renders when the record reaches no live project, because a list whose
 * only entry is None asks a question with one answer.
 */
export function ProjectFiling({
  filed,
  records,
}: Readonly<{
  /** Whether the message files under a project here; a channel reply does not. */
  filed: boolean;
  records: ComposeRecords;
}>) {
  if (!filed) {
    return null;
  }
  return (
    <ProjectPicker
      projects={records.reachableProjects}
      projectId={records.projectFiling.projectId}
      onChange={records.projectFiling.setProjectId}
      inHead
    />
  );
}

/**
 * Where a CHANNEL reply will be filed, said rather than chosen.
 *
 * A channel send carries the words and the consent purpose and nothing else,
 * since SendMessageRequest has no links and no subject. So the server files the
 * reply under the links of the conversation it answers. A picker here would
 * collect an answer nothing could carry. Mail keeps its choice in the subject
 * tag, and a channel has no subject line to keep it in.
 *
 * It reads the anchor's own project link rather than the picker's list, since
 * that link is what the send inherits. A channel conversation hangs off a
 * contact, whose timeline reaches no project list, and it is filed all the same.
 *
 * Nothing renders while a read is unanswered or when the conversation names no
 * project. An unsettled read is not "no project", and naming a project the send
 * then contradicts is worse than the silence.
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
        // The picker labels an option with this same shape. So the project a
        // rep reads here and the one on a mail reply are one name.
        project: project.key
          ? `${project.key} · ${project.name}`
          : project.name,
      })}
    </p>
  );
}

// The mail-only half of the head: who the message is to and what it is about.
// A channel request has no room for recipient and subject inputs, so a channel
// reply renders none of this and inherits none of its branching.
function MailOnlyFields({
  to,
  onToChange,
  cc,
  onCcChange,
  bcc,
  onBccChange,
  copiesOpen,
  onOpenCopies,
  recipients,
  onToEditing,
  subject,
  onSubjectChange,
  rejectionInFlight,
  flagged,
  deadRecipients,
}: Readonly<{
  to: string[];
  onToChange: (next: string[]) => void;
  cc: string[];
  onCcChange: (next: string[]) => void;
  bcc: string[];
  onBccChange: (next: string[]) => void;
  /** Whether the copy rows are drawn. A link until they are asked for. */
  copiesOpen: boolean;
  onOpenCopies: () => void;
  /** The addresses this record already knows, offered as the reader types. */
  recipients: readonly TokenSuggestion[];
  /** The reader has started typing a recipient, before any is committed. */
  onToEditing: () => void;
  subject: string;
  onSubjectChange: (next: string) => void;
  /** The fields a pressed Send is still waiting for. Empty until it is pressed. */
  flagged: ReadonlySet<MissingField>;
  rejectionInFlight: boolean;
  /** The recipients on this draft that are known not to arrive. */
  deadRecipients: readonly string[];
}>) {
  const t = useT();
  return (
    <>
      <AddressBlock
        to={to}
        onToChange={onToChange}
        cc={cc}
        onCcChange={onCcChange}
        bcc={bcc}
        onBccChange={onBccChange}
        copiesOpen={copiesOpen}
        onOpenCopies={onOpenCopies}
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

/**
 * What this message answers, above the fields, with the way to start fresh
 * instead. On a new message it says how to continue a conversation, where the
 * rail offers one.
 */
function AnsweringLine({
  replying,
  answeringLine,
  railShown,
  onStartNew,
  disabled,
}: Readonly<{
  replying: boolean;
  answeringLine: string;
  railShown: boolean;
  onStartNew: () => void;
  disabled: boolean;
}>) {
  const t = useT();
  if (!replying) {
    return (
      <p className="t-caption compose-answering">
        <SquarePen aria-hidden="true" />
        {railShown ? t("compose.startsNew") : answeringLine}
      </p>
    );
  }
  return (
    <p className="t-caption compose-answering">
      <Reply aria-hidden="true" />
      <span>{answeringLine}</span>
      <Button variant="link" onClick={onStartNew} disabled={disabled}>
        {t("compose.startNewInstead")}
      </Button>
    </p>
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
export function ComposeHead({
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
  railShown,
  onStartNew,
  flagged,
  rejectionInFlight,
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
  /** Whether the conversations stand beside the form to continue one. */
  railShown: boolean;
  onStartNew: () => void;
  flagged: ReadonlySet<MissingField>;
  rejectionInFlight: boolean;
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
      {/* Mail chooses where it files in the head. A channel send carries no
          filing field, so it is told what the conversation inherits. */}
      {dial.isChannelReply && <ChannelReplyFiling activityId={activityId} />}
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
      {!dial.isChannelReply && (
        <>
          <AnsweringLine
            // The rail answers what is being replied to; the sentence says it
            // where the reader writes, with the way out of it.
            replying={replying}
            answeringLine={answeringLine}
            railShown={railShown}
            onStartNew={onStartNew}
            disabled={rejectionInFlight}
          />
          <MailOnlyFields
            flagged={flagged}
            to={fields.to}
            onToChange={fields.setTo}
            cc={fields.cc}
            onCcChange={fields.setCc}
            bcc={fields.bcc}
            onBccChange={fields.setBcc}
            copiesOpen={fields.copiesOpen}
            onOpenCopies={fields.openCopies}
            recipients={records.recipients}
            onToEditing={onToEditing}
            subject={fields.subject}
            onSubjectChange={fields.setSubject}
            rejectionInFlight={rejectionInFlight}
            deadRecipients={records.deadRecipients}
          />
        </>
      )}
    </>
  );
}
