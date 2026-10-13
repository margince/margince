import type { ReactNode } from "react";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "../lib/problem";
import { Button, EmptyState, PendingBody } from "./atoms";
import { Callout } from "./callout";
import "./querystates.css";

// The minimal read surface QueryGate/QueryStates need. A real react-query
// `UseQueryResult<Data>` is structurally assignable to it, and a hook that
// merges several queries (e.g. the decided-approvals fan-out) can return a
// plain object of this shape, with no `as unknown as UseQueryResult` cast.
export interface QueryLike<Data> {
  isPending: boolean;
  isError: boolean;
  error: unknown;
  data: Data | undefined;
  refetch: () => unknown;
}

// The pending and error halves of the screen-state matrix: one skeleton
// spelling and one error+retry spelling for every query-backed screen.
// A useQuery and a useInfiniteQuery both expose this isPending/isError/error/refetch shape.
// Success rendering stays the caller's job. Some screens want QueryGate's empty
// check, and others (the History timelines) need grouping and pagination that
// no single success renderer could cover.
export function QueryStates({
  query,
  pendingLabel,
  pendingLines,
  children,
}: Readonly<{
  query: Readonly<{
    isPending: boolean;
    isError: boolean;
    error: unknown;
    refetch: () => unknown;
  }>;
  // What this particular read is fetching, and how much room it will need.
  // Most screens answer the generic question, and the generic answer is true
  // for them. A screen whose body is a table or a timeline says so. Otherwise
  // its content lands in a third of the space the placeholder held and pushes
  // the page down as it arrives.
  pendingLabel: string;
  pendingLines?: number;
  children: ReactNode;
}>) {
  const t = useT();
  if (query.isPending) {
    return (
      <PendingBody
        label={pendingLabel ?? t("common.loading")}
        lines={pendingLines}
      />
    );
  }
  if (query.isError) {
    return (
      <EmptyState>
        {/* role="alert" (the assertive live region), because this subtree
            mounts carrying its message. A polite region inserted with its text
            is often never announced; an assertive one is announced on insertion.
            confirmmodal.tsx marks its mutation failure the same way. Headline
            and cause share one region so the reader hears a whole failure.
            Retry stays outside it: a live region reads out its contents, and
            the button is something to reach, not something to hear. */}
        <div role="alert">
          <p>{t("common.error")}</p>
          <p className="querygate-cause">{problemMessageOf(query.error, t)}</p>
        </div>
        <Button onClick={() => query.refetch()} className="querygate-retry">
          {t("common.retry")}
        </Button>
      </EmptyState>
    );
  }
  return <>{children}</>;
}

/**
 * A write the server refused, beside the control that asked for it. The
 * screen's own claim is the heading, and the server's cause sits under it.
 *
 * One spelling for every screen. Each copy of these lines is one more place
 * for a refusal to stop interrupting or to lose the server's words.
 * `outcome` + `danger` derives the interrupting role.
 *
 * The heading arrives as a key, so no caller can hand this a sentence the
 * catalogue never said. The cause is `message` where the caller holds the
 * words, and `error` where it holds only the thrown failure. With neither,
 * nothing is drawn, so a caller states no condition of its own. A separate
 * condition can draw a band around a notice that renders nothing.
 */
export function WriteRefused({
  titleKey,
  message,
  error,
  actions,
}: Readonly<{
  titleKey: MessageKey;
  message?: string | null;
  error?: unknown;
  /** What the reader can do about it: a retry, a way back out. */
  actions?: ReactNode;
}>) {
  const t = useT();
  const cause =
    message ??
    (error === null || error === undefined ? null : problemMessageOf(error, t));
  if (cause === null) {
    return null;
  }
  return (
    <Callout kind="outcome" tone="danger" title={t(titleKey)} actions={actions}>
      {cause}
    </Callout>
  );
}

// The one "Load more" spelling for every keyset-paginated infinite query
// (record history, field history, the settings audit log). A small button
// fetches the next page and disables itself mid-fetch. It renders only
// while the query still reports another page.
export function LoadMoreButton({
  query,
}: Readonly<{
  query: Readonly<{
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    fetchNextPage: () => unknown;
  }>;
}>) {
  const t = useT();
  if (!query.hasNextPage) {
    return null;
  }
  return (
    <Button
      className="load-more"
      disabled={query.isFetchingNextPage}
      onClick={() => query.fetchNextPage()}
    >
      {t("list.loadMore")}
    </Button>
  );
}

export function QueryGate<Data>({
  query,
  empty,
  pendingLabel,
  pendingLines,
  children,
}: Readonly<{
  query: QueryLike<Data>;
  empty?: (data: Data) => boolean;
  // Forwarded to QueryStates: the gate adds the empty check, not a second
  // pending state, so there is one place a screen names its wait.
  pendingLabel: string;
  pendingLines?: number;
  children: (data: Data) => ReactNode;
}>) {
  const t = useT();
  // A QueryLike isn't a discriminated union, so TS can't narrow it. Past
  // QueryStates' pending/error guards `data` is present. So success rendering
  // keys off its presence, not a react-query `isSuccess` flag that the merged
  // fan-out hooks don't expose.
  const data = query.data;
  let success: ReactNode = null;
  if (data !== undefined) {
    success = empty?.(data) ? (
      <EmptyState>{t("common.empty")}</EmptyState>
    ) : (
      children(data)
    );
  }
  return (
    <QueryStates
      query={query}
      pendingLabel={pendingLabel}
      pendingLines={pendingLines}
    >
      {success}
    </QueryStates>
  );
}
