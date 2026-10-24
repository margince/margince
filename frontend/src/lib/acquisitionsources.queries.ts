import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./problem";

// The acquisition-source catalog: the business channels a deal may be
// attributed to. Every deal surface — the edit form, the list filter, the
// settings card — reads it through here, so they cannot disagree about what
// the list holds or which entries are still choosable.
//
// This is a separate list from the lead origins in leadorigins.ts. A lead origin
// says where a lead came from and carries the scorer's intent weights. This one
// is the channel a deal is attributed to, with no weight. A shared hook would
// make it easy to ask the wrong one.

export type AcquisitionSource = components["schemas"]["AcquisitionSource"];

export const ACQUISITION_SOURCES_KEY = ["acquisition-sources"] as const;

export function useAcquisitionSources() {
  return useQuery({
    queryKey: ACQUISITION_SOURCES_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/acquisition-sources");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

/** The label a stored key renders as, falling back to the key itself. */
export function acquisitionLabel(
  key: string | null | undefined,
  sources: AcquisitionSource[] | undefined,
): string | undefined {
  if (!key) {
    return undefined;
  }
  return sources?.find((source) => source.key === key)?.label ?? key;
}

/**
 * The filter chip's options: every entry the list can still be narrowed by.
 *
 * Retired entries STAY, because deals still carry them — a filter that drops
 * them makes those deals unreachable through the one control that would find
 * them. The retired ones are labelled so the reader knows why a channel they
 * cannot choose on a form is offered here.
 */
export function acquisitionFilterOptions(
  sources: AcquisitionSource[] | undefined,
  retiredSuffix: string,
): { value: string; text: string }[] {
  return (sources ?? []).map((source) => ({
    value: source.key,
    text: source.active ? source.label : `${source.label} ${retiredSuffix}`,
  }));
}

export function useCreateAcquisitionSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAcquisitionSourceRequest"],
    ) => {
      const { data, error, response } = await api.POST("/acquisition-sources", {
        body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ACQUISITION_SOURCES_KEY }),
  });
}

export function useUpdateAcquisitionSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      id: string;
      body: components["schemas"]["UpdateAcquisitionSourceRequest"];
    }) => {
      const { data, error, response } = await api.PATCH(
        "/acquisition-sources/{id}",
        { params: { path: { id: args.id } }, body: args.body },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ACQUISITION_SOURCES_KEY }),
  });
}
