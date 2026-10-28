// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { handledForYouOptions } from "../../lib/handled";
import type { UrlParams } from "../../lib/urlstate";
import { magicOptions } from "../sign-in/magic.queries";
import { worklistDayOptions } from "../worklist/worklist.reads";
import { morningBriefOptions } from "./brief.queries";
import { addressFrom } from "./brief.view";

// Home's morning reads, started as soon as the shell may draw (App.tsx). The
// screen commits only after React's Suspense reveal, and a query starts when
// its screen commits, so without this the reads wait out that delay too. Each
// read uses its hook's own options, so the screen joins the request in flight.
export function prefetchHome(client: QueryClient, params: UrlParams): void {
  if (addressFrom(params).view !== "morning") {
    return;
  }
  void client.prefetchInfiniteQuery(worklistDayOptions("mine", "all"));
  void client.prefetchQuery(morningBriefOptions);
  void client.prefetchQuery(magicOptions);
  void client.prefetchInfiniteQuery(handledForYouOptions);
}
