import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

// The account's one-hop connection graph, as data.
//
// What is left of the connections card after the People tab stopped drawing
// one. The card, its expanded modal and the radial diagram all answered "who
// works at this account" a second and third time beside a roster that already
// said so, in a picture of unlabeled dots that was hidden from screen readers
// by its own design. The coverage comparison still needs the EDGES, so the read
// outlives the drawing.

/** useOrganizationGraph reads the account's one-hop connections. */
export function useOrganizationGraph(id: string, enabled = true) {
  return useQuery({
    enabled,
    // The coverage comparison is opened, closed and reopened while a reader
    // works through an account; a fresh-enough copy serves those as one read.
    // A write that changes the graph (setting a buying role) must invalidate
    // this key explicitly — the minute of freshness would otherwise outlive
    // the save the user is looking at.
    staleTime: 60_000,
    queryKey: ["organization-graph", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}/graph", {
        params: { path: { id } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * StrengthBucket is the graph's own band vocabulary.
 *
 * NOT the three bands a route is shown in (`strong | developing | cold`): this
 * is the four the relationship score itself supports, and the two lists exist
 * because a reader deciding whom to ask needs a coarser answer than the score
 * carries. Casting one to the other is how values no client can render reach
 * the wire.
 */
export type StrengthBucket = "none" | "weak" | "moderate" | "strong";
