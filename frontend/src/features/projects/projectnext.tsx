import type { ComponentProps, ReactNode } from "react";
import { RecordNextBar } from "../../components/latestinbound";
import type { NextBar } from "../../design-system/nextbar";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { reveal } from "../../lib/reveal";
import { useMe } from "../../lib/session";
import { dueOnDay, tasksPeek } from "../../lib/standingpeeks";
import { useViewerZone } from "../../lib/viewerzone";
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
  const zone = useViewerZone();
  const companyZone = useCompanyZone();
  const me = useMe();
  const next = nextCommitment(view, { zone, companyZone });
  if (!next) {
    return <RecordNextBar />;
  }
  const mine = next.assigneeId !== null && next.assigneeId === me.data?.user.id;
  const due =
    next.dueAt &&
    dueOnDay(
      { zone, companyZone },
      next.dueAt,
      next.dueHasTime,
      next.dueIntent,
    );
  const sub = [
    due &&
      t("project.standing.nextDue", {
        date: formatDayMonth(due.at, locale, due.zone),
      }),
    mine
      ? t("project.standing.assignedYou")
      : next.assigneeName &&
        t("project.standing.assignedTo", { name: next.assigneeName }),
  ].filter(Boolean);
  return (
    <RecordNextBar
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
            dueHasTime: row.due_has_time,
            dueIntent: row.due_intent,
            overdue: row.overdue,
          })),
          { t, plural, locale, zone, companyZone },
          next.more,
          onOpenTask,
        ),
      }}
    />
  );
}
