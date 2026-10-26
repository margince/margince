import type { components } from "../../api/schema";
import type { TimelineEventKind } from "../../design-system/standingtimeline";
import { stable } from "../../format/collate";
import { lastUpdate } from "./projectstanding";

// The project's dated moments for the How it stands axis, as data the page
// words. Only what the payload dates: the start, signed contracts, won deals,
// the next open task, the newest update and the target end. Phase moves stay
// on the ladder and in Phase history.

type Project360 = components["schemas"]["Project360"];
type PhaseMover = components["schemas"]["PrincipalAttribution"];

type Moment<What extends string, Extra> = Readonly<
  { key: string; at: string; kind: TimelineEventKind; what: What } & Extra
>;

export type ProjectMoment =
  | Moment<"opened", { phase: string; by: PhaseMover | null }>
  | Moment<"signed", { title: string }>
  | Moment<"won", { name: string; amountMinor?: number; currency?: string }>
  | Moment<"due", { subject: string }>
  | Moment<"update", { subject?: string }>
  | Moment<"end", Record<never, never>>;

// The start opens the axis, so the range runs from the day the work began
// even when the record was entered later.
function openedMoment(view: Project360): ProjectMoment[] {
  const first = view.phase_history?.data[0];
  const at = view.project.started_at ?? first?.changed_at;
  if (!at) {
    return [];
  }
  return [
    {
      key: "opened",
      at,
      kind: Date.parse(at) > Date.parse(view.as_of) ? "ahead" : "done",
      what: "opened",
      phase: first?.to_phase ?? "initiative",
      by: first?.changed_by ?? null,
    },
  ];
}

// The newest filed activity names the update; the rollup only dates it.
function newestSubject(view: Project360): string | undefined {
  const newest = [...(view.activities?.data ?? [])].sort((a, b) =>
    stable(b.occurred_at, a.occurred_at),
  )[0];
  return newest?.subject || undefined;
}

function paperMoments(view: Project360): ProjectMoment[] {
  const signed: ProjectMoment[] = (view.contracts?.data ?? []).flatMap(
    (contract) =>
      contract.signed_on
        ? [
            {
              key: `contract-${contract.id}`,
              at: contract.signed_on,
              kind: "done",
              what: "signed",
              title: contract.title,
            },
          ]
        : [],
  );
  const won: ProjectMoment[] = (view.deals?.data ?? []).flatMap((deal) =>
    deal.status === "won" && deal.closed_at
      ? [
          {
            key: `deal-${deal.id}`,
            at: deal.closed_at,
            kind: "done",
            what: "won",
            name: deal.name,
            amountMinor: deal.amount_minor ?? undefined,
            currency: deal.currency ?? undefined,
          },
        ]
      : [],
  );
  return [...signed, ...won];
}

// The first overdue task and the first one still ahead: the axis shows what
// slipped and what comes next, not the whole task list.
function taskMoments(view: Project360): ProjectMoment[] {
  const dated = (view.commitments?.data ?? []).filter((row) => row.due_at);
  const picks = [
    dated.find((row) => row.overdue),
    dated.find((row) => !row.overdue),
  ];
  return picks.flatMap((row) =>
    row?.due_at
      ? [
          {
            key: `task-${row.activity_id}`,
            at: row.due_at,
            kind: row.overdue ? "late" : "ahead",
            what: "due",
            subject: row.subject,
          },
        ]
      : [],
  );
}

/** Every moment the axis draws, in no particular order (the axis sorts). */
export function projectMoments(view: Project360): ProjectMoment[] {
  const moments = [
    ...openedMoment(view),
    ...paperMoments(view),
    ...taskMoments(view),
  ];
  const last = lastUpdate(view);
  if (last) {
    moments.push({
      key: "update",
      at: last.at,
      kind: "last",
      what: "update",
      subject: newestSubject(view),
    });
  }
  const end = view.project.target_end_date;
  if (end && view.project.phase !== "closed") {
    moments.push({ key: "end", at: end, kind: "end", what: "end" });
  }
  return moments;
}
