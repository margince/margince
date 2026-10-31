// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient, QueryKey } from "@tanstack/react-query";
import { morningBriefOptions } from "../features/brief/brief.queries";
import { addressFrom } from "../features/brief/brief.view";
import { handledForYouOptions } from "../lib/handled";
import { magicOptions } from "../lib/magic.queries";
import type { UrlParams } from "../lib/urlstate";
import { worklistDayOptions } from "../lib/worklistday";
import { notePrefetched } from "./queryclient";

// Home's morning reads, started as soon as the shell may draw (App.tsx). The
// screen commits only after React's Suspense reveal, and its queries start then.
// Each read uses its hook's own options, so the screen joins a request in
// flight. An answer already back is noted, so the screen's mount does not fetch
// it again (liveOnMount in queryclient.ts).
export function prefetchHome(client: QueryClient, params: UrlParams): void {
  if (addressFrom(params).view !== "morning") {
    return;
  }
  // Noted only when the call fetched. A prefetch also resolves on a fresh cache
  // hit, and that answer is not one this load fetched.
  const prefetched = (queryKey: QueryKey, fetch: () => Promise<void>) => {
    const find = () => client.getQueryCache().find({ queryKey, exact: true });
    const before = find()?.state.dataUpdateCount ?? 0;
    void fetch().then(() => {
      const query = find();
      if (query && query.state.dataUpdateCount > before) {
        notePrefetched(query.queryHash, query.state.dataUpdatedAt);
      }
    });
  };
  const day = worklistDayOptions("mine", "all");
  prefetched(day.queryKey, () => client.prefetchInfiniteQuery(day));
  prefetched(morningBriefOptions.queryKey, () =>
    client.prefetchQuery(morningBriefOptions),
  );
  prefetched(magicOptions.queryKey, () => client.prefetchQuery(magicOptions));
  prefetched(handledForYouOptions.queryKey, () =>
    client.prefetchInfiniteQuery(handledForYouOptions),
  );
}
