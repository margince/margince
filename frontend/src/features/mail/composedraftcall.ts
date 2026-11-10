// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { ProjectScope } from "../../design-system/projectpicker";
import type { useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { throwProblem } from "../../lib/problem";
import { stripEveryKeyTag, withSubjectTag } from "../projects/projectrecord";
import type { RelinkKind } from "./composerelink";

type EmailDraft = components["schemas"]["EmailDraft"];
type DraftReason = components["schemas"]["AccountDraftReason"];
type Translate = ReturnType<typeof useT>;

// What a drafting call reported about the text it produced. It is kept apart
// from the fields it filled, because the disclosure is owed for the call.
export type DraftProvenance = Pick<
  EmailDraft,
  | "ai_generated"
  | "ai_disclosure"
  | "voice_profile_version"
  | "voice_degraded"
  | "language"
>;

// Why a draft is not on offer. "no_model" is the deployment's answer, while
// "unsupported_origin" means the rep can still draft from the account.
export type DraftUnavailable = "no_model" | "unsupported_origin";

// The three choices a rep made on the account-started path. It travels as the
// mutation variable of the draft and the send, so a stale closure cannot
// change it (see census/rules/mutation-variable-coverage.census.ts).
export type Grounding = {
  recipientId: string;
  dealId: string;
  projectId: string;
};

// A draft route's 2xx body, as the fill reads it. Only the account draft
// answers `reasoning` and `scope`, so both stay optional here.
type DraftedPayload = Extract<DraftResult, { available: true }>["draft"] & {
  reasoning?: DraftReason[];
  scope?: ProjectScope;
};

// What any drafting path answers. It names the fields the fill reads, so a
// missing required field still fails the type check.
export type DraftResult =
  | { available: false; reason: DraftUnavailable }
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
            | "voice_degraded"
            | "language"
          >
        >;
      // Account path only: a reply explains itself by the message it answers.
      reasoning?: DraftReason[];
      // Account path only: what the scoped read kept, when a project was chosen.
      scope?: ProjectScope;
      // A model was sent the body on screen and answered a revision of it. The
      // deterministic floor never reads that body, so its answer is no revision.
      revised?: boolean;
    };

// What one drafting call is asked under. A rewrite carries its instruction
// for that call alone, so the steer field keeps the rep's own words.
export type DraftAsk = Readonly<{
  grounding: Grounding;
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  // A contact-started draft's further recipients and the record it follows up.
  followUp?: FollowUp;
  intent: string;
  epoch: number;
  subject?: string;
  body: string;
  html: string;
  instruction?: string;
  // The language the rep chose, named on every ask, since the server keeps none.
  language?: Locale;
  // The answer replaces the body on screen: a rewrite verb or a new language.
  rewrite?: boolean;
}>;

export function fillFromDraft(
  result: Extract<DraftResult, { available: true }>,
  form: Readonly<{
    subject: string;
    body: string;
    toEmpty: boolean;
    setSubject: (next: string) => void;
    setBody: (next: string) => void;
    // The words as the model served them. A rewrite is offered only while
    // the body still equals them, because an edited body is the rep's work.
    setServedBody: (next: string) => void;
    // A rewrite, or a draft the route wrote from the body on screen, replaces
    // that body. Any other draft fills an empty field and leaves it alone.
    rewrite?: boolean;
    setTo: (next: string[]) => void;
    setDraftRef: (next: string | null) => void;
    setProvenance: (next: DraftProvenance) => void;
    setReasoning: (next: DraftReason[]) => void;
    setScope: (next: ProjectScope | undefined) => void;
  }>,
): boolean {
  const drafted = result.draft;
  // A subject holding only the project tag has no words of the rep's yet, so
  // the drafted subject fills in behind the tag.
  const written = stripEveryKeyTag(form.subject).trim();
  if (!written) {
    form.setSubject(
      form.subject.trim()
        ? withSubjectTag(drafted.subject, form.subject.trim())
        : drafted.subject,
    );
  }
  const replacesBody = !form.body || form.rewrite === true;
  if (replacesBody) {
    form.setBody(drafted.body);
    form.setServedBody(drafted.body);
    form.setDraftRef(drafted.draft_ref ?? null);
    form.setProvenance({
      // An absent flag reads as false: it may neither hide a disclosure nor
      // claim a model wrote untouched text.
      ai_generated: drafted.ai_generated ?? false,
      ai_disclosure: drafted.ai_disclosure,
      voice_profile_version: drafted.voice_profile_version,
      voice_degraded: drafted.voice_degraded ?? false,
      language: drafted.language,
    });
    form.setReasoning(result.reasoning ?? []);
    form.setScope(result.scope);
  }
  if (form.toEmpty && drafted.to?.length) {
    form.setTo(drafted.to);
  }
  return replacesBody;
}

