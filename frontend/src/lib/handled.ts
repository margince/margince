// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { worklistKey } from "./workkeys";

export type HandledForYou = components["schemas"]["HandledForYou"];

export type Receipt = components["schemas"]["Receipt"];

/**
 * What the product did on this reader's behalf, one page or more.
 *
 * No `enabled` gate, unlike the manager reads beside it. A receipt is the
 * reader's own and every act it reports was taken for them, so there is no tier
 * to hold. The server refuses a caller with no human behind it, which is the
 * only refusal this read has.
 *
 * Every page read so far is one list. A receipt past the first page joins the
 * same population, so a grouped record found there keeps its review verbs.
 */
export function useHandledForYou() {
  return useInfiniteQuery({
    queryKey: [...worklistKey, "handled"],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }): Promise<HandledForYou> => {
      return unwrap(
        await api.GET("/worklist/handled", {
          params: { query: pageParam ? { cursor: pageParam } : {} },
        }),
      );
    },
    getNextPageParam: (last) => last.next_cursor,
    select: (read): HandledForYou => joinedPages(read.pages),
  });
}

// The pages as one answer: the first page's instant and window, every receipt
// in order, and the last page's word on whether more remains.
function joinedPages(pages: readonly HandledForYou[]): HandledForYou {
  const first = pages[0];
  const last = pages.at(-1);
  if (!first || !last) {
    return { as_of: "", receipts: [], truncated: false };
  }
  // One page is the server's own answer, a missing list included.
  if (first === last) {
    return first;
  }
  return {
    ...first,
    receipts: pages.flatMap((page) => page.receipts ?? []),
    truncated: last.truncated,
    next_cursor: last.next_cursor,
  };
}
