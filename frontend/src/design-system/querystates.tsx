import type { ReactNode } from "react";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "../lib/problem";
import { Button, EmptyState, PendingBody } from "./atoms";
import { Callout } from "./callout";
import "./querystates.css";

// The minimal read surface QueryGate/QueryStates need. A real react-query
// `UseQueryResult<Data>` is structurally assignable to it, and a hook that
// MERGES several queries (e.g. the decided-approvals fan-out) can return a
// plain object of this shape — no `as unknown as UseQueryResult` lie required.
export interface QueryLike<Data> {
  isPending: boolean;
  isError: boolean;
  error: unknown;
  data: Data | undefined;
  refetch: () => unknown;
}

// The pending/error halves of the screen-state matrix (§3a) — one skeleton
// spelling, one error+retry spelling — shared by every query-backed screen
// regardless of whether it's a plain useQuery or an useInfiniteQuery (both
// expose this same isPending/isError/error/refetch shape). SUCCESS rendering
// stays the caller's job: some screens want QueryGate's generic empty-check,
// others (the History timelines) need custom grouping/pagination that no
// single success renderer could cover.
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
  // Both optional because most screens are answering the generic question and
  // the generic answer is honest for them; a screen whose body is a table or a
  // timeline says so, or its content lands in a third of the space the
  // placeholder held and pushes the page down as it arrives.
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
        {/* role="alert" — the assertive live region — because this subtree
            MOUNTS carrying its message: a polite region inserted together with
            its text is frequently never announced, while an assertive one is
            announced on insertion, which is why confirmmodal.tsx marks its
            mutation failure the same way. Headline and cause share ONE region so
            the reader hears a whole failure rather than two fragments; Retry
            stays outside it, since a live region reads out its contents and the
            button is something to reach, not something to hear. */}
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
 * A write the server refused, beside the control that asked for it: the
 * screen's own claim as the heading, the cause the server sent under it.
 *
 * ONE spelling, because a dozen screens had each grown their own around the
 * same nine lines — a dozen places for a refusal to stop interrupting or to
 * lose the server's words. `outcome` + `danger` derives the interrupting role.
 *
 * The heading arrives as a KEY, so no caller can hand this a sentence the
 * catalogue never said. The cause is `message` where the caller already holds
 * the words and `error` where it holds only the thrown failure; with NEITHER
 * there is nothing to report and nothing is drawn, so a caller states no
 * condition of its own — answering that separately is how a band came to be
 * drawn around a notice that rendered nothing.
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
// (record history, field history, the settings audit log): a small button
// that fetches the next page and disables itself mid-fetch, rendered only
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
  // A QueryLike isn't a discriminated union, so TS can't narrow it: past
  // QueryStates' pending/error guards `data` is present, so key SUCCESS
  // rendering off its presence rather than a react-query `isSuccess` flag
  // the merged fan-out hooks don't expose.
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
