// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { ProjectScope } from "../design-system/projectpicker";
import type { useT } from "../i18n";
import { throwProblem } from "./common";
import type { RelinkKind } from "./composerelink";
import { stripEveryKeyTag, withSubjectTag } from "./projectrecord";

type EmailDraft = components["schemas"]["EmailDraft"];
type DraftReason = components["schemas"]["AccountDraftReason"];
type Translate = ReturnType<typeof useT>;

// What a drafting call reported about the text it produced. It is kept apart
// from the fields it filled, because the disclosure is owed for the call.
export type DraftProvenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version" | "voice_degraded"
>;

// Why a draft is not on offer. "no_model" is the deployment's answer, while
// "unsupported_origin" means the rep can still draft from the account.
export type DraftUnavailable = "no_model" | "unsupported_origin";

// The three choices a rep made on the account-started path. It travels as the
// mutation variable of the draft and the send, so a stale closure cannot
// change it (see mutation-variable-coverage.test.ts).
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
          >
        >;
      // Account path only: a reply explains itself by the message it answers.
      reasoning?: DraftReason[];
      // Account path only: what the scoped read kept, when a project was chosen.
      scope?: ProjectScope;
    };

// What one drafting call is asked under. A rewrite carries its instruction
// for that call alone, so the steer field keeps the rep's own words.
export type DraftAsk = Readonly<{
  grounding: Grounding;
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  intent: string;
  epoch: number;
  body: string;
  html: string;
  instruction?: string;
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
    // A rewrite replaces the body it was asked about. Any other draft fills
    // an empty field and leaves the rep's words alone.
    rewrite?: boolean;
    setTo: (next: string[]) => void;
    setDraftRef: (next: string | null) => void;
    setProvenance: (next: DraftProvenance) => void;
    setReasoning: (next: DraftReason[]) => void;
    setScope: (next: ProjectScope | undefined) => void;
  }>,
) {
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
  if (!form.body || form.rewrite) {
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
    });
    form.setReasoning(result.reasoning ?? []);
    form.setScope(result.scope);
  }
  if (form.toEmpty && drafted.to?.length) {
    form.setTo(drafted.to);
  }
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
  intent,
  t,
}: Readonly<{
  activityId: string;
  intent: string;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST(
    "/activities/{id}/draft-email",
    {
      params: { path: { id: activityId } },
      body: intent.trim() ? { intent: intent.trim() } : {},
    },
  );
  if (response.status === 501) {
    return { available: false as const, reason: "no_model" as const };
  }
  // openapi-fetch reports a falsy `error` for a bodiless gateway 5xx, so
  // success needs a 2xx with a body.
  if (!response.ok || !data) {
    throwProblem(error || { title: t("compose.actionFailed") });
  }
  return { available: true as const, draft: data };
}

// What a draft route makes of its answer. A 501 says the stack has no model
// lane. Success needs a 2xx with a body, because openapi-fetch reports a falsy
// `error` for a bodiless gateway 5xx.
function draftAnswer(
  response: Response,
  error: unknown,
  data: DraftedPayload | undefined,
  t: Translate,
): DraftResult {
  if (response.status === 501) {
    return { available: false as const, reason: "no_model" as const };
  }
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

// The caller's steering. An absent `rewrite_of` asks for a first draft, so a
// blank one is left out.
function steering(intent: string, rewriteOf: string) {
  return {
    ...(intent.trim() ? { intent: intent.trim() } : {}),
    ...(rewriteOf.trim() ? { rewrite_of: rewriteOf.trim() } : {}),
  };
}

// The lead-started draft. A lead carries its own address, so there is no
// recipient to name and no deal or project to pick.
async function draftFromLead({
  entityId,
  intent,
  t,
}: Readonly<{
  entityId: string;
  intent: string;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST("/leads/{id}/draft-email", {
    params: { path: { id: entityId } },
    body: intent.trim() ? { intent: intent.trim() } : {},
  });
  return draftAnswer(response, error, data, t);
}

// The contact-started draft. The record in the path is the recipient, and the
// chosen project scopes the grounding read.
async function draftFromContact({
  entityId,
  projectId,
  intent,
  rewriteOf,
  t,
}: Readonly<{
  entityId: string;
  projectId: string;
  intent: string;
  rewriteOf: string;
  t: Translate;
}>): Promise<DraftResult> {
  const { data, error, response } = await api.POST(
    "/contacts/{id}/draft-email",
    {
      params: { path: { id: entityId } },
      body: {
        ...(projectId ? { project_id: projectId } : {}),
        ...steering(intent, rewriteOf),
      },
    },
  );
  return draftAnswer(response, error, data, t);
}

// The account-started draft. It grounds itself in the record it was started
// from, so a company needs the recipient named first.
async function draftFromAccount({
  entityType,
  entityId,
  recipientId,
  dealId,
  projectId,
  intent,
  rewriteOf,
  t,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  recipientId: string;
  dealId: string;
  projectId: string;
  intent: string;
  // The body on screen, so "make it shorter" revises this draft.
  rewriteOf: string;
  t: Translate;
}>): Promise<DraftResult> {
  if (entityType === "lead") {
    return draftFromLead({ entityId, intent, t });
  }
  if (entityType === "contact") {
    return draftFromContact({ entityId, projectId, intent, rewriteOf, t });
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
        ...steering(intent, rewriteOf),
      },
    },
  );
  return draftAnswer(response, error, data, t);
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
      const intentOf = ask.instruction ?? ask.intent;
      // A rewrite sends the body on screen; a first draft sends none.
      const rewriteOf = ask.instruction ? ask.body : "";
      if (activityId) {
        return draftFromActivity({ activityId, intent: intentOf, t });
      }
      return draftFromAccount({
        entityType,
        entityId: ask.entityId,
        ...grounding,
        intent: intentOf,
        rewriteOf,
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