// What survives clearing a subject: the project tag. The composer put it
// there to route the reply, so a rejected draft does not take it along.
export function keptTag(subject: string): string {
  const words = stripEveryKeyTag(subject);
  return subject.slice(0, subject.length - words.length).trim();
}

// The reply-side draft answers the message it is anchored to.
async function draftFromActivity({
  activityId,
  steer,
  t,
}: Readonly<{
  activityId: string;
  steer: Steer;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST(
    "/activities/{id}/draft-email",
    {
      params: { path: { id: activityId } },
      body: steering(steer),
    },
  );
  return draftAnswer(response, error, data, t, steer.rewriteOf);
}

// A 2xx is read as a draft only when it has the two fields the contract
// requires. Any other body (a proxy's page, another server version) fails the
// ask rather than writing `undefined` into the subject.
function isDrafted(data: DraftedPayload | undefined): data is DraftedPayload {
  return typeof data?.subject === "string" && typeof data.body === "string";
}

// What a draft route makes of its answer. A 501 says the stack has no model
// lane. Success needs a 2xx with a body, because openapi-fetch reports a falsy
// `error` for a bodiless gateway 5xx.
function draftAnswer(
  response: Response,
  error: unknown,
  data: DraftedPayload | undefined,
  t: Translate,
  rewriteOf = "",
): DraftResult {
  if (response.status === 501) {
    return { available: false as const, reason: "no_model" as const };
  }
  if (!response.ok || !isDrafted(data)) {
    throwProblem(error || { title: t("compose.actionFailed") });
  }
  return {
    available: true as const,
    draft: data,
    reasoning: data.reasoning,
    scope: data.scope,
    revised: rewriteOf.trim() !== "" && data.ai_generated === true,
  };
}

// The caller's steering. An absent `rewrite_of` asks for a first draft, so a
// blank one is left out.
export function draftingSubject(subject: string | undefined): string {
  return stripEveryKeyTag(subject ?? "").trim();
}

// What every draft route reads the same way. `rewriteOf` is the body on
// screen, so "make it shorter" revises this draft.
type Steer = Readonly<{
  intent: string;
  rewriteOf: string;
  subject?: string;
  language?: Locale;
}>;

function steering(steer: Steer) {
  const { intent, rewriteOf, language } = steer;
  const written = draftingSubject(steer.subject);
  return {
    ...(intent.trim() ? { intent: intent.trim() } : {}),
    ...(written ? { subject: written } : {}),
    ...(rewriteOf.trim() ? { rewrite_of: rewriteOf.trim() } : {}),
    ...(language ? { language } : {}),
  };
}

// The contract's cap on `rewrite_of`, in characters. A longer body goes out as
// a first draft, so the route still answers and the fill keeps the body.
const REWRITE_OF_MAX = 20_000;

// The words a draft call is asked to revise: the body on screen, whoever wrote
// it. Not a body the rep formatted: a plain-text revision would drop the links.
function rewriteOfAsk(ask: DraftAsk): string {
  const overCap = [...ask.body.trim()].length > REWRITE_OF_MAX;
  return hasRepFormatting(ask.html) || overCap ? "" : ask.body;
}

// The model's draft is paragraphs and line breaks only. Any other tag is
// formatting or a link the rep added, and a plain-text rewrite would drop it.
function hasRepFormatting(html: string): boolean {
  return /<(?!\/?(?:p|br)\b)[a-z]/i.test(html);
}

/** Whether the body is still the model's served words, with nothing the rep added. */
export function servedUntouched(
  body: Readonly<{ text: string; html: string; served: string }>,
): boolean {
  return body.text === body.served && !hasRepFormatting(body.html);
}

// A follow-up after a meeting: everyone else it was with, and the meeting
// itself, which the server reads first.
export type FollowUp = Readonly<{
  alsoTo: readonly string[];
  aboutActivityId?: string;
}>;

function followUpBody(followUp: FollowUp | undefined) {
  return {
    ...(followUp?.alsoTo.length ? { also_to: [...followUp.alsoTo] } : {}),
    ...(followUp?.aboutActivityId
      ? { about_activity_id: followUp.aboutActivityId }
      : {}),
  };
}

// The lead-started draft. A lead carries its own address, so there is no
// recipient to name and no deal or project to pick.
async function draftFromLead({
  entityId,
  steer,
  t,
}: Readonly<{
  entityId: string;
  steer: Steer;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST("/leads/{id}/draft-email", {
    params: { path: { id: entityId } },
    body: steering(steer),
  });
  return draftAnswer(response, error, data, t, steer.rewriteOf);
}

// The contact-started draft. The record in the path is the recipient, and the
// chosen project scopes the grounding read.
async function draftFromContact({
  entityId,
  followUp,
  projectId,
  steer,
  t,
}: Readonly<{
  entityId: string;
  followUp?: FollowUp;
  projectId: string;
  steer: Steer;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST(
    "/contacts/{id}/draft-email",
    {
      params: { path: { id: entityId } },
      body: {
        ...followUpBody(followUp),
        ...(projectId ? { project_id: projectId } : {}),
        ...steering(steer),
      },
    },
  );
  return draftAnswer(response, error, data, t, steer.rewriteOf);
}

// The account-started draft. It grounds itself in the record it was started
// from, so a company needs the recipient named first.
async function draftFromAccount({
  entityType,
  entityId,
  followUp,
  recipientId,
  dealId,
  projectId,
  steer,
  t,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  followUp?: FollowUp;
  recipientId: string;
  dealId: string;
  projectId: string;
  steer: Steer;
  t: Translate;
}>): Promise<DraftResult> {
  if (entityType === "lead") {
    return draftFromLead({ entityId, steer, t });
  }
  if (entityType === "contact") {
    return draftFromContact({ entityId, followUp, projectId, steer, t });
  }
  // A deal has no draft route: writing to a contact of a nearby account would
  // start a conversation the rep never chose.
  if (entityType !== "company" || !recipientId) {
    return { available: false as const, reason: "unsupported_origin" as const };
  }
  const { data, error, response } = await api.POST(
    "/companies/{id}/draft-email",
    {
      params: { path: { id: entityId } },
      body: {
        contact_id: recipientId,
        ...(dealId ? { deal_id: dealId } : {}),
        // The server scopes the grounding read to this project.
        ...(projectId ? { project_id: projectId } : {}),
        ...steering(steer),
      },
    },
  );
  return draftAnswer(response, error, data, t, steer.rewriteOf);
}

// The drafting call for every origin. They answer one shape, so the fill
// cannot tell them apart and they share one clobber rule.
export function useDraftMutation({
  entityId,
  onUnavailable,
  onDrafted,
  resetUnavailable,
  isCurrent,
  onSkipped,
  t,
}: Readonly<{
  entityId: string;
  onUnavailable: (reason: DraftUnavailable) => void;
  onDrafted: (
    result: Extract<DraftResult, { available: true }>,
    ask: DraftAsk,
  ) => void;
  resetUnavailable: () => void;
  isCurrent: (ask: DraftAsk) => boolean;
  onSkipped: (ask: DraftAsk) => void;
  t: Translate;
}>) {
  return useMutation({
    mutationKey: ["email-draft", entityId],
    mutationFn: async (ask: DraftAsk): Promise<DraftResult> => {
      resetUnavailable();
      const { grounding, activityId, entityType } = ask;
      const steer = {
        intent: ask.instruction ?? ask.intent,
        // The body on screen goes with every ask, so words the rep wrote are
        // revised rather than ignored. A blank body asks for a first draft.
        rewriteOf: rewriteOfAsk(ask),
        language: ask.language,
      };
      if (activityId) {
        return draftFromActivity({ activityId, steer, t });
      }
      return draftFromAccount({
        entityType,
        entityId: ask.entityId,
        followUp: ask.followUp,
        ...grounding,
        steer: { ...steer, subject: ask.subject },
        t,
      });
    },
    onSuccess: (result, ask) => {
      if (!isCurrent(ask)) {
        onSkipped(ask);
        return;
      }
      if (!result.available) {
        onUnavailable(result.reason);
        return;
      }
      onDrafted(result, ask);
    },
  });
}
