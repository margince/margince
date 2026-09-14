import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { throwProblem } from "./common";

/**
 * useContactGraph reads the local graph around one contact.
 *
 * Separate from the 360 on purpose: it answers a different question, it is only
 * asked when the reader opens the network, and loading it with the record page
 * would make every contact open slower for an answer most opens never need.
 *
 * The panel that used to live here is now `ContactNetworkTab`. Two components
 * reading this one query, each with its own route card, node list and edge
 * detail, were two spellings of one question — and they would have drifted.
 */
export function useContactGraph(id: string) {
  return useQuery({
    queryKey: ["contact-graph", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}/graph", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
