// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One rep's day, read the way the Worklist reads it. Home, the Worklist and
// Analytics' own outcomes ask through these options, so one key serves them
// all. A screen that mounts while another waits joins its request.

import { infiniteQueryOptions } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { isLiveWorkRead, worklistKey } from "./workkeys";
import { categoryInView, type WorklistView } from "./worklistviews";

type Worklist = components["schemas"]["Worklist"];
type WorklistScope = Worklist["scope"];
type WorklistFilter = NonNullable<Worklist["filter"]>;

/** The read useWorklist makes, which Home's prefetch also starts. */
export function worklistDayOptions(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner?: string,
  view?: WorklistView,
) {
  // A view joins the key only when named, so Home's read keeps its own key.
  const queryKey = [
    ...worklistKey,
    scope,
    filter,
    owner ?? "",
    ...(view ? [view] : []),
  ];
  return infiniteQueryOptions({
    queryKey,
    ...(!isLiveWorkRead(queryKey)
      ? { refetchOnWindowFocus: true }
      : {
          refetchInterval: (query: {
            state: { data?: { pages: Worklist[] } };
          }) => untilNextDue(query.state.data?.pages ?? [], Date.now()),
        }),
    initialPageParam: undefined as string | undefined,
    // React Query aborts the signal once no screen still observes this key, and
    // the server then stops assembling the day instead of answering nobody.
    queryFn: ({ pageParam, signal }) =>
      readWorklistPage(scope, filter, owner, pageParam, signal, view),
    // A page can empty as rows leave the snapshot; only its cursor ends the walk.
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

// The contract's largest page. Every page assembles the whole day again, so a
// small page buys no speed, and each further page costs a full read.
export const WORKLIST_PAGE_ROWS = 100;

/** One page of the day at those dials, the request `useWorklist` makes. */
export async function readWorklistPage(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner: string | undefined,
  cursor: string | undefined,
  signal?: AbortSignal,
  view?: WorklistView,
): Promise<Worklist> {
  // Decisions and System also send their own filter, so a server that knows
  // no `view` still answers with that view's rows.
  const wire = view && view !== "today" && filter === "all" ? view : filter;
  // Reconnections are read only when their panel opens; see readRelationships.
  const whose = owner
    ? { owner, filter: wire, relationships: "omit" as const }
    : { scope, filter: wire, relationships: "omit" as const };
  const dials = view
    ? { ...whose, view, limit: WORKLIST_PAGE_ROWS }
    : { ...whose, limit: WORKLIST_PAGE_ROWS };
  const page = unwrap(
    await api.GET("/worklist", {
      params: { query: cursor ? { ...dials, cursor } : dials },
      signal,
    }),
  );
  return view === undefined || page.view !== undefined
    ? page
    : withinView(page, view);
}

/** The day's optional reconnections, the one part a day read omits. */
export async function readRelationships(
  scope: WorklistScope,
  owner: string | undefined,
  signal?: AbortSignal,
): Promise<
  Pick<
    Worklist,
    "optional_relationships" | "optional_relationships_more_available"
  >
> {
  const whose = owner ? { owner } : { scope };
  const page = unwrap(
    await api.GET("/worklist", {
      params: { query: { ...whose, relationships: "include", limit: 1 } },
      signal,
    }),
  );
  return {
    optional_relationships: page.optional_relationships ?? [],
    optional_relationships_more_available:
      page.optional_relationships_more_available,
  };
}

// A server that echoes no view ignored it, so the rows outside the view are
// dropped here rather than drawn under the view's heading.
function withinView(page: Worklist, view: WorklistView): Worklist {
  return {
    ...page,
    queue: page.queue.filter((item) => categoryInView(item.category, view)),
  };
}

// A due row changes once its time comes: a meeting begins, a task falls
// overdue. Those are the moments the day is read again, rather than every
// minute, because each read re-assembles every page the reader loaded.
const DUE_SLACK_MS = 1_000;

// A browser fires a timer longer than 2^31 ms (24.8 days) at once. A row due
// next month would then read the day again the moment each read ends.
const LONGEST_WAIT_MS = 60 * 60_000;

/** How long until a loaded row comes due, or false when none still will. */
export function untilNextDue(pages: Worklist[], now: number): number | false {
  let next = Number.POSITIVE_INFINITY;
  for (const page of pages) {
    for (const item of [...(page.queue ?? []), ...(page.focus?.items ?? [])]) {
      const due = item.due_at ? Date.parse(item.due_at) : Number.NaN;
      if (due > now && due < next) next = due;
    }
  }
  return Number.isFinite(next)
    ? Math.min(next - now + DUE_SLACK_MS, LONGEST_WAIT_MS)
    : false;
}
