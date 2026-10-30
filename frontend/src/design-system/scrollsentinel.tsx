// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { Button } from "./atoms";
import { ErrorLine } from "./errorline";
import "./scrollsentinel.css";

// How far below the visible edge the next batch is asked for, so rows arrive
// before the reader reaches the end.
const AHEAD = "0px 0px 480px 0px";

/** What the sentinel last asked for, so one cursor is never asked twice. */
type Asked = Readonly<{ list: string; cursor: string; stalled: boolean }>;

export type ScrollSentinelProps = Readonly<{
  /** Which list this is. A new value starts a separate list. */
  list: string;
  /** Where the next batch starts. Absent once the list has no continuation. */
  cursor: string | undefined;
  /** A batch is in flight. */
  loading: boolean;
  /** The last continuation failed. Rows already loaded stay. */
  failed: boolean;
  /** Asks for the batch at `cursor`. */
  onLoadMore: () => void;
  /** What the end of the list says once every batch is loaded. */
  end?: ReactNode;
}>;

/**
 * The end of a list that loads its next batch as the reader nears it. It asks
 * once per cursor, keeps asking while the list is shorter than the screen, and
 * stops on a failure or a cursor that did not move, offering Retry instead.
 */
export function ScrollSentinel({
  list,
  cursor,
  loading,
  failed,
  onLoadMore,
  end,
}: ScrollSentinelProps) {
  const t = useT();
  const marker = useRef<HTMLDivElement>(null);
  const [asked, setAsked] = useState<Asked | null>(null);
  const mine = asked?.list === list ? asked : null;
  const stalled = mine?.stalled ?? false;
  const observable = typeof IntersectionObserver !== "undefined";
  const waiting =
    cursor !== undefined && !loading && !failed && !stalled && observable;
  // Whether the last ask reached the network. Only an answered ask that left
  // the cursor where it was is a stall; an ask not yet started is not.
  const answered = useRef(false);
  useEffect(() => {
    if (loading) answered.current = true;
  }, [loading]);
  // Held in a ref, so a caller's new function each render does not re-observe.
  const load = useRef(onLoadMore);
  useEffect(() => {
    load.current = onLoadMore;
  }, [onLoadMore]);
  // Recreated whenever a batch lands: observing again reports the marker's
  // position afresh, which is what fills a screen the first batch left short.
  useEffect(() => {
    const element = marker.current;
    if (!waiting || !element || cursor === undefined) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        observer.disconnect();
        if (mine?.cursor === cursor) {
          if (answered.current) setAsked({ list, cursor, stalled: true });
          return;
        }
        answered.current = false;
        setAsked({ list, cursor, stalled: false });
        load.current();
      },
      { rootMargin: AHEAD },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [waiting, cursor, list, mine?.cursor]);
  const retry = () => {
    answered.current = false;
    setAsked(cursor === undefined ? null : { list, cursor, stalled: false });
    onLoadMore();
  };
  return (
    <div ref={marker} className="scroll-sentinel">
      <div className="t-caption" aria-live="polite">
        {sentinelLine({ cursor, loading, failed, stalled, observable, end, t })}
      </div>
      {cursor !== undefined && !loading && (failed || stalled || !observable) && (
        <Button onClick={retry}>
          {observable ? t("common.retry") : t("list.loadMore")}
        </Button>
      )}
    </div>
  );
}

function sentinelLine({
  cursor,
  loading,
  failed,
  stalled,
  observable,
  end,
  t,
}: Readonly<{
  cursor: string | undefined;
  loading: boolean;
  failed: boolean;
  stalled: boolean;
  observable: boolean;
  end: ReactNode;
  t: ReturnType<typeof useT>;
}>): ReactNode {
  if (loading) return t("scrollSentinel.loading");
  if (failed) return <ErrorLine inline>{t("scrollSentinel.failed")}</ErrorLine>;
  if (stalled) return t("scrollSentinel.stalled");
  if (cursor === undefined) return end ?? null;
  return observable ? null : t("scrollSentinel.more");
}
