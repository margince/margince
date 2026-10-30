import type { components } from "../../api/schema";
import { RecordNextBar } from "../../components/latestinbound";
import { Button } from "../../design-system/atoms";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { type PeekTask, tasksPeek } from "../../lib/standingpeeks";
import type { Lead } from "./leads";
import { leadNext } from "./leadstandingparts";

type Activity = components["schemas"]["Activity"];

/**
 * The lead's next move, at the top of its Overview: qualify it, or its next
 * open task. A lead has no snooze, so the bar offers none. The other open
 * tasks open the work queue, since a lead names no task list of its own.
 */
export function LeadNextBar({
  lead,
  asOf,
  refusedReasonId,
  onQualify,
  activities = [],
}: Readonly<{
  lead: Lead;
  // The lead's loaded activity page, where its open tasks are read from.
  activities?: readonly Activity[];
  asOf: string;
  // The page's one sentence about why this lead takes no changes.
  refusedReasonId?: string;
  onQualify: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const record = useRecordZone();
  const next = leadNext(lead, { t, locale, zone, record, asOf });
  return (
    <RecordNextBar
      // A lead ready to qualify outranks what just came in.
      urgent={next.qualify || undefined}
      title={next.title}
      late={next.late}
      sub={next.sub}
      action={
        next.qualify ? (
          <Button
            variant="ghost"
            reasonId={refusedReasonId}
            onClick={onQualify}
          >
            {t("lead.promote")}
          </Button>
        ) : undefined
      }
      more={{
        count: next.more,
        onOpen: () => navigate({ screen: "worklist" }),
        peek: tasksPeek(
          otherTasks(lead, activities).slice(0, next.more),
          { t, plural, locale, zone },
          next.more,
          () => navigate({ screen: "worklist" }),
        ),
      }}
    />
  );
}

// The open tasks on the page behind the one the bar names, nearest date first,
// undated last. The lead keeps only that one task's subject, so the first
// task that matches it and its date is the one the bar draws.
function otherTasks(lead: Lead, activities: readonly Activity[]): PeekTask[] {
  const open = activities
    .filter((row) => row.kind === "task" && !row.is_done && !row.archived_at)
    .sort(
      (a, b) =>
        Date.parse(a.due_at ?? "9999-12-31") -
        Date.parse(b.due_at ?? "9999-12-31"),
    );
  const named = open.findIndex(
    (row) =>
      row.subject === lead.next_task_subject &&
      (row.due_at ?? null) === (lead.next_task_due_at ?? null),
  );
  return open
    .filter((_, index) => index !== named)
    .map((row) => ({
      id: row.id,
      subject: row.subject ?? "",
      dueAt: row.due_at,
    }));
}
