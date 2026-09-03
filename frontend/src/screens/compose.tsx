import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, ChevronUp, Clock, Sparkles, X } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { navigate } from "../app/router";
import {
  Badge,
  Button,
  Checkbox,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Calendar, type ISODay, isoDay } from "../design-system/calendar";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Eyebrow } from "../design-system/eyebrow";
import { Popover } from "../design-system/popover";
import {
  liveProjects,
  type PickableProject,
  ProjectPicker,
  type ProjectScope,
  useSoleProjectDefault,
} from "../design-system/projectpicker";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select, type SelectOption } from "../design-system/select";
import { useToast } from "../design-system/toast";
import {
  formatDateAbbrev,
  formatDateTime,
  INTL_LOCALE,
  identifierNumber,
} from "../format/format";
import { viewerZone, zoneNameAndOffset } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { entityTimelineKeys } from "./activitykeys";
import {
  isConsentNotGranted,
  ProblemError,
  problemFieldErrorsOf,
  problemMessageOf,
  throwProblem,
  useViewerId,
} from "./common";
import { recordNamesIn, useOrganization360 } from "./company360";
import {
  ConversationChoices,
  ThreadPane,
  useRecentConversations,
  useThreadMessages,
} from "./composethread";
import { useConsentPurposes } from "./consent";
import { useRoster } from "./entityref";
import { usePerson360 } from "./person360";
import {
  stripEveryKeyTag,
  stripSubjectTag,
  subjectTag,
  useProjectRecord,
  withSubjectTag,
} from "./projectrecord";
import { SCHEDULED_SCREEN } from "./scheduledsends";
import { useVoiceProfile } from "./voice-profile";
import "./compose.css";

// The composer surface for the three already-routed ops (draftEmail /
// sendEmail / relinkActivity): a human's edit-then-confirm reply, and a
// mis-captured activity's relink. Pure frontend — every op is live, audited,
// and typed on the backend; this file only calls them.

type Activity = components["schemas"]["Activity"];
type ConsentPurpose = components["schemas"]["ConsentPurpose"];
type EmailDraft = components["schemas"]["EmailDraft"];
type VoiceProfile = components["schemas"]["VoiceProfile"];

// What a drafting call reported about the text it produced. Held apart from the
// fields it filled because the disclosure is owed for the call that put model
// output on this surface, whatever the human then does to the words.
type DraftProvenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version"
>;

// The link targets a relink can point at (relinkActivity's entity_type enum,
// minus `activity` — a relink never points at another activity). Reused by
// ComposeModal and TimelineActions so the whole surface speaks one vocabulary.
export type RelinkKind =
  | "person"
  | "organization"
  | "deal"
  | "lead"
  | "project";

// The relink target is chosen via cross-object search (/search covers every
// kind; the per-entity list endpoints don't all expose `q`). Each candidate's
// entity_type comes from its SearchResult.type, remembered here so the confirm
// can recover it — RecordPickerCandidate itself only carries {id,name}.
// Activity results are dropped: relink's target enum has no `activity`.
function useSearchTargets() {
  const kindById = useRef(new Map<string, RelinkKind>());
  const search = useCallback(
    async (q: string): Promise<RecordPickerCandidate[]> => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q, limit: 10 } },
      });
      if (error) throwProblem(error);
      const out: RecordPickerCandidate[] = [];
      for (const result of data.data) {
        // Neither is a record this can be filed against: an activity is the
        // message itself, and a tag is a word rather than something a message
        // can be about.
        if (result.type === "activity" || result.type === "tag") continue;
        kindById.current.set(result.id, result.type);
        out.push({ id: result.id, name: result.title ?? result.id });
      }
      return out;
    },
    [],
  );
  return { search, kindOf: (id: string) => kindById.current.get(id) ?? null };
}

// A 🟢 internal association (no autonomy dot): move or also-link a captured
// activity's typed link to the right person/org/deal/lead. Idempotent on the
// backend — re-relinking the same target is a no-op that still answers 200.
// `threadKey` is the activity's conversation key when it has one. With it the
// dialog offers to move the whole thread through `relinkThread`, which applies
// this same association to every message of the conversation the rep may
// edit, in one transaction — a mis-filed conversation is usually mis-filed
// whole.
// What one confirmed relink asks for, read at the moment the reader confirmed
// it rather than at whichever render the mutation's options were last armed on.
type RelinkRequest = Readonly<{
  activityId: string;
  threadKey?: string | null;
  target: RecordPickerCandidate;
  version?: number | null;
  thread: boolean;
  replace: boolean;
}>;

