import type { ComponentProps, ReactNode } from "react";
import { NextBar } from "../../design-system/nextbar";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { reveal } from "../../lib/reveal";
import { useMe } from "../../lib/session";
import { tasksPeek } from "../../lib/standingpeeks";
import { PROJECT_COMMITMENTS_ANCHOR } from "./projectreadings";
import type { Project360 } from "./projectsections";
import { nextCommitment } from "./projectstanding";

// The project's next move: its soonest-due open task. The subject opens the
// task itself. The verbs are the task's own, built by the page that may write
// it, and absent for a reader who may not.
export function ProjectNext({
  view,
  onOpenTask,
  action,
  snooze,
}: Readonly<{
  view: Project360;
  onOpenTask: (activityId: string) => void;
  action?: ReactNode;
  snooze?: ComponentProps<typeof NextBar>["snooze"];
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const me = useMe();
  const next = nextCommitment(view);
  if (!next) {
    return <NextBar />;
  }
  const mine = next.assigneeId !== null && next.assigneeId === me.data?.user.id;
  const sub = [
    next.dueAt &&
      t("project.standing.nextDue", {
        date: formatDayMonth(next.dueAt, locale, zone),
      }),
    mine
      ? t("project.standing.assignedYou")
      : next.assigneeName &&
        t("project.standing.assignedTo", { name: next.assigneeName }),
  ].filter(Boolean);
  return (
    <NextBar
      title={
        <button
          type="button"
          className="project-rowlink"
          onClick={() => onOpenTask(next.activityId)}
        >
          {next.subject}
        </button>
      }
      late={
        next.overdue
          ? {
              label:
                next.daysLate > 0
                  ? plural("project.standing.daysLate", next.daysLate, {
                      count: formatNumber(next.daysLate, locale),
                    })
                  : t("project.commitments.overdue"),
              tone: "danger",
            }
          : undefined
      }
      sub={sub.length > 0 ? sub.join(" · ") : undefined}
      action={action}
      snooze={snooze}
      more={{
        count: next.more,
        onOpen: reveal(PROJECT_COMMITMENTS_ANCHOR),
        peek: tasksPeek(
          (view.commitments?.data ?? []).slice(1).map((row) => ({
            id: row.activity_id,
            subject: row.subject,
            dueAt: row.due_at,
            overdue: row.overdue,
          })),
          { t, plural, locale, zone },
          next.more,
          onOpenTask,
        ),
      }}
    />
  );
}
