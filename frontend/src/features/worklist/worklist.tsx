import { useQueryClient } from "@tanstack/react-query";
import { Button } from "../../design-system/atoms";
import { SurfaceState } from "../../design-system/surfacestate";
import { viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { useOpenEmail } from "../../lib/openemail";
import { problemCodeOf } from "../../lib/problem";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { useWorklistAddress } from "./worklist.address";
import { WORKLIST_FILTER_PARAM } from "./worklist.header";
import {
  loadedQueue,
  useRefreshWalk,
  useWorklist,
  worklistKey,
} from "./worklist.queries";
import "./worklist.css";

// The Worklist: one ranked list, not fourteen lanes.
//
// The page it replaces was organized by producer, so a reader had to compare
// the position of one panel against another to work out that an item several
// screens down mattered more. This draws the order the server decided, and
// never re-sorts: the tie-breaks depend on a base-currency conversion and a
// materiality threshold the browser does not hold.
//
// Every row answers the five questions the surface exists for — what happened,
// why now, what is at stake, what to do, and can I do it here.

// The address dial, re-exported from where it lives. It belongs beside the
// strip that draws the lanes (`worklist.header.tsx`), and it is reachable from
// the SCREEN because another surface names one of this screen's lanes to open
// it — a reading in the Brief. One spelling of the parameter, two ways in.
export { WORKLIST_FILTER_PARAM };

import { WorklistBody } from "./worklist.body";
import { accumulatedDay } from "./worklist.reads";

// The Worklist screen.
export function WorklistScreen({
  opensOn,
  embedded = false,
}: Readonly<{
  // What the address asked for, from `#/worklist/<segment>`: a user id opens
  // that contact's queue, and the literal "unassigned" opens the unowned pile.
  // Both are doors a team board row needs — a row that could only reach this
  // page would ask the reader to pick the same thing a second time.
  //
  // An unassigned path supplies the default scope; an explicit scope query
  // overrides it so the scope dial remains usable after following that link.
  opensOn?: string;
  embedded?: boolean;
}> = {}) {
  const t = useT();
  const {
    scope,
    view,
    filter,
    owner,
    selectedId,
    setScope,
    setView,
    setFilter,
    setOwner,
    setSelectedId,
  } = useWorklistAddress(opensOn, embedded);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const day = useWorklist(
    scope,
    filter,
    owner === "" ? undefined : owner,
    true,
    view,
  );
  const refreshWalk = useRefreshWalk();
  const queryClient = useQueryClient();
  // A failed next batch is not a failed page. The rows already loaded stay,
  // and the scroll sentinel below them says the batch failed.
  const lostTheDay = day.isError && day.data === undefined;
  // A refusal is not a failure. A named owner this reader has no reach over
  // answers 403, and "did not load. Retry" invites a retry that can only
  // refuse again.
  const refused =
    lostTheDay && problemCodeOf(day.error) === "permission_denied";
  const state = day.isPending
    ? "loading"
    : refused
      ? "withheld"
      : lostTheDay
        ? "failed"
        : "ready";
  const loadedDay = accumulatedDay(day.data?.pages);
  // Arrival and departure counts describe the latest page's view of the whole walk.
  const walk = day.data?.pages.at(-1)?.walk;
  // The rows, which DO grow. Deduped in the query, because a re-rank between
  // reads can serve one row on two pages.
  const queue = day.data ? loadedQueue(day.data.pages) : [];
  return (
    <div className="wrap worklist">
      {/* No label: the shell already heads this page, and a second "Worklist"
          would be announced twice and nest an h3 above the queue's own h2. */}
      {/* The way BACK, drawn OUTSIDE the surface state.
          A named owner this reader may not open answers 403, so the day fails
          to load and the body — the owner picker with it — never renders.
          Without this the reader is stranded on a page whose only control is
          gone, and reloading is the way out. */}
      {owner !== "" && (state === "failed" || state === "withheld") && (
        <Button onClick={() => setOwner("")}>
          {t("worklist.owner.backToMine")}
        </Button>
      )}
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.clear")}
        loadingLabel={t("worklist.loading")}
        detail={
          refused
            ? { withheldReason: t("worklist.owner.refused") }
            : { onRetry: () => void day.refetch() }
        }
      >
        {loadedDay && (
          <WorklistBody
            embedded={embedded}
            day={loadedDay}
            walk={walk}
            // Refreshing starts a NEW walk, which is what brings in the work
            // that arrived behind the reader — and a refetch is NOT that: it
            // re-runs every loaded page with its own cursor, so a reader who
            // had paged once resumed the walk they were asking to leave.
            onRefresh={refreshWalk}
            queue={queue}
            scope={scope}
            view={view}
            filter={filter}
            owner={owner}
            selectedId={selectedId}
            onScope={setScope}
            onView={setView}
            onFilter={setFilter}
            onOwner={setOwner}
            onSelect={setSelectedId}
            onOpenEmail={setOpenEmail}
            cursor={day.data?.pages.at(-1)?.next_cursor ?? undefined}
            loadingMore={day.isFetchingNextPage}
            moreFailed={day.isFetchNextPageError}
            onMore={() => void day.fetchNextPage()}
          />
        )}
      </SurfaceState>
      {/* One drawer over the whole queue, at page level rather than inside a
          row: two mounted dialogs would be two `aria-modal` elements, and the
          day stays legible behind the message being read. */}
      {/* A reply sent from the drawer refreshes the queue, exactly as the
          row's own Reply does. Without it the message a rep just answered
          keeps its place in the waiting lane and keeps offering to answer it,
          which is the one thing this lane exists to stop saying. Invalidated
          rather than reset: the walk stays where the reader had paged it. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void queryClient.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </div>
  );
}
