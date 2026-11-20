// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  InfiniteQueryObserver,
  type QueryClient,
  type QueryKey,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { morningBriefOptions } from "../features/brief/brief.queries";
import { addressFrom } from "../features/brief/brief.view";
import { type AccessSnapshot, grants } from "../lib/accessgrants";
import { handledForYouOptions } from "../lib/handled";
import { magicOptions } from "../lib/magic.queries";
import { currentParams, type UrlParams } from "../lib/urlstate";
import { homeDayOptions } from "../lib/worklistday";
import { notePrefetched } from "./queryclient";

// Fails closed while the session probe is out: the brief and the receipt need
// `deal.read`, so they start only once the grant is known.
export function mayReadHomeDeals(me: AccessSnapshot): boolean {
  return me !== undefined && grants(me, "deal", "read");
}

// Noted only when the call fetched. A prefetch also resolves on a fresh cache
// hit, and that answer is not one this load fetched.
function prefetched(
  client: QueryClient,
  queryKey: QueryKey,
  fetch: () => Promise<void>,
): void {
  const find = () => client.getQueryCache().find({ queryKey, exact: true });
  const before = find()?.state.dataUpdateCount ?? 0;
  void fetch().then(() => {
    const query = find();
    if (query && query.state.dataUpdateCount > before) {
      notePrefetched(query.queryHash, query.state.dataUpdatedAt);
    }
  });
}

const opensMorning = (params: UrlParams) =>
  addressFrom(params).view === "morning";

// Home's morning reads, started with the session probe (App.tsx). The screen
// commits only after React's Suspense reveal, and its queries start then.
// Each read uses its hook's own options, so the screen joins a request in
// flight. An answer already back is noted, so the screen's mount does not fetch
// it again (liveOnMount in queryclient.ts).
export function prefetchHome(
  client: QueryClient,
  params: UrlParams,
): () => void {
  if (!opensMorning(params)) {
    return () => undefined;
  }
  const day = homeDayOptions();
  // The returned cleanup belongs to the route. A prefetch has no observer, so a
  // reader who leaves first would leave the day assembling. One is held while the
  // read is in flight, so React Query can abort it; after, it would count as a
  // screen showing the day.
  const release = new InfiniteQueryObserver(client, {
    ...day,
    refetchInterval: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  }).subscribe(() => undefined);
  prefetched(client, day.queryKey, () =>
    client.prefetchInfiniteQuery(day).then(release),
  );
  prefetched(client, handledForYouOptions.queryKey, () =>
    client.prefetchInfiniteQuery(handledForYouOptions),
  );
  return release;
}

// The server refuses both to a reader without `deal.read`, and a failed
// prefetch would leave an error in the cache for the screen to show. They start
// apart from the day, which is in flight when the grant arrives and would be
// aborted and sent again by a shared cleanup.
export function prefetchHomeDeals(
  client: QueryClient,
  params: UrlParams,
): void {
  if (!opensMorning(params)) {
    return;
  }
  prefetched(client, morningBriefOptions.queryKey, () =>
    client.prefetchQuery(morningBriefOptions),
  );
  prefetched(client, magicOptions.queryKey, () =>
    client.prefetchQuery(magicOptions),
  );
}

/** Starts Home's reads for the shell: the day at once, the deal reads on the grant. */
export function useHomePrefetch(opensHome: boolean, me: AccessSnapshot): void {
  const client = useQueryClient();
  const readsDeals = mayReadHomeDeals(me);
  useEffect(() => {
    if (!opensHome) {
      return;
    }
    return prefetchHome(client, currentParams());
  }, [opensHome, client]);
  useEffect(() => {
    if (opensHome && readsDeals) {
      prefetchHomeDeals(client, currentParams());
    }
  }, [opensHome, client, readsDeals]);
}
