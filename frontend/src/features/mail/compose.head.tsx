import type { components } from "../../api/schema";
import { Callout } from "../../design-system/callout";
import {
  type PickableProject,
  ProjectPicker,
} from "../../design-system/projectpicker";
import type { TokenSuggestion } from "../../design-system/tokeninput";
import { INTL_LOCALE } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useProjectRecord } from "../../screens/projectrecord";
import type { Transport } from "../contacts/contacttransports";
import type { VoiceProfile } from "./compose";
import { StaleThreadNotice } from "./compose.notices";
import { useThreadProject } from "./composeanchor";
import { DraftBand } from "./composedraftband";
import type { DraftProvenance, DraftUnavailable } from "./composedraftcall";
import {
  AccountDraftContext,
  DraftOffer,
  type PendingAction,
} from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { ComposeRecords } from "./composefiling";
import { AddressBlock, SubjectRow, TransportRow } from "./composehead";
import { SavedDraftNotices, type useSavedDraft } from "./composesaveddraft";
import type { MissingField } from "./composesend";
import type { useTransportDial } from "./composetarget";
import type { VoiceDraft } from "./composevoicedraft";

/**
 * The one control that says which project a message belongs to, on a reply
 * and on a message started from an account alike.
 *
 * It carries no explanation of its own. The tag it puts in the Subject field
 * shows there, and a rep who does not want it deletes it like any other text.
 * A sentence saying so would restate what the field already shows.
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

// The mail-only half of the composer. AI drafting has no endpoint for a channel,
// and a channel request has no room for recipient and subject inputs. A channel
// reply renders none of this, so it inherits none of its branching.
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
