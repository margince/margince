import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { sectionState } from "../../design-system/surfacestate";
import { activityDay } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  EvidenceSources,
  FoundMove,
  fromCitations,
  type OpenReceipt,
} from "../records/record360";
import type { Company360 } from "./company360";
import {
  type DeferOf,
  DismissFailed,
  useCompanySetAside,
} from "./companysetaside";
import "./company360.css";

type Suggestion = components["schemas"]["Company360Suggestion"];

// The body an `add_task` suggestion carries, and the body POST /tasks takes.
// One type, so a step the server prepared cannot be posted as something else.
type CreateTaskRequest = components["schemas"]["CreateTaskRequest"];

export type SuggestionAction = NonNullable<Suggestion["action"]>;

const SUGGESTION_ACTION_LABELS: Record<SuggestionAction["kind"], MessageKey> = {
  draft_reply: "co.suggest.act.draftReply",
  open_deal: "co.suggest.act.openDeal",
  add_task: "co.suggest.act.addTask",
};

const SUGGESTION_ACTION_VARIANT: Record<
  SuggestionAction["kind"],
  "ai" | "primary" | "ghost"
> = {
  draft_reply: "ai",
  add_task: "primary",
  open_deal: "ghost",
};

/**
 * The names this page already holds, for citations the server could not name.
 *
 * The writer names a record when it had the name at hand; nothing invents one.
 * But an account's own 360 holds its contacts and its deals. Printing "contact"
 * beside a reason, while the roster below names that contact, is the page
 * failing to read itself. Only records this view carries are named; anything
 * else answers undefined and falls back to the kind.
 */
export function recordNamesIn(view?: Company360) {
  const names = new Map<string, string>();
  for (const contact of view?.contacts?.data ?? []) {
    names.set(`contact:${contact.contact_id}`, contact.full_name);
  }
  for (const deal of view?.deals?.data ?? []) {
    names.set(`deal:${deal.deal_id}`, deal.name);
  }
  const company = view?.company;
  if (company) {
    names.set(`company:${company.id}`, company.display_name);
  }
  return (entityType: string, entityId: string) =>
    names.get(`${entityType}:${entityId}`);
}

