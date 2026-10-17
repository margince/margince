import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { sectionState } from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import "./company360.css";
import { EntityRef } from "../../components/entityref";
import { TaskCompleteCheck, type useTaskUpdate } from "../worklist/taskactions";

// The company view's data layer and its right-rail cards.
//
// One read (GET /companies/{id}/360) serves the whole page, and its
// `sections_omitted` is the thing that makes the page honest: a section the
// caller's role cannot read is ABSENT from the payload and named there, so
// every card below can say "hidden from you" instead of drawing an empty
// list that reads as "there is none".

export { AskSection } from "./company360.ask";
export { DealsCard } from "./company360.deals";
export {
  ENGAGEMENT_LABELS,
  ENGAGEMENT_TONE,
  medianDaysLabel,
  StateStrip,
} from "./company360.strip";
export {
  nextCommitmentLine,
  ProposedNextSteps,
  recordNamesIn,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360.suggestions";

export type Company360 = components["schemas"]["Company360"];

type NextStep = components["schemas"]["Company360NextStep"];

/**
 * useCompany360 reads the whole company page in one round trip.
 *
 * `enabled` lets a caller hold the hook while it asks for nothing. The page
 * does so once the row says archived, because the server refuses that 360.
 * Chrome with no record under it does too: an empty id is a 422.
 */
export function useCompany360(id: string, enabled = true) {
  return useQuery<Company360>({
    queryKey: ["company360", id],
    enabled: enabled && id !== "",
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies/{id}/360", {
          params: { path: { id } },
        }),
      );
    },
  });
}

// VIEW_ACK_DWELL_MS is how long the account must stay open before the visit
// counts. Opening a record and bouncing straight back out is not reading it,
// and an ack from that would mark unread activity as seen.
const VIEW_ACK_DWELL_MS = 5_000;

/**
 * useAcknowledgeCompanyView advances THIS reader's "last seen" baseline
 * for the account — the thing that makes "N new since your last visit" mean
 * anything on the next visit. Without it the server keeps answering with no
 * baseline at all, so every visit reads as the first one.
 *
 * The 360 deliberately does not advance the baseline itself (a prefetch must
 * not be indistinguishable from a visit), so this is the only caller. Leaving
 * before the dwell elapses cancels the timer: the baseline moves only for a
 * visit that actually happened, and when in doubt it stays where it is —
 * showing an item twice is a smaller wrong than hiding one.
 *
 * Success does NOT invalidate the 360. The "new since your last visit" line
 * describes the visit in progress; refetching it out from under the reader
 * would erase the very thing they opened the page to see.
 */
export function useAcknowledgeCompanyView(id: string, visited: boolean) {
  const ack = useMutation({
    mutationFn: async (companyId: string) => {
      unwrap(
        await api.POST("/companies/{id}/view-ack", {
          params: { path: { id: companyId } },
        }),
      );
    },
  });
  // The mutation's own error state holds a failure; nothing renders it. A
  // baseline that did not move costs the reader one repeated line next time,
  // which is not worth an error banner over the account they came to read.
  const fire = ack.mutate;
  useEffect(() => {
    if (!visited) {
      return;
    }
    const timer = window.setTimeout(() => fire(id), VIEW_ACK_DWELL_MS);
    return () => window.clearTimeout(timer);
  }, [id, visited, fire]);
}

/**
 * NextSteps is the middle column's first block: the open tasks on this
 * account, overdue first, each showing what it is linked to.
 *
 * The tick is `update`'s own verb (`TaskCompleteCheck`), not `renderAction`'s
 * — a row names its primary move as the row, not as one more item in a menu.
 * `renderAction` is left for whatever ELSE a caller wants beside the tick
 * (snooze), and stays hidden until the row is hovered or focused, since a
 * list of open tasks reads by title and due date first and only reveals its
 * verbs on approach.
 */
export function NextSteps({
  view,
  proposed,
  renderAction,
  onOpenTask,
  update,
}: Readonly<{
  view: Company360;
  // The steps nobody has accepted yet, drawn above the ones on the list. They
  // are the caller's rows because only a caller that can WRITE one should offer
  // it — a read-only account draws none — and they lead rather than follow: a
  // reader whose account has no open task at all would otherwise meet the
  // empty-list sentence and stop reading before the recommendation.
  proposed?: ReactNode;
  renderAction?: (step: NextStep) => ReactNode;
  // Given, the subject opens the task where it is listed. Absent, it stays
  // plain text rather than a button that goes nowhere.
  onOpenTask?: (step: NextStep) => void;
  // Wires the tick to the real completion write. Absent (the stories file,
  // a read-only account) draws the row with no checkbox at all — a box that
  // cannot be ticked is worse than no box.
  update?: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const steps = view.next_steps?.data ?? [];
  const state = sectionState(
    view,
    "next_steps",
    Boolean(view.next_steps),
    steps.length,
  );
  // A withheld block is dropped entirely — the middle column is the story,
  // and a refusal in the middle of it says nothing a rep can act on. Every
  // other state is shown, because "no open task" and "we could not tell"
  // lead to different next moves.
  if (state === "withheld") {
    return null;
  }
  return (
    <Panel title={t("co.next.title")}>
      {proposed}
      {state === "unavailable" && (
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.unavailable")}</p>
        </PanelBody>
      )}
      {state === "empty" && (
        <PanelBody>
          <p className="surfacestate-empty">{t("co.next.empty")}</p>
        </PanelBody>
      )}
      {state === "ready" &&
        steps.map((step) => (
          <PanelRow key={step.activity_id} className="co-task-row">
            {update && (
              <TaskCompleteCheck
                activityId={step.activity_id}
                version={step.version}
                update={update}
              />
            )}
            <span className="co-task-body">
              {onOpenTask ? (
                <button
                  type="button"
                  className="co-rowlink"
                  onClick={() => onOpenTask(step)}
                >
                  {step.subject}
                </button>
              ) : (
                <span>{step.subject}</span>
              )}
              <span className="co-row-meta t-caption">
                {step.overdue && (
                  <Badge tone="danger">{t("co.next.overdue")}</Badge>
                )}
                {!step.overdue && step.due_at && (
                  <span>
                    {t("co.next.due", {
                      // The record's own clock, like the timeline below it. A
                      // deadline is a promise colleagues read back, so
                      // `dueInstant` mints the picked day's end in this same
                      // zone and it is rendered in it. Reading that last second
                      // on the browser's clock instead is what showed an
                      // approved 9 September as a next step due the 10th.
                      when: formatDate(step.due_at, locale, recordZone),
                    })}
                  </span>
                )}
                {!step.due_at && <span>{t("co.next.undated")}</span>}
                {step.linked_deal_id && (
                  <EntityRef kind="deal" id={step.linked_deal_id} />
                )}
                {step.linked_contact_id && (
                  <EntityRef kind="contact" id={step.linked_contact_id} />
                )}
                {step.assignee_id && (
                  <EntityRef kind="user" id={step.assignee_id} />
                )}
              </span>
            </span>
            {renderAction && (
              <span className="co-task-verbs">{renderAction(step)}</span>
            )}
          </PanelRow>
        ))}
    </Panel>
  );
}
