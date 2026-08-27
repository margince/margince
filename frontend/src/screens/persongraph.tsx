import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { throwProblem } from "./common";

/**
 * usePersonGraph reads the local graph around one contact.
 *
 * Separate from the 360 on purpose: it answers a different question, it is only
 * asked when the reader opens the network, and loading it with the record page
 * would make every person open slower for an answer most opens never need.
 *
 * The panel that used to live here is now `PersonNetworkTab`. Two components
 * reading this one query, each with its own route card, node list and edge
 * detail, were two spellings of one question — and they would have drifted.
 */
export function usePersonGraph(id: string) {
  return useQuery({
    queryKey: ["person-graph", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/graph", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