export function RelinkModal({
  activityId,
  activityVersion,
  threadKey,
  entityType,
  entityId,
  open,
  onClose,
}: Readonly<{
  activityId: string;
  // The version the reader's copy of the activity was read at, sent as
  // If-Match so a relink cannot overwrite a change nobody saw. Absent only
  // where the caller genuinely has none; the thread door takes no version at
  // all, since one cannot condition a move across many activities.
  activityVersion?: number | null;
  threadKey?: string | null;
  entityType: RelinkKind;
  entityId: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const { search, kindOf } = useSearchTargets();
  const [target, setTarget] = useState<RecordPickerCandidate | null>(null);
  const [replace, setReplace] = useState(false);
  const [wholeThread, setWholeThread] = useState(false);

  // What the confirm decided arrives as the mutation's VARIABLE — ALL of it,
  // including which activity and which conversation, because a mixture is worse
  // than either: a fresh `thread` read beside a stale `threadKey` falls through
  // the thread door into the single relink, where the version guard has already
  // been skipped on the strength of `thread` being true.
  //
  // Read through this closure each would be the value from the
  // render before the confirm was enabled, because react-query re-arms a
  // mutation's options in a passive effect: a version that landed just before
  // the click would refuse a relink that is perfectly valid, and a toggle
  // flipped at the same moment would move the wrong set. The remaining guard is
  // a real path and stays — `kindOf` answers from the search results, and a
  // target whose remembered kind was lost must be surfaced rather than relinked
  // to nothing.
  const mutation = useMutation({
    mutationFn: async ({
      activityId,
      threadKey,
      target,
      version,
      thread,
      replace,
    }: RelinkRequest) => {
      const kind = kindOf(target.id);
      if (!kind) {
        throwProblem({ title: t("compose.relinkTarget") });
      }
      // A relink of ONE activity conditions on the version this reader saw, and
      // a copy that arrived without one cannot make that claim. Refused by name
      // rather than through requireVersion's bare throw: the reader sees the
      // mutation's error, and "something went wrong" is not a thing anyone can
      // act on when the answer is "reopen it and try again".
      if (!thread && version == null) {
        throwProblem({ title: t("compose.relinkNoVersion") });
      }
      if (threadKey && thread) {
        const { data, error } = await api.POST("/activities/relink-thread", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: {
            thread_key: threadKey,
            entity_type: kind,
            entity_id: target.id,
            replace_existing_of_type: replace,
          },
        });
        if (error) throwProblem(error);
        return data;
      }
      const { data, error } = await api.POST("/activities/{id}/relink", {
        params: {
          // The version the reader's copy was read at, so a relink cannot
          // overwrite a change nobody saw. requireVersion refuses the write
          // rather than sending it unpinned: unpinned is last-write-wins, and
          // the mutation's own error path is what tells the reader it did not
          // go through.
          //
          // The idempotency key travels INSIDE the precondition rather than in
          // a `header:` of its own: they are one slot, and written twice the
          // later wins.
          ...ifMatch(requireVersion(version ?? undefined), {
            "Idempotency-Key": crypto.randomUUID(),
          }),
          path: { id: activityId },
        },
        body: {
          entity_type: kind,
          entity_id: target.id,
          replace_existing_of_type: replace,
        },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: () => {
      for (const queryKey of entityTimelineKeys(entityType, entityId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A relink is exactly the write that changes where a reply files, and the
      // composer's filing line reads the activity to say so. Without this the
      // line keeps naming the project the activity was moved AWAY from, which
      // is a wrong answer to the one question it exists to answer.
      queryClient.invalidateQueries({ queryKey: ["activity", activityId] });
      onClose();
    },
  });

  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("compose.relinkTitle")}
      confirmLabel={t("compose.relinkConfirm")}
      confirmDisabled={!target}
      onConfirm={() =>
        target &&
        mutation.mutate({
          activityId,
          threadKey,
          target,
          version: activityVersion,
          thread: wholeThread,
          replace,
        })
      }
      pending={mutation.isPending}
      error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
    >
      <div className="compose-fields">
        <RecordPicker
          label={t("compose.relinkTarget")}
          searchTargets={search}
          onPick={setTarget}
          selected={target}
        />
        <Checkbox
          className="t-body"
          label={t("compose.relinkReplace")}
          checked={replace}
          onChange={(event) => setReplace(event.target.checked)}
        />
        <p className="t-caption">{t("compose.relinkReplaceHint")}</p>
        {threadKey && (
          <>
            <Checkbox
              className="t-body"
              label={t("compose.relinkThread")}
              checked={wholeThread}
              onChange={(event) => setWholeThread(event.target.checked)}
            />
            <p className="t-caption">{t("compose.relinkThreadHint")}</p>
          </>
        )}
      </div>
    </ConfirmModal>
  );
}

// Fill the form from a served draft, without ever clobbering a field the rep
// already edited.
//
// The reference, the disclosure and the reasons all describe the WORDS they
// were served with, so all three ride on exactly the condition that applies
// the body. A re-draft over text the rep already wrote keeps that text —
// adopting the newer reference would report a stranger's draft as the rep's
// own edit of it, the newer disclosure would credit a model with words a
// human typed, and the newer reasons would explain a draft nobody is looking
// at.
function fillFromDraft(
  result: Extract<DraftResult, { available: true }>,
  form: Readonly<{
    subject: string;
    body: string;
    toEmpty: boolean;
    setSubject: (next: string) => void;
    setBody: (next: string) => void;
    // The words as the model served them, kept beside the words on screen so
    // the composer can tell the two apart. It is what decides whether a
    // REWRITE is offered: rewriting is an instruction to the machine about its
    // own draft, and once a rep has edited the text there is no longer a
    // machine draft to rewrite — only the rep's work to overwrite.
    setServedBody: (next: string) => void;
    // A rewrite replaces the body it was asked about. Every other draft fills
    // an EMPTY field and never clobbers, because the rep may have written
    // something; a rewrite is only ever offered over the model's own untouched
    // words, and replacing them is the whole ask.
    rewrite?: boolean;
    setTo: (next: string[]) => void;
    setDraftRef: (next: string | null) => void;
    setProvenance: (next: DraftProvenance) => void;
    setReasoning: (next: components["schemas"]["AccountDraftReason"][]) => void;
    setScope: (next: ProjectScope | undefined) => void;
  }>,
) {
  const drafted = result.draft;
  // A subject holding only the project tag is a subject the rep has not written
  // yet: the composer put it there, not them. The drafted subject fills in
  // behind the tag rather than being dropped as "the field is taken".
  const written = stripEveryKeyTag(form.subject).trim();
  if (!written) {
    form.setSubject(
      form.subject.trim()
        ? withSubjectTag(drafted.subject, form.subject.trim())
        : drafted.subject,
    );
  }
  if (!form.body || form.rewrite) {
    form.setBody(drafted.body);
    form.setServedBody(drafted.body);
    form.setDraftRef(drafted.draft_ref ?? null);
    form.setProvenance({
      // Absent reads as false, which the contract says outright: a missing
      // flag may not silently become a missing disclosure, but it also may
      // not claim a model wrote text no model touched.
      ai_generated: drafted.ai_generated ?? false,
      ai_disclosure: drafted.ai_disclosure,
      voice_profile_version: drafted.voice_profile_version,
    });
    form.setReasoning(result.reasoning ?? []);
    form.setScope(result.scope);
  }
  if (form.toEmpty && drafted.to?.length) {
    form.setTo(drafted.to);
  }
}

// The reply-side draft: it answers the message it is anchored to, so it needs
// nothing but that activity and the caller's optional steering.
async function draftFromActivity({
  activityId,
  intent,
  t,
}: Readonly<{
  activityId: string;
  intent: string;
  t: ReturnType<typeof useT>;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST(
    "/activities/{id}/draft-email",
    {
      params: { path: { id: activityId } },
      body: intent.trim() ? { intent: intent.trim() } : {},
    },
  );
  if (response.status === 501) return { available: false as const };
  // Success is the real 2xx WITH a draft body, never merely the absence of an
  // error: openapi-fetch reports a falsy `error` (and undefined `data`) for a
  // bodiless non-2xx (a gateway 502/503/504), which would otherwise fall
  // through as a fabricated draft and crash the fill on undefined fields.
  if (!response.ok || !data) {
    throwProblem(error || { title: t("compose.actionFailed") });
  }
  return { available: true as const, draft: data };
}

/**
 * What survives clearing a subject: the project tag, and nothing else.
 *
 * A tag is not the rep's words and not the draft's — the composer put it there
 * to route the reply. Wiping it with the rejected draft would leave the filing
 * checkbox asserting a routing the subject no longer performs.
 */
function keptTag(subject: string): string {
  const words = stripEveryKeyTag(subject);
  return subject.slice(0, subject.length - words.length).trim();
}

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
 * The message this composer should answer, when the caller did not name one.
 *
 * Opened from a record page there is no anchor: the dialog asked the reader to
 * authorise a draft with an empty To, an empty Subject, and nothing on screen
 * saying what was being replied to. The reader could press "Draft with AI"
 * without knowing who they were writing to or which message they were
 * answering — reported from the running product in exactly those words.
 *
 * The latest message on the record is the answer, and the PROJECT narrows it:
 * a reader who has picked a project is working inside that project, so the
 * conversation to continue is that project's own last message rather than the
 * account's. Changing the selection changes the answer, which is why this is a
 * query keyed on both rather than a value read once when the dialog opened.
 *
 * `kind: email` because this composer sends mail: the last CALL on an account
 * is not a message anyone can reply to, and offering it as one would put a
 * recipient in the To field that the call never had.
 */
function useLatestMessage(
  entityType: RelinkKind,
  entityId: string,
  projectId: string,
  enabled: boolean,
): { activity?: Activity; settled: boolean } {
  // A project narrows through the list's OWN project_id filter, not through
  // entity_type — a project is not one of the entity kinds that filter takes,
  // and asking for one returns an empty page rather than an error, which reads
  // as "no earlier message" for every project on the installation.
  const query = useQuery({
    queryKey: ["compose-latest-message", entityType, entityId, projectId],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: {
          query: {
            entity_type: entityType,
            entity_id: entityId,
            ...(projectId ? { project_id: projectId } : {}),
            kind: "email",
            limit: 1,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled,
  });
  // A row whose content is WITHHELD is not an anchor. The list is
  // discover-gated, so a limited-audience message on this account comes back
  // with its subject and body nulled — and the draft endpoint, which is
  // content-gated, answers 404 for it. Anchoring there strands the composer:
  // the account path is hidden because an anchor exists, the draft cannot run,
  // and the reader is told they are replying to a message the server will not
  // let them answer. Falling back to the account path is the honest outcome.
  const newest = query.data?.data?.[0];
  return {
    // The list is ordered newest first (occurred_at DESC), so one row is the
    // latest — asked for as one row rather than fetched and sorted here.
    activity: newest?.content_state === "withheld" ? undefined : newest,
    // A read that has not answered, or failed, is not "no message": the
    // composer says nothing rather than claiming this is a fresh thread.
    settled: query.isSuccess,
  };
}

/**
 * Who a reply to this anchor goes to, resolved without drafting.
 *
 * Sending REQUIRES `to`, so an empty field is not a convenience gap: it is a
 * reply the reader must address by hand against a record that already holds
 * the answer. The server ranks the message's participants — its sender before
 * anyone copied on it — and this asks that same resolution the draft uses, so
 * what the composer shows on open and what a draft fills in cannot disagree.
 *
 * Undefined while unsettled and for a contact with no address on record. The
 * caller only ever fills an EMPTY field from it, so a reader who typed their
 * own recipient keeps it.
 */
function useReplyRecipient(anchor: string | undefined): string | undefined {
  const query = useQuery({
    queryKey: ["compose-reply-recipient", anchor],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/activities/{id}/reply-recipient",
        { params: { path: { id: anchor ?? "" } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: anchor !== undefined,
  });
  return query.data?.address === "" ? undefined : query.data?.address;
}

/**
 * What the composer says it is answering, or null while it does not yet know.
 *
 * Only for the composer with no conversation pane beside it. Where the pane
 * renders, the messages themselves say what is being answered and this sentence
 * repeats them. It survives for the case that has nothing to show: a message
 * opened from a record, where what is being continued was decided here.
 */
function answeringSentence(
  latest: { activity?: Activity; settled: boolean },
  callerAnchor: string | undefined,
  recipients: readonly string[],
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string | null {
  if (callerAnchor !== undefined || !latest.settled) {
    return null;
  }
  const answering = latest.activity;
  if (!answering) {
    return t("compose.answeringNothing");
  }
  const when = formatDateTime(answering.occurred_at, locale, zone);
  const subject = answering.subject?.trim();
  // WHO, as soon as it is known. The composer used to ask the reader to pick
  // the person, and picking them is what made the consent purpose beneath an
  // attestation about a named human. Now the thread supplies the address, so
  // the line has to name it — a purpose chosen against a recipient the reader
  // never saw is a weaker attestation than the one this replaced.
  //
  // The activity list does not carry the recipient, so this fills in when the
  // draft does. Before that the line still says what is being answered, which
  // is the question the reader had first.
  const who = recipients.join(", ");
  if (who && subject) {
    return t("compose.answeringTo", { who, subject, when });
  }
  return subject
    ? t("compose.answering", { subject, when })
    : t("compose.answeringNoSubject", { when });
}

/**
 * The project link a reply's own thread already carries, if any.
 *
 * Read from the anchor activity rather than assumed, because the thread is the
 * first rung of the same ladder the capture side walks: a sibling message that
 * was filed — by capture or by a human relink — is the settled answer, and it
 * outranks anything the deal says.
 */
function useThreadProject(activityId?: string): {
  activity?: Activity;
  projectId?: string;
  settled: boolean;
  failed: boolean;
} {
  const query = useQuery({
    queryKey: ["activity", activityId, "filing"],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}", {
        params: { path: { id: activityId ?? "" } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: Boolean(activityId),
  });
  return {
    // The anchor itself, not only its filing: the conversation pane needs the
    // `thread_key` this same read already carries, and a second GET of one
    // activity to reach one field is two answers to "what is being answered".
    activity: query.data,
    // Its own flag rather than folded into `settled`, because the two callers
    // want different halves: the filing says nothing on a failure, while the
    // pane must stop holding its place — a read that failed is not one still
    // arriving, and a placeholder kept on it is a wait that never ends.
    failed: query.isError,
    projectId: (query.data?.links ?? []).find(
      (link) => link.entity_type === "project",
    )?.entity_id,
    // A read that has not answered — or FAILED — is not "no project". Treating
    // an error as absence falls through to the deal's project, which the send
    // would then contradict: the server re-reads the anchor and inherits the
    // thread's own filing. Unsettled means say nothing.
    settled: !activityId || (!query.isPending && !query.isError),
  };
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
 * person, whose timeline reaches no project list at all, and it is filed all
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
    <p className="t-caption">
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

// The account-started draft (ADR-0087/A132). It grounds itself in the account
// rather than in a message, so it needs the recipient named before it can say
// anything — that is the one thing this path knows that an empty compose box
// does not.
//
// It answers the same `{available, draft}` shape the reply path does, so the
// fill below cannot tell them apart and the two origins cannot drift into
// different clobber rules.
async function draftFromAccount({
  entityType,
  entityId,
  recipientId,
  dealId,
  projectId,
  intent,
  t,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  recipientId: string;
  dealId: string;
  projectId: string;
  intent: string;
  t: ReturnType<typeof useT>;
}>): Promise<DraftResult> {
  // Only a company page can ground one: a person or a deal has no 360 to
  // write from, and grounding a message to a contact in some nearby account
  // would be a conversation the rep never chose.
  if (entityType !== "organization" || !recipientId) {
    return { available: false as const };
  }
  const { data, error, response } = await api.POST(
    "/organizations/{id}/draft-email",
    {
      params: { path: { id: entityId } },
      body: {
        person_id: recipientId,
        ...(dealId ? { deal_id: dealId } : {}),
        // The project the rep attributed the message to. The server grounds
        // the draft in the 360 SCOPED to it, so the other projects'
        // correspondence never reaches the model.
        ...(projectId ? { project_id: projectId } : {}),
        ...(intent.trim() ? { intent: intent.trim() } : {}),
      },
    },
  );
  if (response.status === 501) return { available: false as const };
  if (!response.ok || !data) {
    throwProblem(error || { title: t("compose.actionFailed") });
  }
  return {
    available: true as const,
    draft: data,
    reasoning: data.reasoning,
    scope: data.scope,
  };
}

// What either drafting path answers.
//
// The two wire shapes are not the same — only the account draft carries
// `reasoning` and `generated_by` — so this names the fields the fill actually
// reads and nothing else. Intersecting the two contract types instead would
// make every optional field of one optional on both, and the fill would stop
// noticing when a required field went missing.
type DraftResult =
  | { available: false }
  | {
      available: true;
      draft: Pick<EmailDraft, "subject" | "body" | "to"> &
        Partial<
          Pick<
            EmailDraft,
            | "draft_ref"
            | "ai_generated"
            | "ai_disclosure"
            | "voice_profile_version"
          >
        >;
      // Present only on the account-started path; a reply explains itself by
      // the message it is answering.
      reasoning?: components["schemas"]["AccountDraftReason"][];
      // Likewise account-only: what the scoped read kept, when a project was
      // chosen.
      scope?: ProjectScope;
    };

// The account-started path's own state: who the draft is grounded on, which
// open deal and which project it is about, and what the server said it wrote
// from.
//
// Held together because the three move together — a new recipient invalidates
// the reasons as surely as an emptied body does — and held OUT of ComposeModal
// because that component already carries the send, the consent gate, the
// refusal vocabulary and the voice-rejection flow.
function useAccountGrounding(
  personId: string | undefined,
  onGroundingChanged: () => void,
) {
  const [recipientId, setRecipientId] = useState(personId ?? "");
  const [dealId, setDealId] = useState("");
  // One choice, two effects: the project scopes the draft's grounding AND
  // files the sent message under the project (composedLinks).
  const [projectId, setProjectId] = useState("");
  const [reasoning, setReasoning] = useState<
    components["schemas"]["AccountDraftReason"][]
  >([]);
  // The server's scope report for the draft on screen; retired with the
  // reasons, because it describes the same read.
  const [scope, setScope] = useState<ProjectScope | undefined>(undefined);
  // Changing who the draft is to, or which deal it is about, retires the draft
  // that was written for the previous pair. Leaving it would show a message
  // addressed to B carrying A's words, A's disclosure and A's reasons — and
  // re-drafting could not repair it, because the fill never clobbers a
  // non-empty field.
  const reground = (apply: (next: string) => void) => (next: string) => {
    apply(next);
    setReasoning([]);
    setScope(undefined);
    onGroundingChanged();
  };
  return {
    recipientId,
    setRecipientId: reground(setRecipientId),
    dealId,
    setDealId: reground(setDealId),
    projectId,
    setProjectId: reground(setProjectId),
    grounding: { recipientId, dealId, projectId } satisfies Grounding,
    reasoning,
    setReasoning,
    scope,
    setScope,
  };
}

// What a sent message files under: the page it was written from, and every
// record the rep named while writing it.
//
// The anchor alone is not enough. A rep who picks "Related to → Acme Renewal"
// has said what the message is about, and a send that files only under the
// organization loses that: the deal's own timeline never sees the message, and
// nothing downstream can attribute the correspondence to the work it belongs
// to. The grounding choices ARE the attribution — they are the same statement,
// so they travel together.
//
// Duplicates are dropped rather than sent twice: on a person page the anchor
// and the recipient are the same record, and the link table treats a repeat as
// a conflict rather than a no-op. A link is identified by BOTH of its fields,
// the way the server identifies it — matching on the id alone would drop a
// legitimate second link whenever two records of different kinds happened to
// share a uuid, and the message would then be missing from that record's
// timeline with nothing to say why.
//
// `chosen` is null on a reply, whose links are the thread's own business: the
// rep picked nothing, and the recipient is already a participant on the
// activity being answered.
// Grounding is the three choices a rep made on the account-started path. It
// travels as the mutation VARIABLE of both the draft and the send, never out
// of the mutationFn's closure: the click handler belongs to the committed
// render, so a selection it passes cannot be older than the picker that shows
// it (see mutation-variable-coverage.test.ts for the stale-closure window).
export type Grounding = {
  recipientId: string;
  dealId: string;
  projectId: string;
};

function composedLinks(
  anchor: { entityType: RelinkKind; entityId: string },
  chosen: Grounding | null,
  // The project this send files under when the rep CHOSE nothing — derived
  // from the thread or from the anchor record, and already declinable in the
  // UI. Empty means no project link, which is what a decline produces.
  derivedProjectId = "",
): { entity_type: RelinkKind; entity_id: string }[] {
  const links: { entity_type: RelinkKind; entity_id: string }[] = [
    { entity_type: anchor.entityType, entity_id: anchor.entityId },
  ];
  const add = (kind: RelinkKind, id: string) => {
    const already = links.some(
      (l) => l.entity_type === kind && l.entity_id === id,
    );
    if (!id || already) {
      return;
    }
    links.push({ entity_type: kind, entity_id: id });
  };
  if (!chosen) {
    // An anchored send: the record it was started from, plus the filing it
    // states. A reply that states no filing still sends anchor-only, which is
    // the shape every existing reply has.
    add("project", derivedProjectId);
    return links;
  }
  add("person", chosen.recipientId);
  add("deal", chosen.dealId);
  add("project", chosen.projectId);
  return links;
}

// A freeform email-chip input: typed address + Enter/comma (or blur) adds a
// chip, the X icon removes it. No client-side email regex beyond type=email —
// the server is the authority (422 on a malformed address), so this never
// rejects what the backend might accept.
function RecipientField({
  label,
  values,
  onChange,
  onEditing,
  invalid,
}: Readonly<{
  label: string;
  values: string[];
  onChange: (next: string[]) => void;
  // Called the first time the reader types here. The committed values do not
  // say this: text sits in `draft` until Enter or blur, so a field the reader
  // is halfway through filling looks exactly like an untouched empty one to
  // anyone reading `values`.
  onEditing?: () => void;
  /** The send is waiting on this field, and the input carries the mark. */
  invalid?: boolean;
}>) {
  const t = useT();
  const [draft, setDraft] = useState("");
  const add = () => {
    const value = draft.trim();
    if (value && !values.includes(value)) onChange([...values, value]);
    setDraft("");
  };
  return (
    <div className="recipient-field">
      {/* No visible label of its own: the row it sits in already names it
          (MailRow), and the field printed a second "To" beside the first. The
          `label` is still the input's accessible name, which is the half a
          screen reader needs and a sighted reader gets from the row. */}
      <ul className="chips">
        {values.map((value) => (
          <li key={value}>
            {value}{" "}
            <button
              type="button"
              aria-label={t("compose.removeRecipient", { recipient: value })}
              onClick={() =>
                onChange(values.filter((other) => other !== value))
              }
            >
              <X size={14} aria-hidden />
            </button>
          </li>
        ))}
      </ul>
      <TextInput
        type="email"
        aria-label={label}
        value={draft}
        aria-invalid={invalid || undefined}
        onChange={(event) => {
          setDraft(event.target.value);
          onEditing?.();
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === ",") {
            event.preventDefault();
            add();
          }
        }}
        onBlur={add}
      />
    </div>
  );
}

// The account-started path's three choices: who this is to, which open deal
// it is about, and which project it belongs to.
//
// All are read off the account's own 360 rather than a fresh search, for the
// reason the whole draft is: the endpoint grounds itself in the caller's view
// of the account, so a contact this picker offers that the view does not carry
// would be one the draft then refuses.
function AccountDraftContext({
  orgId,
  recipientId,
  onRecipientChange,
  dealId,
  onDealChange,
}: Readonly<{
  orgId: string;
  recipientId: string;
  onRecipientChange: (next: string) => void;
  dealId: string;
  onDealChange: (next: string) => void;
}>) {
  const t = useT();
  const query = useOrganization360(orgId);
  // An overlay workspace has no native 360 to ground from; the endpoint
  // refuses there too, so the pickers simply have nothing to offer.
  const view = query.data?.state === "ready" ? query.data.view : undefined;
  const contacts = view?.people?.data ?? [];
  const deals = view?.deals?.data ?? [];

  // No contact on the account is an honest dead end for the DRAFT — the model
  // has no relationship to write from — and saying so beats an empty picker the
  // rep tries and cannot use. They can still type an address into To and write
  // the mail themselves.
  //
  // The PROJECT picker survives that dead end, and must: which body of work a
  // message is about has nothing to do with whether the account has a contact
  // yet. Returning early here took the project choice away from exactly the
  // message that most needs it — a check-in to an account nobody has spoken to
  // in a while, which is the first mail on a fresh delivery and the one with no
  // thread to inherit a project from. It would land unfiled, and the ladder
  // would ask about it in Approvals afterwards instead.
  if (query.isSuccess && contacts.length === 0) {
    return <p className="t-caption">{t("compose.noGroundableRecipient")}</p>;
  }
  return (
    <>
      <label className="t-body compose-check">
        {t("compose.draftTo")}
        <Select
          aria-label={t("compose.draftTo")}
          options={[
            { value: "", label: t("compose.draftToUnset") },
            ...contacts.map((contact) => ({
              value: contact.person_id,
              label: contact.full_name,
            })),
          ]}
          value={recipientId}
          onChange={onRecipientChange}
        />
      </label>
      {deals.length > 0 && (
        <label className="t-body compose-check">
          {t("compose.relatedTo")}
          <Select
            aria-label={t("compose.relatedTo")}
            options={[
              { value: "", label: t("compose.relatedToNone") },
              ...deals.map((deal) => ({
                value: deal.deal_id,
                label: deal.name,
              })),
            ]}
            value={dealId}
            onChange={onDealChange}
          />
        </label>
      )}
    </>
  );
}

// A reason's chip opens the record it names. Only the two kinds that HAVE a
// screen are routed: a fact or a profile field has a receipt rather than a
// page, and this dialog is the wrong place to open one over.
function openCited(entityType: string, entityId: string) {
  if (entityType === "deal") {
    navigate({ screen: "deals", id: entityId });
  }
  if (entityType === "person") {
    navigate({ screen: "contacts", id: entityId });
  }
}

// What the draft was written from, in the two shapes State D draws: a "Based
// on" line naming the inputs in order, and a row of "Why this draft?" chips.
//
// Both render the SAME reasons — they are one answer read two ways, which is
// why the server sends parts rather than a sentence. The line is for scanning
// before reading the draft; the chips are for checking one input after.
//
// A reason carrying evidence is pressable and opens the record it names. One
// without — the rep's own instruction — is flat, because there is nothing to
// open and a chip that looks pressable and is not is worse than a plain one.
function DraftReasons({
  reasons,
  onOpenRecord,
}: Readonly<{
  reasons: readonly components["schemas"]["AccountDraftReason"][];
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  if (reasons.length === 0) {
    return null;
  }
  return (
    <div className="compose-reasons">
      <p className="t-caption">
        {t("compose.basedOn", {
          inputs: reasons.map((reason) => reason.label).join(" · "),
        })}
      </p>
      <p className="t-caption">{t("compose.whyThisDraft")}</p>
      <ul className="chips">
        {reasons.map((reason) => (
          <li key={`${reason.kind}:${reason.label}`}>
            {reason.evidence_ref && onOpenRecord ? (
              <button
                type="button"
                className="link-button"
                onClick={() =>
                  onOpenRecord(
                    reason.evidence_ref?.entity_type ?? "",
                    reason.evidence_ref?.entity_id ?? "",
                  )
                }
              >
                {reason.label}
              </button>
            ) : (
              reason.label
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

// One control's worth of mutation state, flattened so a presentational child
// renders a pending/failed action without speaking react-query. `disabled` is
// wider than `pending`: a control is also barred while a sibling action that
// would contradict it is in flight.
type PendingAction = Readonly<{
  run: () => void;
  pending: boolean;
  disabled: boolean;
  error: string | null;
}>;

// The drawer BEFORE a machine has written anything: what the rep wants said,
// and the one control that asks for it. It is the pre-draft face of the same
// block the disclosure band takes over once a draft exists — the two never
// show together, because the band's whole claim is about words that are on
// screen.
//
// Rejecting a draft is not here. It is a verdict on the finished words and it
// sits in the action row with the other verdicts (send, cancel), where a rep
// decides what happens to the message rather than how it gets written.
function DraftOffer({
  intent,
  onIntentChange,
  draft,
  unavailable,
}: Readonly<{
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  unavailable: boolean;
}>) {
  const t = useT();
  return (
    <div className="compose-offer">
      <div className="compose-draftbar">
        <TextInput
          placeholder={t("compose.intent")}
          value={intent}
          onChange={(event) => onIntentChange(event.target.value)}
        />
        {/* The agent's own verb, so it carries the agent's own colour and its
            mark. Drawn as an ordinary ghost button it read as the quietest
            control in the drawer when it is the one thing in here a machine
            does. Indigo means "Margince does this" everywhere else on the
            record; a composer that said it in grey is the one surface where
            the reader has to guess. */}
        <Button
          small
          variant="ai"
          onClick={draft.run}
          disabled={draft.disabled}
          pending={draft.pending}
          busyLabel={t("compose.drafting")}
        >
          <Sparkles aria-hidden="true" />
          {t("compose.draftWithAi")}
        </Button>
      </div>
      {unavailable && (
        <p className="t-caption">{t("compose.draftUnavailable")}</p>
      )}
      {/* The failure appears without any navigation, so it is announced rather
          than merely coloured: a rep who cannot see the line has to be told
          the draft did not land, on the same terms the send refusals are. */}
      {!unavailable && draft.error && (
        <p
          className="t-caption"
          role="alert"
          style={{ color: "var(--danger)" }}
        >
          {draft.error}
        </p>
      )}
    </div>
  );
}

// The ways a send is refused for a reason the rep can act on, as opposed to
// failing. Anything outside this list keeps the server's own message on the
// modal's generic error line: inventing copy for a condition this surface does
// not understand would put words in the server's mouth.
export type Refusal = "consent" | "mailbox" | "sharedUnsubscribe" | null;

// The consent gate is a sentinel-mapped 409 and names itself at the top level;
// the two pre-flight refusals are 422s, where the top-level code is only ever
// "validation_error" and the rule that fired is the field + code pair the
// server asserted. Matching the field too keeps the copy tied to the input it
// is about: "reconnect your mailbox" is an answer about `from`, and would be
// wrong advice if some later rule refused `recipients` under the same code.
export function refusalOf(error: unknown): Refusal {
  if (error instanceof ProblemError && isConsentNotGranted(error.problem)) {
    return "consent";
  }
  for (const { field, code } of problemFieldErrorsOf(error)) {
    if (field === "from" && code === "mailbox_not_send_capable") {
      return "mailbox";
    }
    if (field === "recipients" && code === "shared_unsubscribe_token") {
      return "sharedUnsubscribe";
    }
  }
  return null;
}

// Each refusal states the condition and where it is resolved. The consent gate
// is the default-deny suppression (A22/ADR-0011) this surface exists to make
// visible. A mailbox connected before this product could send holds a read-only
// grant and the provider will not widen one in place, so reconnecting is the
// whole fix. And a message carrying an unsubscribe link carries ONE recipient's
// consent credential, so it may only ever have one addressee.
export function SendRefusal({
  refusal,
  personId,
}: Readonly<{ refusal: Refusal; personId?: string }>) {
  const t = useT();
  if (refusal === "consent") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">
          <strong>{t("compose.consentBlockedTitle")}</strong>
        </p>
        <p className="t-body" style={{ color: "var(--danger)" }}>
          {t("compose.consentBlocked")}
        </p>
        {personId && (
          <a href={`#/contacts/${personId}`} className="link-button">
            {t("compose.consentGoto")}
          </a>
        )}
      </div>
    );
  }
  if (refusal === "mailbox") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.mailboxNotSendCapable")}</p>
        <a href="#/settings/connections" className="link-button">
          {t("compose.mailboxNotSendCapableGoto")}
        </a>
      </div>
    );
  }
  if (refusal === "sharedUnsubscribe") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.sharedUnsubscribeToken")}</p>
      </div>
    );
  }
  return null;
}

// What rejecting a draft needs: the reference to name and the profile that
// served it. The pair IS the offer — non-null exactly when the judgment has a
// subject, and the request's own arguments when it is made.
function rejectionTarget(
  draftRef: string | null,
  voiceProfileId: string | null,
): { profileId: string; draftRef: string } | null {
  if (draftRef === null || voiceProfileId === null) {
    return null;
  }
  return { profileId: voiceProfileId, draftRef };
}

// sharedUnsubscribeAhead predicts the refusal above from what is on the form.
// Every purpose but the locked transactional one renders an unsubscribe link,
// and that link is one addressee's own consent record, so a second addressee is
// refused outright. This mirrors the server rule (which remains the authority)
// only to move a certain refusal ahead of the irreversible click.
const TRANSACTIONAL_PURPOSE = "transactional";

function sharedUnsubscribeAhead(
  to: string[],
  cc: string[],
  purpose: string,
): boolean {
  if (purpose === "" || purpose === TRANSACTIONAL_PURPOSE) {
    return false;
  }
  const addressees = new Set(
    [...to, ...cc].map((address) => address.trim().toLowerCase()),
  );
  return addressees.size > 1;
}

// The purpose list the rep chooses from. The unset entry is a real OPTION
// rather than the select's placeholder: a placeholder is only a face for an
// unset value, and a rep who picked a purpose has to be able to come back to
// none before sending. Its face stays the em dash the field has always shown —
// a glyph, with no words to translate.
function purposeOptions(
  purposes: readonly ConsentPurpose[] | undefined,
): SelectOption[] {
  return [
    { value: "", label: "—" },
    ...(purposes ?? []).map((purpose) => ({
      value: purpose.key,
      label: purpose.label,
    })),
  ];
}

// Send preconditions differ by wire shape: mail needs an addressee and a
// subject on top of a body; a channel reply carries neither (design §9.3 —
// the recipient is resolved server-side, and a channel has no subject line),
// so only the words and the consent purpose gate it.
// The one place the send's ORIGIN is chosen, so the three transports cannot
// drift into different bodies. An anchor sends the reply the composer has
// always sent; no anchor sends the account-started twin, which carries the
// records it is filed under instead of inheriting them from a conversation
// (ADR-0087 §1). A channel reply is anchor-only by nature — there is no such
// thing as starting a Telegram thread with a stranger from a company page —
// so it keeps the anchored path and never reaches the account arm.
async function sendFrom(args: {
  activityId?: string;
  isChannelReply: boolean;
  mail: {
    subject: string;
    body: string;
    to: string[];
    cc?: string[];
    draft_ref?: string;
    consent_purpose: string;
    scheduled_at?: string;
    scheduled_tz?: string;
  };
  channelBody: { body: string; consent_purpose: string };
  links: { entity_type: RelinkKind; entity_id: string }[];
}) {
  if (args.isChannelReply) {
    if (!args.activityId) {
      // A channel reply answers a conversation the person opened; there is no
      // way to start one with a stranger from a company page. Falling through
      // to the mail arm would post a channel message as an account EMAIL —
      // wrong transport, wrong body shape, and a message the rep never meant
      // to send by mail. No caller constructs this today; the refusal is what
      // keeps that true.
      throw new Error(
        "compose: a channel reply needs the conversation it answers",
      );
    }
    return api.POST("/activities/{id}/send-message", {
      params: { path: { id: args.activityId } },
      body: args.channelBody,
    });
  }
  if (args.activityId) {
    return api.POST("/activities/{id}/send-email", {
      params: { path: { id: args.activityId } },
      body: args.mail,
    });
  }
  return api.POST("/emails", { body: { ...args.mail, links: args.links } });
}

// scheduleFields turns the picker's wall-clock text into what the wire wants:
// an absolute instant plus the IANA zone the human chose it in.
//
// The browser gives "2026-08-17T09:00" — the rep's local time with no offset.
// `new Date(...)` resolves it against the browser's own zone, which is the zone
// the rep is looking at, so the instant is the one they meant. The zone NAME
// travels beside it rather than a numeric offset: an offset would freeze the
// DST rules of the day it was picked, and a message scheduled across a DST
// boundary would arrive an hour wrong.
//
// Empty means send now, which is what the composer meant before this existed.
export function scheduleFields(local: string): {
  scheduled_at?: string;
  scheduled_tz?: string;
} {
  if (local === "") return {};
  const at = new Date(local);
  if (Number.isNaN(at.getTime())) return {};
  return {
    scheduled_at: at.toISOString(),
    scheduled_tz: viewerZone(),
  };
}

// What the send is still waiting for, in the order the fields are read.
//
// A LIST rather than a boolean, because the Send button no longer refuses by
// going grey: a disabled control states that something is wrong and nothing
// about what, and the reader is left comparing the form against a button that
// will not talk to them. Pressing it now names every field it is waiting for,
// on the field itself.
export type MissingField = "to" | "subject" | "body" | "purpose";

export function missingToSend(
  isChannelReply: boolean,
  fields: { to: string[]; subject: string; body: string; purpose: string },
): readonly MissingField[] {
  const missing: MissingField[] = [];
  // A channel resolves its own recipient and carries no subject, so neither is
  // a thing this reader could supply.
  if (!isChannelReply && fields.to.length === 0) {
    missing.push("to");
  }
  if (!isChannelReply && fields.subject.trim() === "") {
    missing.push("subject");
  }
  if (fields.body.trim() === "") {
    missing.push("body");
  }
  if (fields.purpose === "") {
    missing.push("purpose");
  }
  return missing;
}

// The mail-only half of the composer: AI drafting (there is no draft-message
// endpoint for a channel) plus the recipient/subject inputs a channel reply's
// request shape has no room for. Kept as its own component so a channel
// reply — which renders none of this — doesn't inherit its branching.
// One row of the mail's own head: what it is, then what it says.
//
// Label BESIDE the value, in a column of its own width, because these five
// answers are read as a block — who it is to, what it is about, what it is
// filed under — and a stack of label-above-field turned that block into ten
// lines a reader travels rather than five they scan. The column is fixed, so
// nothing in it wraps into a two-word paragraph the way it did when the label
// was one flex item against a full-width control.
function MailRow({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <div className="mailrow">
      <span className="mailrow-label t-caption">{label}</span>
      <div className="mailrow-value">{children}</div>
    </div>
  );
}

// The band that says a MACHINE wrote the words below, and what it wrote them
// from. It is the Art. 50 disclosure and the draft's reasoning in one block,
// because to a reader they are one statement: this is not your colleague's
// message, and here is what it stands on.
//
// Indigo, like every other place a machine did the work. It is the loudest
// thing in the drawer on purpose — a rep who misses it sends a model's words
// under their own name.
//
// The server's disclosure line is a compliance string rendered verbatim, never
// reworded; a response that omits it still discloses, because a missing line
// may not silently become a missing disclosure.
//
// The voice tag names the PROFILE version that styled the draft, and the
// provisional label reports what that profile is today. Neither implies a
// weaker draft: nothing gates drafting on maturity, so a provisional profile
// styles this text exactly as a fuller one would. Both hang off the served
// version, because maturity is a corpus-word band that reaches `provisional`
// while the profile is still only collecting — and reporting a voice's
// maturity over a draft no voice touched would overstate this surface's own
// provenance, which Art. 50 does not permit.
function DraftBand({
  provenance,
  maturity,
  reasons,
  children,
}: Readonly<{
  provenance: DraftProvenance;
  maturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  // The steer and the verb that asks for another draft. They belong INSIDE
  // the band once one exists: the band is the machine's own block, and asking
  // it to write again is the same conversation rather than a control that
  // happens to sit nearby.
  children: ReactNode;
}>) {
  const t = useT();
  if (!provenance.ai_generated) {
    return null;
  }
  return (
    <section className="compose-band" data-testid="ai-disclosure-banner">
      <Eyebrow>{t("compose.aiDisclosureTitle")}</Eyebrow>
      <p className="t-body">
        {provenance.ai_disclosure || t("compose.aiDisclosureFallback")}
      </p>
      <DraftReasons reasons={reasons} onOpenRecord={openCited} />
      {provenance.voice_profile_version != null && (
        <>
          <p className="t-caption">
            {/* A profile VERSION, never grouped: version 1234 is one
                identifier, and "1.234" reads as a different one. */}
            {t("compose.voiceVersion", {
              n: identifierNumber(provenance.voice_profile_version),
            })}
          </p>
          {maturity === "provisional" && (
            <p className="t-caption">
              <Badge>{t("compose.provisional")}</Badge>{" "}
              {t("compose.provisionalHint")}
            </p>
          )}
        </>
      )}
      {children}
    </section>
  );
}

// The four things a rep asks the machine to do to its own draft, as one press
// each. Each is an instruction for ONE call — it never becomes the standing
// steer in the intent field.
// The label a rep reads and the instruction the model is given are two
// different strings and both are translated: the button says "Shorter" and the
// model is asked for it in a sentence, because an instruction of one word is
// one the model has to guess the scope of.
const REWRITES = [
  {
    key: "shorter",
    label: "compose.rewriteShorter",
    instruction: "compose.rewriteShorterAsk",
  },
  {
    key: "warmer",
    label: "compose.rewriteWarmer",
    instruction: "compose.rewriteWarmerAsk",
  },
  {
    key: "formal",
    label: "compose.rewriteFormal",
    instruction: "compose.rewriteFormalAsk",
  },
  {
    key: "deadline",
    label: "compose.rewriteDeadline",
    instruction: "compose.rewriteDeadlineAsk",
  },
] as const satisfies readonly {
  key: string;
  label: MessageKey;
  instruction: MessageKey;
}[];

// Offered only over the machine's OWN untouched words. Once the rep has
// edited the body, a rewrite would throw their work away to answer a question
// about text that is no longer there — so the row withdraws rather than
// growing a confirm nobody would read.
// `disabled` and not `pending`: these buttons do not report a write of their
// own, they refuse to start a second one. Named for what it does, because named
// for a state it does not have it read as the draft button's own spinner and a
// change to that button silently unblocked these.
function RewriteRow({
  onRewrite,
  disabled,
}: Readonly<{
  onRewrite: (instruction: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  return (
    <div className="compose-rewrite">
      <Eyebrow>{t("compose.rewrite")}</Eyebrow>
      {REWRITES.map((rewrite) => (
        <Button
          key={rewrite.key}
          small
          variant="aiQuiet"
          disabled={disabled}
          onClick={() => onRewrite(t(rewrite.instruction))}
        >
          <Sparkles aria-hidden="true" />
          {t(rewrite.label)}
        </Button>
      ))}
    </div>
  );
}

// What a field is still waiting for, said beside the field rather than in a
// summary at the bottom: a reader who has just pressed Send is looking at the
// control they must fix, not at a list of them.
function FieldNeed({ show, need }: Readonly<{ show: boolean; need: string }>) {
  if (!show) {
    return null;
  }
  return (
    <p className="t-caption compose-need" role="alert">
      {need}
    </p>
  );
}

function MailOnlyFields({
  intent,
  onIntentChange,
  draft,
  draftUnavailable,
  provenance,
  voiceMaturity,
  reasons,
  to,
  onToChange,
  onToEditing,
  cc,
  onCcChange,
  subject,
  onSubjectChange,
  rejectionInFlight,
  answering,
  flagged,
  deadRecipients,
}: Readonly<{
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  draftUnavailable: boolean;
  provenance: DraftProvenance | null;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  to: string[];
  onToChange: (next: string[]) => void;
  /** The reader has started typing a recipient, before any is committed. */
  onToEditing: () => void;
  cc: string[];
  onCcChange: (next: string[]) => void;
  subject: string;
  onSubjectChange: (next: string) => void;
  /** The fields a pressed Send is still waiting for. Empty until it is pressed. */
  flagged: ReadonlySet<MissingField>;
  rejectionInFlight: boolean;
  /** What this message answers, in the reader's own words. Null while the
   * lookup is unsettled — an unanswered read is not "no earlier message". */
  answering: string | null;
  /** The recipients on this draft that are known not to arrive. */
  deadRecipients: readonly string[];
}>) {
  const t = useT();
  const offer = (
    <DraftOffer
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
      {/* WHAT is being answered, above the recipient — the first thing a
          reader looks for and the thing this dialog used to leave out. Opened
          from a record page it named no conversation at all, so a reader could
          press "Draft with AI" without knowing who they were writing to. */}
      {answering !== null && (
        <p className="t-small mw-answering">{answering}</p>
      )}
      <MailRow label={t("compose.to")}>
        <RecipientField
          label={t("compose.to")}
          values={to}
          onChange={onToChange}
          onEditing={onToEditing}
          invalid={flagged.has("to")}
        />
        <FieldNeed
          show={flagged.has("to")}
          need={t("compose.emptyRecipients")}
        />
      </MailRow>
      <MailRow label={t("compose.cc")}>
        <RecipientField
          label={t("compose.cc")}
          values={cc}
          onChange={onCcChange}
        />
      </MailRow>
      {/* Under the addresses, because it is about the ones standing there —
          and a warning rather than a refusal: the rep may know something the
          ledger does not, and a bounce is a fact about the past. */}
      {deadRecipients.length > 0 && (
        <Callout tone="warn" live="status">
          {t("compose.deadRecipients", {
            addresses: deadRecipients.join(", "),
          })}
        </Callout>
      )}
      <MailRow label={t("compose.subject")}>
        <TextInput
          aria-label={t("compose.subject")}
          placeholder={t("compose.subject")}
          value={subject}
          disabled={rejectionInFlight}
          aria-invalid={flagged.has("subject") || undefined}
          onChange={(event) => onSubjectChange(event.target.value)}
        />
        <FieldNeed
          show={flagged.has("subject")}
          need={t("compose.missingSubject")}
        />
      </MailRow>
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
  purpose,
}: Readonly<{ to: string[]; cc: string[]; purpose: string }>) {
  const t = useT();
  return (
    <>
      {sharedUnsubscribeAhead(to, cc, purpose) && (
        <p className="t-caption" style={{ color: "var(--danger)" }}>
          {t("compose.multiRecipientWarning")}
        </p>
      )}
    </>
  );
}

// The 🟡 confirm-first composer (draftEmail + sendEmail), extended for a
// captured messaging channel (sendMessage): on a `kind === "message"`
// activity the reply posts /activities/{id}/send-message instead, dropping
// subject and Cc — a channel has no concept of either, and the recipient is
// never named by the caller (design §9.3): the server resolves it from the
// conversation's own channel identity. Everything else — the confirm-first
// interaction, AI drafting (mail-only; there is no draft-message endpoint),
// the 409 consent rendering, and the post-send timeline refresh — is shared.
// Draft with AI fills the fields; the human edits and confirms; the human's
// own click IS the approval (ADR-0055), so the human REST path sends no
// X-Approval-Token and no Idempotency-Key — that plumbing is the
// agent/passport path. The 409 consent gate is the whole reason this surface
// exists: the default-deny suppression (A22/ADR-0011) has never been visible
// to a user before.
// The drafting call, both origins, lifted out of ComposeModal — which already
// carries the send, the consent gate, the refusal vocabulary and the
// voice-rejection flow, and was over the complexity bar with this inside it.
//
// The two paths answer the same shape on purpose, so the fill below cannot
// tell them apart and the origins cannot drift into different clobber rules.
// What one drafting call is asked under: where it is grounded, and — for a
// rewrite — the instruction that replaces the rep's own steering for that call
// alone. A rewrite may not overwrite what the rep typed in the steer field:
// they would come back to a box holding "make it shorter" as their standing
// instruction for every draft after.
type DraftAsk = Readonly<{
  grounding: Grounding;
  instruction?: string;
}>;

function useDraftMutation({
  activityId,
  entityType,
  entityId,
  intent,
  onUnavailable,
  onDrafted,
  resetUnavailable,
  t,
}: Readonly<{
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  intent: string;
  onUnavailable: () => void;
  onDrafted: (
    result: Extract<DraftResult, { available: true }>,
    ask: DraftAsk,
  ) => void;
  resetUnavailable: () => void;
  t: ReturnType<typeof useT>;
}>) {
  return useMutation({
    mutationKey: ["email-draft", entityId],
    mutationFn: async (ask: DraftAsk): Promise<DraftResult> => {
      resetUnavailable();
      const { grounding } = ask;
      const intentOf = ask.instruction ?? intent;
      // A reply answers the message it is anchored to; an account-started
      // message has none, so it is grounded in the account itself and needs
      // the recipient named first.
      if (activityId) {
        return draftFromActivity({ activityId, intent: intentOf, t });
      }
      return draftFromAccount({
        entityType,
        entityId,
        ...grounding,
        intent: intentOf,
        t,
      });
    },
    onSuccess: (result, ask) => {
      if (!result.available) {
        onUnavailable();
        return;
      }
      onDrafted(result, ask);
    },
  });
}

/**
 * The project the anchor RECORD names, for the sends whose anchor can name one.
 *
 * Only a deal does today: it carries `project_id` as a column, and a message
 * about a deal is a message about that deal's work. A company or a person
 * reaches several projects at once and names none of them, so there is nothing
 * to derive — those anchors answer nothing here and the account path's picker
 * is what asks.
 */
function useAnchorProject(
  entityType: RelinkKind,
  entityId: string,
): { projectId?: string | null; companyId?: string; settled: boolean } {
  const query = useQuery({
    // The SAME key the deal page's own read uses, so opening the composer on a
    // deal costs no request and cannot disagree with the page behind it. A
    // second key would be a second answer to one question.
    queryKey: ["deal", entityId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}", {
        params: { path: { id: entityId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: entityType === "deal",
    staleTime: 60_000,
  });
  return {
    projectId: query.data?.project_id,
    // The company whose projects the picker offers. A deal names one; a
    // company page IS one; the other anchors reach none this composer can ask
    // about.
    companyId:
      entityType === "deal"
        ? (query.data?.organization_id ?? undefined)
        : entityType === "organization"
          ? entityId
          : undefined,
    // A read that failed says nothing about this deal's project. Reporting it
    // as "no project" would drop the filing silently; unsettled keeps the
    // composer quiet instead, which is the same rule the thread read follows.
    settled: entityType !== "deal" || (!query.isPending && !query.isError),
  };
}

/**
 * Which project this message files under, and the subject tag that follows it.
 *
 * The rep chooses; the composer only SUGGESTS. The suggestion is the thread's
 * own project when the conversation is already filed — a conversation is one
 * body of work and a sibling settled it — and otherwise the anchor's, which is
 * a deal's project. It is adopted once, when it first arrives, so a rep who
 * then picks None is not overruled on the next render.
 *
 * The tag is KEPT in the subject for as long as a project is chosen, put back
 * whenever the field stops carrying it — a subject is replaced wholesale by a
 * draft arriving or by a rep retyping it, and a tag written once at the moment
 * of choosing does not survive either. Removing it is done through the picker,
 * which is what the send honours; there is no state where the text and the
 * picker disagree.
 */
function useProjectFiling(input: {
  activityId?: string;
  anchorProjectId?: string | null;
  anchorSettled: boolean;
  projects: readonly PickableProject[];
  subject: string;
  setSubject: (next: string) => void;
}): { projectId: string; setProjectId: (next: string) => void } {
  const thread = useThreadProject(input.activityId);
  // Empty string is a real answer ("None"), so unanswered is undefined.
  const [picked, setPicked] = useState<string | undefined>(undefined);
  const settled = thread.settled && input.anchorSettled;
  const suggested = settled
    ? (thread.projectId ?? input.anchorProjectId ?? "")
    : "";
  // Adopted once, when the suggestion first arrives. `picked` then holds it, so
  // a rep who chooses None is not overruled on the next render.
  const adopted = useRef("");
  useEffect(() => {
    if (suggested && adopted.current !== suggested) {
      adopted.current = suggested;
      setPicked(suggested);
    }
  }, [suggested]);
  const chosen = picked ?? suggested;
  // The account path has no thread and no deal to suggest from, so its rule is
  // the older one: a company with exactly ONE live project defaults to it. The
  // two never both fire — a suggestion above means there was something to
  // inherit, and this only applies when there was not.
  useSoleProjectDefault(input.projects, chosen, setPicked);
  // A value the option list does not carry is not shown as chosen — the control
  // would render blank while the subject claimed a filing nobody can see named.
  // Read rather than written: writing it back would race the adoption above,
  // clearing the suggestion on the render before the list arrives and leaving
  // `adopted` unwilling to try again. The list settling is what makes it
  // appear.
  const offered =
    chosen === "" ||
    input.projects.some((project) => project.project_id === chosen);
  const projectId = offered ? chosen : "";
  const { project } = useProjectRecord(projectId || undefined);
  const tag = subjectTag(project);
  // Keeping the tag in the subject is a rule about the FIELD, not an action
  // taken once when the picker moves. A subject is replaced wholesale — by a
  // draft arriving, or by a rep selecting all and retyping — and a tag written
  // only at the moment of choosing is gone the first time either happens.
  //
  // So the tag is kept at the front of the subject for as long as a project is
  // chosen. Taking it off is done through the picker (choose No project), which
  // is the control that means it; there is no way to half-choose a project by
  // editing the text, because that state is not one the send could honour.
  const setSubject = input.setSubject;
  const subject = input.subject;
  const previousTag = useRef("");
  useEffect(() => {
    const priorTag = previousTag.current;
    previousTag.current = tag;
    const withoutOld = priorTag ? stripSubjectTag(subject, priorTag) : subject;
    const wanted = tag ? withSubjectTag(withoutOld, tag) : withoutOld;
    if (wanted !== subject) {
      setSubject(wanted);
    }
  }, [tag, subject, setSubject]);
  return { projectId, setProjectId: setPicked };
}

// The three moments a rep actually picks when they choose not to send now.
//
// Built from a `Date` the caller passes rather than read off the clock in
// here, so the presets a test sees are the presets it set up.
function schedulePresets(now: Date): readonly {
  key: MessageKey;
  at: Date;
}[] {
  const at = (days: number, hour: number) => {
    const day = new Date(now);
    day.setDate(day.getDate() + days);
    day.setHours(hour, 0, 0, 0);
    return day;
  };
  // Monday from a Monday is NEXT Monday: a rep picking "Monday morning" on a
  // Monday afternoon means the one that has not happened yet.
  const untilMonday = (8 - now.getDay()) % 7 || 7;
  return [
    { key: "compose.scheduleTomorrow", at: at(1, 8) },
    { key: "compose.scheduleAfternoon", at: at(1, 13) },
    { key: "compose.scheduleMonday", at: at(untilMonday, 8) },
  ];
}

// The hours a business message is actually scheduled for. Four, not a clock:
// the choice is "start of the day, mid-morning, after lunch, end of the day",
// and a rep who wants 11:47 is not a rep this control is for.
const SCHEDULE_HOURS = [8, 9, 13, 17] as const;

// A moment as `datetime-local` spells it: the wall clock, no offset. Built
// field by field rather than sliced out of an ISO string, because
// `toISOString` is UTC and would move every preset by the reader's own offset.
function localMoment(at: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${isoDay(at)}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

/** A `datetime-local` value back as a `Date`, or nothing if the field is empty. */
function momentOf(local: string): Date | null {
  if (!local) {
    return null;
  }
  const at = new Date(local);
  return Number.isNaN(at.getTime()) ? null : at;
}

/** A scheduled moment, spelled the way every line in this drawer spells one. */
function momentLabel(at: Date, locale: Locale, zone: string): string {
  return `${formatDateTime(at.toISOString(), locale, zone)} · ${zoneNameAndOffset(INTL_LOCALE[locale], at)}`;
}

// The other way to send, behind the confirm button's own caret.
//
// A moment is a choice ABOUT the send rather than a field of the message, so
// it belongs with the send control and not in the form above it — it was a
// datetime field between the consent purpose and the recipient warnings, which
// is where a rep reads the message rather than where they decide to release
// it.
function ScheduleMenu({ onOpen }: Readonly<{ onOpen: () => void }>) {
  const t = useT();
  return (
    <Popover
      variant="primary"
      className="compose-sendmenu"
      label={
        <>
          <ChevronUp aria-hidden="true" size={16} />
          <span className="sr-only">{t("compose.sendOptions")}</span>
        </>
      }
    >
      <Button variant="ghost" onClick={onOpen}>
        <Clock aria-hidden="true" size={16} />
        {t("compose.scheduleSend")}
      </Button>
    </Popover>
  );
}

// Choosing when the message goes out: the three moments most sends take, and
// the calendar for the ones that do not.
//
// Two steps in one dialog rather than two dialogs. The presets ARE the answer
// most of the time, and a rep who wants one of them should not have to walk a
// calendar to reach it; a rep who wants a different Thursday needs the month
// in front of them. One dialog keeps that as one decision.
function ScheduleDialog({
  open,
  onClose,
  sendAt,
  onChoose,
  now,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  sendAt: string;
  onChoose: (next: string) => void;
  // The clock, passed in: a component that read it itself could not be tested
  // against a fixed set of presets.
  now: Date;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const headingId = useId();
  const [picking, setPicking] = useState(false);
  const chosen = momentOf(sendAt) ?? now;
  const [day, setDay] = useState<ISODay>(isoDay(chosen));
  const [hour, setHour] = useState(chosen.getHours());
  const [month, setMonth] = useState(
    () => new Date(chosen.getFullYear(), chosen.getMonth(), 1),
  );
  // Each opening starts from the current moment, not wherever the last
  // opening left the calendar — a rep who paged to December and closed
  // without choosing should not find December still showing next time.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep — this re-seeds the picker on open, not on every change to sendAt/now while it's already open.
  useEffect(() => {
    if (!open) return;
    const start = momentOf(sendAt) ?? now;
    setPicking(false);
    setDay(isoDay(start));
    setHour(start.getHours());
    setMonth(new Date(start.getFullYear(), start.getMonth(), 1));
  }, [open]);
  const picked = new Date(`${day}T${String(hour).padStart(2, "0")}:00`);
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} size="wide">
      <h2 id={headingId} className="t-h2">
        {picking ? t("compose.schedulePick") : t("compose.scheduleSend")}
      </h2>
      {picking ? (
        <>
          <div className="schedule-pick">
            <Calendar
              month={month}
              onMonthChange={setMonth}
              selected={day}
              onSelect={setDay}
              today={now}
              locale={locale}
            />
            <div className="schedule-when">
              <Eyebrow>{t("compose.scheduleDate")}</Eyebrow>
              <p className="schedule-date t-body">
                {formatDateAbbrev(picked.toISOString(), locale, zone)}
              </p>
              <Eyebrow>{t("compose.scheduleTime")}</Eyebrow>
              <div className="schedule-hours">
                {SCHEDULE_HOURS.map((option) => (
                  <button
                    key={option}
                    type="button"
                    className="schedule-hour t-mono"
                    aria-pressed={option === hour}
                    onClick={() => setHour(option)}
                  >
                    {`${String(option).padStart(2, "0")}:00`}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <p className="t-caption schedule-foot">
            {t("compose.scheduleGoesOut", {
              when: momentLabel(picked, locale, zone),
            })}
          </p>
          <div className="actions">
            <Button onClick={onClose}>{t("create.cancel")}</Button>
            <Button
              variant="primary"
              onClick={() => {
                onChoose(localMoment(picked));
                onClose();
              }}
            >
              {t("compose.scheduleSend")}
            </Button>
          </div>
        </>
      ) : (
        <div className="schedule-presets">
          <p className="t-caption">
            {zoneNameAndOffset(INTL_LOCALE[locale], now)}
          </p>
          {schedulePresets(now).map((preset) => (
            <button
              key={preset.key}
              type="button"
              className="schedule-preset"
              onClick={() => {
                onChoose(localMoment(preset.at));
                onClose();
              }}
            >
              <span className="t-body">{t(preset.key)}</span>
              <span className="t-caption">
                {formatDateTime(preset.at.toISOString(), locale, zone)}
              </span>
            </button>
          ))}
          <Button variant="ghost" onClick={() => setPicking(true)}>
            <CalendarDays aria-hidden="true" size={16} />
            {t("compose.schedulePick")}
          </Button>
          {/* Only once a moment is set. Offered over an unscheduled send it
              would be a control that undoes nothing. */}
          {sendAt !== "" && (
            <Button
              variant="ghost"
              onClick={() => {
                onChoose("");
                onClose();
              }}
            >
              {t("compose.scheduleNow")}
            </Button>
          )}
        </div>
      )}
    </Modal>
  );
}

// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: this modal was already at the ceiling; the account-started origin (ADR-0087/A132) adds three necessary branches — the recipient/deal pickers, the grounded-draft gate and the drawer placement. The drafting call, the form fill, the body edit and both draft controls are already extracted; what is left is one dialog's own wiring, and splitting the send/consent/refusal/voice flow apart from the fields it gates would scatter it.
export function ComposeModal({
  activityId,
  entityType,
  entityId,
  personId,
  kind,
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
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  personId?: string;
  // Undefined (or any non-channel kind) keeps the mail behaviour this modal
  // already had before a channel existed — every mail test renders this
  // component without ever naming a kind.
  kind?: Activity["kind"];
  open: boolean;
  onClose: () => void;
  // A message left this composer — sent now or scheduled. Called before the
  // close, and only then: a caller whose own reading of the record depends on
  // what was sent re-asks on this, and not on a cancel that changed nothing.
  onSent?: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const purposes = useConsentPurposes();
  const voiceProfile = useVoiceProfile();
  // Which ENDPOINT the reply posts to is a question about the kind, not the
  // transport: every channel message goes to send-message whatever carried it,
  // and the server resolves the recipient from the conversation's own channel
  // identity (design §9.3).
  const isChannelReply = kind === "message";
  const [to, setTo] = useState<string[]>([]);
  const [cc, setCc] = useState<string[]>([]);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [intent, setIntent] = useState("");
  const [purpose, setPurpose] = useState("");
  // The moment a rep chose to send at, as the browser's datetime-local gives
  // it: wall-clock text in THEIR zone, with no offset. It becomes an absolute
  // instant at submit — see scheduleFields.
  const [sendAt, setSendAt] = useState("");
  // Whether the moment picker is up. Separate from `scheduling` below, which
  // says a moment has been CHOSEN: the dialog is open while the rep is still
  // deciding, and closing it without picking leaves the send where it was.
  const [pickingMoment, setPickingMoment] = useState(false);
  const { locale } = useLocale();
  const zone = viewerZone();
  const toast = useToast();
  const [provenance, setProvenance] = useState<DraftProvenance | null>(null);
  // The served voice draft the body in this form came from. It is what lets the
  // server say whether the rep sent the draft or rewrote it, so it may only ever
  // name the text actually on screen.
  const [draftRef, setDraftRef] = useState<string | null>(null);
  // The words as the model last served them. Compared against `body`, it is
  // what says whether the text on screen is still the machine's — which is the
  // only text a rewrite may replace.
  const [servedBody, setServedBody] = useState("");
  // Two honest non-error outcomes, kept OUT of react-query's error channel so
  // the form stays usable: the model / mailer simply isn't configured (501).
  const [draftUnavailable, setDraftUnavailable] = useState(false);
  const [sendUnavailable, setSendUnavailable] = useState(false);
  // Retiring the previous pair's draft is the composer's job, not the
  // grounding hook's: the body, the recipients, the reference and the
  // disclosure all live here.
  //
  // Only DRAFTED text is cleared. `draftRef` and `provenance` are set only by
  // the fill, so their presence is what says the words on screen came from a
  // draft rather than from the rep — and a rep who typed their own message
  // and then picked a different contact must not lose it.
  const account = useAccountGrounding(personId, () => {
    if (!provenance && !draftRef) {
      return;
    }
    setBody("");
    setSubject("");
    setTo([]);
    setDraftRef(null);
    setProvenance(null);
  });
  // Whether this composer can ground a draft in an account: an account-started
  // message on a company page, over mail. A channel reply resolves its
  // recipient server-side and has no draft endpoint at all.
  // What this composer is answering. The caller names it when the reader opened
  // a specific message; opened from a record page it does not, and the reader
  // was then asked to authorise a draft against a conversation nothing on
  // screen identified. The project narrows it, so switching projects switches
  // the conversation being continued.
  const latest = useLatestMessage(
    entityType,
    entityId,
    account.grounding.projectId,
    open && !activityId,
  );
  // The conversation the reader PICKED, when they picked one. A composer opened
  // from the record used to anchor itself to the account's latest exchange
  // without asking: the reader pressed a button that says "write an email" and
  // got a reply to whatever came last, addressed to somebody they had not
  // chosen. The threads are offered beside the form now, and this holds their
  // answer. The old behaviour's benefit survives — continuing the last exchange
  // is still one press — without the surprise.
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (!open) {
      setChosen(undefined);
    }
  }, [open]);
  // Offered on every record's account-started mail. A caller that anchored the
  // send (a timeline Reply, a panel that resolved what is owed) has already
  // chosen, so nothing is offered over its choice; a channel reply's
  // conversation is the provider's, with nothing of ours to pick from.
  const offeringThreads = !isChannelReply && !activityId;
  const answering =
    activityId ?? (offeringThreads ? chosen : latest.activity?.id);
  // Addressing the reply before a draft is asked for. A channel reply resolves
  // its recipient server-side and shows no To field, so it asks nothing.
  const threadRecipient = useReplyRecipient(
    open && !isChannelReply ? answering : undefined,
  );
  // Offered ONCE, when the lookup first answers, and never again.
  //
  // Keyed on the recipient rather than on the field being empty, because an
  // empty field is not the same as an untouched one. `to` stays empty while
  // RecipientField holds text the reader has typed but not yet committed, so
  // a lookup landing mid-word would add the thread's address and then the
  // reader's — sending the reply to somebody they never chose. It is also
  // empty again after they DELETE the prefill, and re-adding what they just
  // removed makes the field impossible to clear.
  //
  // The ref, not state: this decides whether to offer at all, so re-rendering
  // on it would be a render caused by the thing it is trying not to do twice.
  const offered = useRef(false);
  // WHICH address was offered, so that on a change of conversation it is the
  // one thing replaced. Without it the field kept the first thread's
  // counterparty while the pane showed another's messages, and a reply to the
  // second conversation went to the first one's correspondent.
  const offeredAddress = useRef<string | undefined>(undefined);
  // The offer's slot was just emptied by a change of conversation, so the next
  // anchor's address goes in even beside recipients the reader typed — the
  // slot is ours, and what stands next to it is theirs.
  const vacated = useRef(false);
  // The reader has taken the field over. Set on the first keystroke, which is
  // the only signal there is — the committed values stay empty until they
  // press Enter.
  const stopOfferingRecipient = useCallback(() => {
    offered.current = true;
  }, []);
  // A reader who picks a DIFFERENT conversation is owed its address: the offer
  // is once per anchor, not once per composer. Without this the field kept the
  // first thread's counterparty while the pane showed another's messages. The
  // previous offer leaves the field HERE, on the change itself, rather than
  // when the new address resolves — a lookup still out, or a conversation with
  // no address on record, must not leave the old counterparty standing as the
  // recipient of a reply to somebody else. What the reader added beside it is
  // theirs and stays.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the anchor alone — this re-arms the offer when the conversation changes, not when its address resolves.
  useEffect(() => {
    offered.current = false;
    const previous = offeredAddress.current;
    if (previous === undefined) {
      return;
    }
    offeredAddress.current = undefined;
    vacated.current = true;
    setTo((current) => current.filter((address) => address !== previous));
  }, [answering]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the anchor is a key here, not a read — two conversations with one counterparty resolve to the same address, and the offer the change above vacated has to be made again even though the address did not move.
  useEffect(() => {
    if (threadRecipient === undefined || offered.current) {
      return;
    }
    offered.current = true;
    offeredAddress.current = threadRecipient;
    // A draft that named its own recipient, or a reader who committed one
    // before the lookup answered, both outrank the thread's default — unless
    // the field is non-empty only because the slot a previous offer held was
    // just vacated, in which case this anchor's address takes that slot.
    // The refs are settled before the updater, which stays a pure function
    // of the field: StrictMode may run it twice.
    const fillsVacancy = vacated.current;
    vacated.current = false;
    setTo((current) => {
      if (current.length > 0 && !fillsVacancy) {
        return current;
      }
      return current.includes(threadRecipient)
        ? current
        : [...current, threadRecipient];
    });
  }, [threadRecipient, answering]);
  // The anchor as a RECORD, not just an id: the conversation pane reads its
  // `thread_key`. A caller-named activity is fetched (that read already runs,
  // for the thread's filing); a resolved one is the row `latest` is holding.
  const viewerId = useViewerId();
  // What the conversation's rows are CALLED. An activity link carries ids, and
  // "Sent to 8f21c4…" is not a reader telling you who was on a message. Two
  // sources, because a thread has two sides: colleagues come from the workspace
  // roster, the account's own people from the record behind this drawer — whose
  // read is already in cache there, so this costs the composer nothing on the
  // page it opens over.
  const roster = useRoster("user", open);
  const namesOrg = useOrganization360(
    entityType === "organization" ? entityId : "",
  );
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  const records = recordNamesIn(
    namesOrg.data?.state === "ready" ? namesOrg.data.view : undefined,
  );
  const nameOf = (linkType: string, linkId: string) =>
    linkType === "user" ? colleagues.get(linkId) : records(linkType, linkId);
  const anchorRead = useThreadProject(activityId ?? chosen);
  // The latest message stands in only when it IS the anchor being answered.
  // While a caller-named or picked anchor is still loading, standing the
  // newest message in would draw a conversation the reader did not choose —
  // and offer its counterparty — for a beat, then swap it.
  const anchorActivity = answering
    ? (anchorRead.activity ??
      (answering === latest.activity?.id ? latest.activity : undefined))
    : undefined;
  const conversation = useThreadMessages(open ? anchorActivity : undefined);
  // An anchor named but not yet read. The pane holds its place on this, so
  // the drawer does not open narrow and snap wide when the read answers. A
  // read that FAILED is not one still arriving: the pane lets go, and the
  // composer keeps answering the anchor it was given without drawing it.
  const anchorUnresolved =
    Boolean(answering) && anchorActivity === undefined && !anchorRead.failed;
  const recent = useRecentConversations(
    entityType,
    entityId,
    open && offeringThreads && !chosen,
    { nameOf, t, locale },
  );
  // The sentence the reader reads. Null while the lookup is unsettled: an
  // unanswered read is not "no earlier message", and saying so would tell them
  // this starts a new thread when it may continue one.
  const answeringLine = answeringSentence(
    latest,
    activityId,
    to,
    t,
    locale,
    zone,
  );

  // The account-started path: no conversation to continue, so the reader names
  // the recipient and the draft is grounded in the account itself.
  //
  // Keyed on the RESOLVED anchor, not the caller's. A record with earlier mail
  // has a message to answer, and answering it is what a reader opening the
  // composer there means — the account path asked them to name a recipient the
  // thread already knows, in front of a To field the draft would have filled.
  const groundable =
    !answering && entityType === "organization" && !isChannelReply;
  // What SHAPE the composer takes, split from what it GROUNDS. One flag used to
  // answer both, so a reply inherited the account path's box — and an account
  // that had mail lost the drawer that path was given. The shape is every
  // record's, not the account's: a mail written from a person, lead or deal
  // keeps that record on screen beside it just as an account's does, so the
  // same verb cannot change shape with the page it was pressed on.
  const asDrawer = !isChannelReply;
  // The conversation rides beside the form only where there IS one and there is
  // room for a second column. A channel reply answers a live conversation the
  // provider owns, and has no thread of its own to draw.
  //
  // Correspondence only. A timeline row offers Reply on a note as readily as on
  // a mail, and a note is not a conversation: drawn beside the form under the
  // heading "this conversation", one filed note claimed to be the exchange the
  // reply continues.
  const answeringCorrespondence =
    anchorActivity?.kind === "email" || anchorActivity?.kind === "message";
  // Held open while the anchor or its thread is still loading, for the same
  // reason the offer column below holds its place: the shape is decided by
  // what is being answered, not by when the network answers. An anchor that
  // resolves to a filed note collapses the column then — the one case that
  // still changes shape, and the honest one.
  const showConversation =
    asDrawer &&
    ((answeringCorrespondence &&
      (conversation.messages.length > 0 ||
        conversation.pending ||
        conversation.failed)) ||
      anchorUnresolved);
  // The ways in, when the reader has not taken one. Nothing to offer is not a
  // column: an account with no mail gets the plain drawer it had before.
  // While the lookup is still out, the column holds its place with the
  // pending body — decided by the answer, the drawer opened narrow and
  // snapped wide a beat later, a shape change under a reader already aiming
  // at a field.
  // A read that failed keeps the column too, with the failure and a retry in
  // it: collapsed, the composer would offer a fresh mail as though the record
  // had no history, which is the surprise the choices exist to prevent.
  const showChoices =
    offeringThreads &&
    !chosen &&
    (recent.pending || recent.failed || recent.conversations.length > 0);
  const splitColumns = showConversation || showChoices;
  // Where this send files, and the subject tag that travels with it. The
  // account path keeps its own picker (it chooses a project rather than
  // inheriting one), so this covers the anchored sends: a reply, and a message
  // started from a deal.
  const anchorProject = useAnchorProject(entityType, entityId);
  // The projects this message may be filed under: the live ones the anchor's
  // company reaches. That set already includes the projects the company works
  // as a partner or a subcontractor, because the 360's own section is built
  // from the company edges rather than from a project's anchor column.
  const anchorCompany = useOrganization360(anchorProject.companyId ?? "");
  const reachableProjects = liveProjects(
    anchorCompany.data?.state === "ready"
      ? anchorCompany.data.view.projects
      : undefined,
  );
  const projectFiling = useProjectFiling({
    activityId: answering,
    anchorProjectId: anchorProject.projectId,
    anchorSettled: anchorProject.settled,
    projects: reachableProjects,
    subject,
    setSubject,
  });

  // An emptied body no longer holds the served draft, so everything that
  // describes those words goes with it: the reference would bind the next
  // send's outcome to text the rep deleted, the disclosure would announce a
  // model draft over an empty field, and the reasons would explain a draft
  // that is no longer there. The fill re-adopts all three on the next draft.
  const editBody = (next: string) => {
    setBody(next);
    if (next) {
      return;
    }
    setDraftRef(null);
    setProvenance(null);
    account.setReasoning([]);
    account.setScope(undefined);
  };

  const draft = useDraftMutation({
    activityId: answering,
    entityType,
    entityId,
    intent,
    onUnavailable: () => setDraftUnavailable(true),
    onDrafted: (result, ask) =>
      fillFromDraft(result, {
        subject,
        body,
        toEmpty: to.length === 0,
        rewrite: ask.instruction !== undefined,
        setSubject,
        setBody,
        setServedBody,
        setTo,
        setDraftRef,
        setProvenance,
        setReasoning: account.setReasoning,
        setScope: account.setScope,
      }),
    resetUnavailable: () => setDraftUnavailable(false),
    t,
  });

  // Rejecting a draft is a judgment the rep makes, so it has its own control
  // and never rides on closing the dialog. It also may not be guessed at: the
  // reference is deterministic and the drafted signal is inserted once, so a
  // rejection recorded because someone navigated away would silently stand in
  // for the real outcome of an identical draft that is later sent.
  const rejectable = rejectionTarget(draftRef, voiceProfile.data?.id ?? null);
  const discard = useMutation({
    mutationFn: async (rejected: { profileId: string; draftRef: string }) => {
      const { error, response } = await api.POST(
        "/voice-profiles/{id}/draft-rejections",
        {
          params: { path: { id: rejected.profileId } },
          body: { draft_ref: rejected.draftRef },
        },
      );
      // The rejection landed only on a real 2xx. openapi-fetch reports a falsy
      // `error` for a bodiless non-2xx (a gateway 502/503/504), and treating
      // that as success would clear the draft off this surface while the
      // server's signal is still open — the rep would be told their verdict
      // was recorded when it never left the building.
      if (!response.ok) {
        throwProblem(error || { title: t("compose.actionFailed") });
      }
    },
    onMutate: () => {
      // A rejection and a send are contradictory verdicts on one draft, and
      // whichever reaches the signal first owns it for good. Dropping the
      // reference the moment the judgment leaves means a send that starts
      // anyway carries no draft at all: an unrecorded send, never a message
      // that actually went out filed as rejected. The control withdraws with
      // it, which is where a succeeding rejection leaves the surface anyway.
      setDraftRef(null);
    },
    onError: (_error, rejected) => {
      // The rejection never landed, so the signal is still open and the words
      // on screen are still the ones it named. Restoring the reference keeps
      // the judgment retryable and lets a send that follows report honestly.
      setDraftRef(rejected.draftRef);
    },
    onSuccess: () => {
      // The rejected words leave with the judgment; the recipients the rep
      // addressed are their own work and stay. So does the project tag: it is
      // not part of the draft being rejected, and clearing it would leave the
      // checkbox claiming a filing the subject no longer carries.
      setSubject((current) => keptTag(current));
      setBody("");
      setProvenance(null);
    },
  });

  const send = useMutation({
    mutationKey: ["email", entityId],
    // The grounding is the variable, not a closure read: a stale closure
    // could file the mail under a project the picker no longer shows.
    mutationFn: async (grounding: Grounding | null) => {
      setSendUnavailable(false);
      // No X-Approval-Token, no Idempotency-Key on either path: the human's
      // own click IS the approval on the REST path (ADR-0055).
      const mail = {
        subject,
        body,
        to,
        cc: cc.length ? cc : undefined,
        draft_ref: draftRef ?? undefined,
        consent_purpose: purpose,
        ...scheduleFields(sendAt),
      };
      const { data, error, response } = await sendFrom({
        // The SAME anchor the draft used. Split, these disagree in the worst
        // possible way: the draft answers a thread and writes "Re: …", and the
        // send then takes the account path — which files under the links the
        // body names rather than the anchor's own (the person the mail was
        // with gets none, so the message is missing from their timeline), and
        // starts a new RFC message-id chain, so what the reader was shown as a
        // reply reaches the recipient as an orphan.
        activityId: answering,
        isChannelReply,
        mail,
        channelBody: { body, consent_purpose: purpose },
        links: composedLinks(
          { entityType, entityId },
          grounding,
          projectFiling.projectId,
        ),
      });
      if (response.status === 501) return { sent: false as const };
      // Only a real 202 is a send. openapi-fetch returns a falsy `error` for a
      // bodiless non-2xx (a gateway 502/503/504); inferring success from
      // `!error` would close the modal reporting an irreversible send that
      // never left the building. Gate on the status, not the error body.
      if (!response.ok) {
        throwProblem(error || { title: t("compose.actionFailed") });
      }
      // 201 is a message that will go later; 202 is one that has gone. Both
      // close the composer, and the caller is told which so it can say so.
      return {
        sent: true as const,
        scheduled: response.status === 201,
        activity: data,
      };
    },
    onSuccess: (result) => {
      if (!result.sent) {
        setSendUnavailable(true);
        return;
      }
      for (const queryKey of entityTimelineKeys(entityType, entityId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A scheduled send is the one outcome here the rep may need to UNDO, and
      // until now the composer computed that it had scheduled one and said
      // nothing — it closed the same way a sent message closes it. The confirm
      // dialog they just read promised "you can move it or take it back from
      // Scheduled messages", and nothing in the product went there.
      //
      // The verb makes it sticky, which is what this needs and a plain
      // confirmation does not: the door is the point, and a toast that withdrew
      // itself after three and a half seconds would take the door with it.
      if (result.scheduled) {
        toast.show(t("compose.scheduledQueued"), {
          action: {
            label: t("compose.scheduledOpenQueue"),
            onAct: () => navigate({ screen: SCHEDULED_SCREEN }),
          },
        });
      }
      onSent?.();
      onClose();
    },
  });

  // A refusal is a distinct product state, not a generic failure: it keeps the
  // form open under copy naming the rep's next move, and the raw server detail
  // must not appear alongside it.
  const refusal = refusalOf(send.error);
  const sendError =
    send.isError && refusal === null ? problemMessageOf(send.error, t) : null;
  const missing = missingToSend(isChannelReply, {
    to,
    subject,
    body,
    purpose,
  });
  // Whether the reader has ASKED to send yet. The fields say nothing until
  // then: a form that reports what is missing before anybody has tried is a
  // form scolding a reader for not having finished typing.
  const [attempted, setAttempted] = useState(false);
  // Each opening starts clean — a refusal belongs to the send it answered.
  useEffect(() => {
    if (!open) {
      setAttempted(false);
    }
  }, [open]);
  const flagged = attempted ? new Set(missing) : new Set<MissingField>();
  // Where the first unanswered field is, so a press that cannot send moves the
  // reader to the thing to fix rather than leaving them to hunt for the red
  // text. Read off the DOM at the moment of the press: the three controls are a
  // token field, an input and a select, with no one ref shape between them, and
  // `aria-invalid` is the mark they already share.
  const fields = useRef<HTMLDivElement | null>(null);
  const focusFirstMissing = () => {
    fields.current
      ?.querySelector<HTMLElement>('[aria-invalid="true"]')
      ?.focus();
  };
  // While a rejection is in flight the draft it names is being disposed of, so
  // nothing else on this surface may act on that draft: sending would race the
  // rejection for the signal, and re-drafting would hand the rep words the
  // rejection's clear-down is about to wipe. The drafted text is frozen for the
  // same span — a failed rejection hands its reference back, and a reference
  // may only ever name the words on screen, never words typed while it was
  // away.
  const rejectionInFlight = discard.isPending;
  // The two draft controls, assembled here so the JSX below reads as a layout
  // rather than as a list of conditions.
  //
  // An account-started draft grounds itself on the recipient, so there is
  // nothing to draft until one is chosen: the button is disabled with the
  // picker directly above it, rather than running and coming back with a
  // refusal about a field already on screen.
  const draftControl = {
    // The project the PICKER holds, not the account hook's own copy: one
    // control owns that answer now, and the draft must be grounded in the
    // project the reader can see chosen.
    run: () =>
      draft.mutate({
        grounding: {
          ...account.grounding,
          projectId: projectFiling.projectId,
        },
      }),
    pending: draft.isPending,
    // The write in flight is NOT spelled here: a drafting button keeps its full
    // ink and a turning mark, and only a precondition the reader could meet
    // takes the control away from them.
    disabled: rejectionInFlight || (groundable && !account.recipientId),
    error: draft.isError ? problemMessageOf(draft.error, t) : null,
  };
  // The account-started path's two additions to the mail's head — who this is
  // to, and which deal it is about — built here so the JSX below reads as a
  // layout rather than as a list of conditions. They sit with the other rows
  // that say what the message IS, above the words themselves.
  const accountContext = groundable ? (
    <AccountDraftContext
      orgId={entityId}
      recipientId={account.recipientId}
      onRecipientChange={account.setRecipientId}
      dealId={account.dealId}
      onDealChange={account.setDealId}
    />
  ) : null;
  const discardControl = rejectable
    ? {
        run: () => discard.mutate(rejectable),
        pending: discard.isPending,
        disabled: send.isPending,
        error: discard.isError ? problemMessageOf(discard.error, t) : null,
      }
    : null;
  // A moment picked in the send-later field makes this dialog a different
  // promise. The three sentences it otherwise prints — the title, the confirm
  // button and the body — all say the send is happening NOW and cannot be taken
  // back, and both halves of that are false for a scheduled message: it waits,
  // and it can be moved or withdrawn from the scheduled queue until it goes.
  // Mail-only, because the send-later control is (a channel reply answers a live
  // conversation and has no field to pick a moment in).
  const scheduling = !isChannelReply && sendAt !== "";
  const scheduled = momentOf(sendAt);
  // The person this mail is TO, however the composer came to know them: a
  // person page names it as the record the composer was opened from, and a
  // company draft PICKS one in the account context — which is the flow with the
  // most reason to warn, since the rep is choosing between the account's
  // contacts rather than answering somebody who already wrote. A deal timeline
  // names none, and warns about nothing.
  //
  // The chosen recipient wins where there is one: on an account draft the
  // record the composer was opened from is the organization, and the person is
  // whoever the reader just picked.
  //
  // Nothing at all for a channel reply. MailOnlyFields is not rendered for one
  // — its recipient is resolved server-side and there are no address fields to
  // warn under — so asking would spend a composite read on an answer with
  // nowhere to go.
  const recipientPerson = isChannelReply
    ? undefined
    : ((account.recipientId || undefined) ??
      (entityType === "person" ? entityId : undefined));
  const deadRecipients = useDeadRecipients(recipientPerson, [...to, ...cc]);
  return (
    <>
      <ScheduleDialog
        open={pickingMoment}
        onClose={() => setPickingMoment(false)}
        sendAt={sendAt}
        onChoose={setSendAt}
        now={new Date()}
      />
      <ConfirmModal
        open={open}
        onClose={onClose}
        title={t(
          isChannelReply
            ? "compose.sendMessageConfirmTitle"
            : scheduling
              ? "compose.scheduleConfirmTitle"
              : "compose.sendConfirmTitle",
        )}
        tier="confirm"
        // The rep is about to send irreversibly, so the body they are
        // confirming has to be readable at a glance rather than through a
        // five-line porthole — and the Send button has to sit above the fold,
        // not below a scroll. A reply takes the split width because it carries
        // a second column: the conversation it is answering.
        size={splitColumns ? "split" : "wide"}
        // A DRAWER for every mail on an account, whether it starts a
        // conversation or answers one, so the record it is about stays on
        // screen beside it. It used to turn on `groundable`, which is false as
        // soon as the account has earlier mail — so the same button gave a
        // drawer on a quiet account and a centred box on a busy one, and an
        // account whose first message was still loading changed shape under
        // the reader mid-open.
        placement={asDrawer ? "right" : "center"}
        confirmLabel={t(scheduling ? "compose.schedule" : "compose.send")}
        // The button stays live with fields outstanding. Grey, it refused
        // without saying what for, and the reader was left comparing the form
        // against a control that would not answer. It is disabled only while a
        // rejection is in flight, which is a genuine "not now".
        confirmDisabled={rejectionInFlight}
        onConfirm={() => {
          if (missing.length > 0) {
            setAttempted(true);
            // After the paint that marks them, or there is nothing to find.
            globalThis.requestAnimationFrame(focusFirstMissing);
            return;
          }
          send.mutate(
            groundable
              ? { ...account.grounding, projectId: projectFiling.projectId }
              : null,
          );
        }}
        pending={send.isPending}
        error={sendError}
        actionsLead={
          discardControl && (
            <Button
              onClick={discardControl.run}
              disabled={discardControl.disabled}
              // What discarding DOES, on the control itself: it is not an undo,
              // it tells the voice profile this draft missed. A rep who reads it
              // as "clear the box" would train the model on every draft they
              // merely changed their mind about.
              title={t("compose.discardDraftHint")}
            >
              {t("compose.discardDraft")}
            </Button>
          )
        }
        confirmMenu={
          isChannelReply ? undefined : (
            <ScheduleMenu onOpen={() => setPickingMoment(true)} />
          )
        }
      >
        {/* The conversation on the left, the reply on the right. Wrapped only
          when there IS one: an account-started message has no thread, and a
          lone form inside a two-column grid is a form with an empty half. */}
        <div
          ref={fields}
          className={splitColumns ? "compose-split" : undefined}
        >
          {showChoices && (
            <ConversationChoices
              conversations={recent.conversations}
              pending={recent.pending}
              failed={recent.failed}
              onRetry={recent.retry}
              onChoose={setChosen}
            />
          )}
          {showConversation && (
            <ThreadPane
              messages={conversation.messages}
              pending={conversation.pending || anchorUnresolved}
              failed={conversation.failed}
              onRetry={conversation.retry}
              viewerUserId={viewerId}
              nameOf={nameOf}
              named
              onLeave={chosen ? () => setChosen(undefined) : undefined}
            />
          )}
          <div className="compose-fields">
            {/* Every message says where it files. Mail ASKS — its answer travels
            in the subject tag. A channel reply is TOLD: its send carries no
            filing field, so the conversation's own links are inherited
            whatever a control here collected. The condition is the
            transport's own, not the mail-only branch below. */}
            {isChannelReply ? (
              <ChannelReplyFiling activityId={activityId} />
            ) : (
              <ProjectFiling
                projects={reachableProjects}
                projectId={projectFiling.projectId}
                onChange={projectFiling.setProjectId}
              />
            )}
            {accountContext}
            {/* AI drafting is mail-only — there is no draft-message endpoint, and
            a channel reply's recipient is resolved server-side, so neither
            the draft controls nor the To/Cc/Subject fields apply to it. */}
            {!isChannelReply && (
              <MailOnlyFields
                // The pane IS the answer to "what am I replying to", in the
                // messages themselves. The sentence stays for the composer that
                // has no pane — an account-started message, where what is being
                // continued was decided here and shown nowhere.
                answering={splitColumns ? null : answeringLine}
                flagged={flagged}
                intent={intent}
                onIntentChange={setIntent}
                draft={draftControl}
                draftUnavailable={draftUnavailable}
                provenance={provenance}
                voiceMaturity={voiceProfile.data?.maturity}
                reasons={account.reasoning}
                to={to}
                onToChange={setTo}
                onToEditing={stopOfferingRecipient}
                cc={cc}
                onCcChange={setCc}
                subject={subject}
                onSubjectChange={setSubject}
                rejectionInFlight={rejectionInFlight}
                deadRecipients={deadRecipients}
              />
            )}
            <Textarea
              className="compose-body"
              aria-label={t("compose.body")}
              placeholder={t("compose.body")}
              value={body}
              disabled={rejectionInFlight}
              aria-invalid={flagged.has("body") || undefined}
              onChange={(event) => editBody(event.target.value)}
            />
            <FieldNeed
              show={flagged.has("body")}
              need={t("compose.missingBody")}
            />
            <p className="t-caption">{t("compose.bodyHint")}</p>
            {/* Only over the machine's OWN untouched words: a rewrite replaces the
            body, and once the rep has edited it there is no model draft left
            to rewrite — only their work to throw away. */}
            {body !== "" && body === servedBody && (
              <RewriteRow
                // The draft in flight blocks these too, though it does not block
                // the button that started it: a rewrite REPLACES the body, so a
                // second request begun beside the first lands whichever answer
                // returns last over the reader's editor.
                disabled={draftControl.pending || draftControl.disabled}
                onRewrite={(instruction) =>
                  draft.mutate({
                    grounding: {
                      ...account.grounding,
                      projectId: projectFiling.projectId,
                    },
                    instruction,
                  })
                }
              />
            )}

            <label className="t-body compose-check">
              {t("compose.purpose")}
              <Select
                aria-label={t("compose.purpose")}
                options={purposeOptions(purposes.data?.data)}
                value={purpose}
                aria-invalid={flagged.has("purpose") || undefined}
                onChange={setPurpose}
              />
            </label>
            <FieldNeed
              show={flagged.has("purpose")}
              need={t("compose.missingPurpose")}
            />
            <p className="t-caption">{t("compose.purposeHint")}</p>

            {!isChannelReply && (
              <MailSendNotices to={to} cc={cc} purpose={purpose} />
            )}
            {sendUnavailable && (
              <p className="t-caption">{t("compose.sendUnavailable")}</p>
            )}
            {/* The rejection failed, and the rep has to be told: the judgment is
            still open and the words on screen are still the ones it names.
            Announced rather than merely coloured, on the same terms as every
            other failure in this drawer. */}
            {discardControl?.error && (
              <p
                className="t-caption"
                role="alert"
                style={{ color: "var(--danger)" }}
              >
                {discardControl.error}
              </p>
            )}
            <SendRefusal refusal={refusal} personId={personId} />
            <p className="t-caption">
              {t(
                isChannelReply
                  ? "compose.sendMessageBody"
                  : scheduling
                    ? "compose.scheduleBody"
                    : "compose.sendBody",
              )}
            </p>
            {/* The moment in words, under the control that carries it: a rep who
            picked one three clicks ago has to be able to read it back without
            reopening the picker to find out what they chose. */}
            {scheduled && (
              <p className="t-caption">
                {t("compose.willGoOut", {
                  when: momentLabel(scheduled, locale, zone),
                })}
              </p>
            )}
          </div>
        </div>
      </ConfirmModal>
    </>
  );
}

// WHICH OF THESE ADDRESSES IS KNOWN NOT TO ARRIVE. The person page badges an
// address whose latest delivery hard-bounced with nothing clean since, and the
// composer is where that matters: the mark was visible only on a page the rep
// is not looking at while they write.
//
// Read off the 360 under the SAME key the person page fetches under, so opening
// the composer from that page costs no request at all and the two surfaces
// cannot disagree about which address is dead. A composer that never learned a
// person — a deal timeline has no single one — asks for nothing and warns about
// nothing.
//
// The section carries its own grant. A caller who may not read the send ledger
// gets it omitted rather than empty, and this then marks nothing: an unanswered
// read is not "the address is fine", and a warning invented from an absence
// would be a claim about correspondence the reader may not see.
//
// Free-typed addresses with no person context stay unwarned on purpose (#3160):
// deriving deadness for an arbitrary string needs an endpoint of its own.
function useDeadRecipients(
  personId: string | undefined,
  recipients: readonly string[],
) {
  const view = usePerson360(personId as string, personId != null);
  const dead = view.data?.dead_addresses;
  if (personId == null || dead == null || dead.length === 0) return [];
  // Addresses compare case-insensitively — a rep who types Anna@… must be
  // warned about anna@…, and the ledger stores what the provider reported.
  const marked = new Set(dead.map((address) => address.toLowerCase()));
  // ONE MENTION PER ADDRESS. To and Cc are asked about together, and a rep who
  // has the same address in both would otherwise read it named twice in a
  // sentence about one thing being wrong with it.
  const named = new Map<string, string>();
  for (const address of recipients) {
    const key = address.toLowerCase();
    if (marked.has(key) && !named.has(key)) {
      named.set(key, address);
    }
  }
  return [...named.values()];
}

// A channel reply can only land on a live, unblocked identity, and the
// failure otherwise arrives after the rep has already written the message —
// worse than never offering the box (design §9.3). Reachability is read off
// the person the row's own timeline names: `["person", personId]` is the same
// query key the 360 screen already fetches under, so this rides its cache
// instead of opening a second request. A caller that never learned a personId
// (e.g. a deal timeline, which has no single person to check) gets the
// pre-existing behaviour of always offering the reply — this only ever turns
// the action OFF, never on, for a row it cannot verify.
function useChannelReachable(
  isChannel: boolean,
  personId: string | undefined,
  provider: string | undefined,
) {
  const person = useQuery({
    queryKey: ["person", personId],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}", {
        params: { path: { id: personId as string } },
      });
      if (error) throwProblem(error);
      return data;
    },
    enabled: isChannel && personId != null,
  });
  if (!isChannel || personId == null) return true;
  // Matched against the row's OWN transport. A hardcoded "telegram" here would
  // withhold the reply on every other transport's rows and offer it on a
  // Telegram-reachable person's rows whatever carried the conversation — the
  // kind stopped naming the transport at ADR-0107/A158, so the row has to say.
  return (person.data?.reachability ?? []).some(
    (channel) => channel.provider === provider && channel.reachable,
  );
}

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
// withheld when the person behind it cannot be reached on the transport that
// carried it, and a rep offered a reply on one surface and refused it on the
// other would have no way to tell which answer was true.
export function ChannelReplyAction({
  activityId,
  kind,
  channelProvider,
  entityType,
  entityId,
  personId,
}: Readonly<{
  activityId: string;
  kind: Activity["kind"];
  channelProvider?: string;
  entityType: RelinkKind;
  entityId: string;
  personId?: string;
}>) {
  const t = useT();
  const [reply, setReply] = useState(false);
  const reachable = useChannelReachable(
    kind === "message",
    personId,
    channelProvider,
  );
  if (!reachable) {
    return null;
  }
  return (
    <>
      <Button small onClick={() => setReply(true)}>
        {t("compose.reply")}
      </Button>
      {reply && (
        <ComposeModal
          activityId={activityId}
          entityType={entityType}
          entityId={entityId}
          personId={personId}
          kind={kind}
          open={reply}
          onClose={() => setReply(false)}
        />
      )}
    </>
  );
}