// SuggestionActionButton exists so the action is narrowed once, at the call
// site, rather than re-narrowed inside a callback where TypeScript has already
// lost it.
function SuggestionActionButton({
  action,
  pending,
  onPerform,
}: Readonly<{
  action: SuggestionAction;
  // Whether this row's own write is in flight. Only this row's: one task being
  // written must not freeze the reader's other choices.
  pending?: boolean;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const t = useT();
  // Indigo says the press makes the model write, and only the reply does: its
  // composer drafts on open. Adding the prepared task accepts a rule's
  // sentence, and opening a deal is navigation.
  const byMargince = action.kind === "draft_reply";
  return (
    <Button
      variant={SUGGESTION_ACTION_VARIANT[action.kind]}
      pending={pending}
      onClick={() => onPerform(action)}
    >
      {byMargince && <Sparkles aria-hidden="true" />}
      {t(SUGGESTION_ACTION_LABELS[action.kind])}
    </Button>
  );
}

/**
 * Whether a row can offer this action as a button.
 *
 * `add_task` is the section's own verb: the server prepared the body, so the
 * row writes it and needs no caller. It still refuses to draw without that
 * body. The contract promises one on every `add_task`, and a button that
 * posted nothing would be a control that does nothing.
 *
 * The other two open a surface above this section (the composer, the deal
 * page), so without a caller to open it there is no button.
 */
function performable(
  action: SuggestionAction,
  onPerform?: (action: SuggestionAction) => void,
): boolean {
  return action.kind === "add_task" ? Boolean(action.task) : Boolean(onPerform);
}

// nextCommitmentLine is the daily brief's own footer reading: what is owed
// and how soon. It is not a suggestion: nobody proposed it, the open tasks
// section has one. So it sits in the footer rather than as a row. Exported so
// the brief (companytoday.reading.tsx) reads the same truncation rule, not a copy.
export function nextCommitmentLine(
  view: Company360 | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): { headline: string; overdue: boolean } | undefined {
  const steps = view?.next_steps?.data ?? [];
  const step = steps[0];
  if (!step) {
    return undefined;
  }
  // The section is a page of 25 with `has_more` beside it, so past the cap
  // the count is a claim about the page. "12 overdue" on an account with 40
  // is the kind of small wrong figure a rep plans against.
  const truncated = view?.next_steps?.page?.has_more === true;
  const overdueCount = steps.filter((each) => each.overdue).length;
  const count = overdueCount > 0 ? overdueCount : steps.length;
  const key = overdueCount > 0 ? "overdue" : "open";
  return {
    headline: truncated
      ? t(`co.suggest.commitment.${key}AtLeast`, {
          count: formatNumber(count, locale),
        })
      : t(`co.suggest.commitment.${key}Count`, {
          count: formatNumber(count, locale),
        }),
    overdue: overdueCount > 0,
  };
}

// useSuggestionsBody is the advice section's data and rows: the dismiss
// mutation and the "move" rows, which companytoday.reading.tsx composes into the brief.
// Each row leads with the reason the rule fired, so a rep can disagree with it.
// A dismissal is keyed on the evidence: the advice stays gone while that holds.
export function useSuggestionsBody({
  companyId,
  view,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPerform,
  advice,
  keep,
}: Readonly<{
  companyId: string;
  view?: Company360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt | undefined;
  // Opens a cited message in the page's email drawer. A rule that fired on an
  // unanswered mail names that mail as its grounds, and the reader's next act
  // is to read it.
  onOpenEmail?: (activityId: string) => void;
  // Opening a surface is the page's job, not this section's: the composer and
  // the deal page both live above it. Writing the prepared step is this
  // section's own verb and needs no caller (see `performable`).
  onPerform?: (action: SuggestionAction) => void;
  // The merged advice (the rules' rows and the scan's findings as one list),
  // when the page holds a scan. It replaces the 360's own rows: the server
  // merged, deduplicated and capped once. A second list here would be a second
  // answer to "what needs a contact".
  advice?: { findings: Suggestion[]; dropped: number };
  // Which advice this caller draws; absent, all of it (the advice card). The
  // Tasks tab passes a predicate because it shows the steps, not the moves. A
  // tab called Tasks listing a stalled deal would be the advice card again
  // under a heading that says otherwise.
  keep?: (suggestion: Suggestion) => boolean;
}>): {
  // Whether the section has rows worth showing. A withheld, empty or
  // unavailable suggestion block carries none. Advice is additive, and
  // "no advice" or "we cannot advise you" are not things a rep acts on.
  ready: boolean;
  rows: ReactNode;
  // How many rows `rows` draws: one node carries several, so a caller weighing
  // what the list holds cannot count them for itself.
  count: number;
  // Whether any row this section draws offers to answer a specific message.
  //
  // Computed from the post-filter list, not the raw advice. A caller skips its
  // generic "write to them" row only when the reader can see the specific one.
  // Otherwise a `keep` that filtered the reply out would leave neither offered.
  hasDraftReply: boolean;
  // The truncation count and a failed dismissal, additive on top of whatever
  // else the caller's own footer carries.
  footer?: ReactNode;
  // The "Not now" of these rows, which the account's moment shares.
  deferOf: DeferOf;
} {
  const { locale } = useLocale();
  const t = useT();
  const recordZone = useRecordZone();
  const client = useQueryClient();
  const notNow = useCompanySetAside(companyId);
  const write = useMutation({
    // The body is the server's, passed as a variable. The click posts the step
    // the row was drawn from, never one recomposed here from the row's words.
    mutationFn: async (body: CreateTaskRequest) => {
      unwrap(await api.POST("/tasks", { body }), t);
    },
    // Three reads change. The 360 decides whether the advice still stands,
    // since it fired on there being no open task. The task lists elsewhere
    // gain the row this wrote.
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["company360", companyId] });
      client.invalidateQueries({ queryKey: ["activities"] });
      client.invalidateQueries({ queryKey: ["tasks"] });
    },
  });
  // Performing the advice. `add_task` is this section's own verb and is written
  // here; the rest opens a surface only the caller can reach.
  const perform = (action: SuggestionAction) => {
    if (action.kind !== "add_task") {
      onPerform?.(action);
      return;
    }
    if (action.task) {
      write.mutate(action.task);
    }
  };

  const all: Suggestion[] = advice?.findings ?? view?.suggestions ?? [];
  const nameOf = recordNamesIn(view);
  const dropped = advice ? advice.dropped : view?.suggestions_dropped;
  const state = sectionState(
    view,
    "suggestions",
    Boolean(advice ?? view?.suggestions),
    all.length,
  );
  // The state is read off the whole section, because that is what the grant
  // and the omission are about. `keep` then narrows what this caller draws. A
  // caller left with nothing shows nothing, as an empty section does.
  const suggestions = keep ? all.filter(keep) : all;
  if (state !== "ready" || suggestions.length === 0) {
    const failure = notNow.failure;
    const footer = failure ? <DismissFailed failure={failure} /> : undefined;
    const { deferOf } = notNow;
    return {
      ready: false,
      rows: null,
      count: 0,
      hasDraftReply: false,
      deferOf,
      footer,
    };
  }
  // How many the cap dropped, said once on the page. The finds list says it,
  // being the list left when the proposed steps are set aside. The proposed steps
  // report none. "2 more" under them would count advice that panel was never
  // going to draw.
  const notShown = keep === proposesAStep ? 0 : (dropped ?? 0);
  const footer =
    notShown > 0 || notNow.failure || write.isError ? (
      <>
        {/* A truncated list with no count reads as "that is everything".
            Absent means the section was never computed, which this card
            does not render at all. */}
        {notShown > 0 && (
          <p className="co-row-meta t-caption">
            {t("co.suggest.more", { count: formatNumber(notShown, locale) })}
          </p>
        )}
        {notNow.failure && <DismissFailed failure={notNow.failure} />}
        {/* Same rule for the write, and it matters more: a rep who thinks the
            step was written stops looking for it. */}
        {write.isError && (
          <ErrorLine>
            {`${t("co.suggest.addTaskFailed")} ${problemMessageOf(write.error, t)}`}
          </ErrorLine>
        )}
      </>
    ) : undefined;
  const rows = suggestions.map((suggestion) => (
    <FoundMove
      key={suggestion.fingerprint}
      // The day the reading behind the row is dated. Never a deadline the
      // system chose.
      when={
        suggestion.due_at
          ? activityDay(
              suggestion.due_at,
              suggestion.due_precision ?? "instant",
              locale,
              recordZone,
            )
          : undefined
      }
      // The ask: what the rule wants done. Falls back to the kind only when the
      // rule named no title of its own.
      title={suggestion.title ?? t(`co.suggest.kind.${suggestion.kind}`)}
      // The why, and behind it the records the rule fired on.
      why={suggestion.reason}
      // The why's grounds, as rows rather than chips. Its basis is the message
      // the reader acts on, so the message gets its subject, its sender and its
      // preview instead of a kind word.
      basis={
        <EvidenceSources
          sources={fromCitations(suggestion.evidence)}
          nameOf={nameOf}
          onOpenRecord={onOpenRecord}
          onOpenReceipt={onOpenReceipt}
          onOpenEmail={onOpenEmail}
        />
      }
      // What performing the advice means, named by the server. A rule that
      // could not name one carries null and this renders nothing rather than
      // a control that does nothing.
      action={
        suggestion.action && performable(suggestion.action, onPerform) ? (
          <SuggestionActionButton
            action={suggestion.action}
            pending={
              write.isPending && write.variables === suggestion.action.task
            }
            onPerform={perform}
          />
        ) : undefined
      }
      // Only the row in flight is disabled: one dismissal must not freeze the
      // rep's other choices.
      defer={notNow.deferOf(suggestion.fingerprint)}
    />
  ));
  return {
    ready: true,
    rows,
    deferOf: notNow.deferOf,
    count: suggestions.length,
    hasDraftReply: suggestions.some(
      (suggestion) => suggestion.action?.kind === "draft_reply",
    ),
    footer,
  };
}

