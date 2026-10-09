import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./common";
import { worklistKey } from "./worklist.queries";

// The reads behind the hidden-backlog panel: the figures, and the messages
// behind one figure.

export type HiddenBacklog = components["schemas"]["HiddenBacklog"];
export type HiddenBacklogRows = components["schemas"]["HiddenBacklogRows"];
export type HiddenRule = HiddenBacklogRows["rule"];

// What the queue is NOT showing, and which rule holds it back.
//
// Read on its own key rather than folded into the day: it is five SQL reads
// where the queue is one, and a rep opening the Worklist should not wait on a
// diagnostic to see their work. A slow or failing guardrail must cost the queue
// nothing, which is what a separate query buys.
//
// `enabled` carries the same tier the team board's does. The figures are counted
// under the caller's own visibility either way, so this is not what makes them
// safe — it is what keeps a surface the reader has no route to from firing a
// request behind their back.
export function useHiddenBacklog(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: [...worklistKey, "hidden"],
    queryFn: async (): Promise<HiddenBacklog> => {
      return unwrap(await api.GET("/worklist/hidden", {}));
    },
  });
}

// The messages one hiding rule holds back, read only once a reader opens the
// figure: the counts ride every worklist load, the rows are asked for.
export function useHiddenBacklogRows(rule: HiddenRule, enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: [...worklistKey, "hidden", rule],
    queryFn: async (): Promise<HiddenBacklogRows> => {
      return unwrap(
        await api.GET("/worklist/hidden/{rule}", {
          params: { path: { rule } },
        }),
      );
    },
  });
}
