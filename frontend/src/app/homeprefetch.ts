// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient, QueryKey } from "@tanstack/react-query";
import { morningBriefOptions } from "../features/brief/brief.queries";
import { addressFrom } from "../features/brief/brief.view";
import { worklistDayOptions } from "../features/worklist/worklist.reads";
import { handledForYouOptions } from "../lib/handled";
import { magicOptions } from "../lib/magic.queries";
import type { UrlParams } from "../lib/urlstate";
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
  const noted = (queryKey: QueryKey) => () => {
    const query = client.getQueryCache().find({ queryKey, exact: true });
    if (query?.state.dataUpdatedAt) {
      notePrefetched(query.queryHash, query.state.dataUpdatedAt);
    }
  };
  const day = worklistDayOptions("mine", "all");
  void client.prefetchInfiniteQuery(day).then(noted(day.queryKey));
  void client
    .prefetchQuery(morningBriefOptions)
    .then(noted(morningBriefOptions.queryKey));
  void client.prefetchQuery(magicOptions).then(noted(magicOptions.queryKey));
  void client
    .prefetchInfiniteQuery(handledForYouOptions)
    .then(noted(handledForYouOptions.queryKey));
}