// A suggestion that would become a task. The action decides it, not the kind.
// A rule that names a prepared step offers one whatever it fired on. Reading
// the kind here would leave the next such rule out.
export function proposesAStep(suggestion: Suggestion): boolean {
  return suggestion.action?.kind === "add_task";
}

/**
 * ProposedNextSteps is the advice that would become a task, drawn where the
 * tasks are.
 *
 * A recommended step nobody has accepted yet is not on the task list. Without
 * this, a reader who opens Tasks to see what happens next finds nothing
 * scheduled, though a step was worked out two cards away.
 *
 * Only the advice that names a step. A stalled deal and an unanswered mail are
 * moves too, and neither is a task. Drawing them here would copy the advice
 * card under a heading that says otherwise.
 */
export function ProposedNextSteps(
  props: Readonly<{
    companyId: string;
    view?: Company360;
    onOpenRecord?: (entityType: string, entityId: string) => void;
    onOpenReceipt: OpenReceipt;
    onOpenEmail?: (activityId: string) => void;
  }>,
) {
  const t = useT();
  const body = useSuggestionsBody({ ...props, keep: proposesAStep });
  if (!body.ready) {
    return null;
  }
  return (
    <Panel title={t("co.next.proposed")}>
      {body.rows}
      {body.footer && <PanelBody>{body.footer}</PanelBody>}
    </Panel>
  );
}
