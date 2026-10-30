import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./problem";
import { shippedLabel, withShippedLabels } from "./shippedlabels";

// The installation's partner ladder. The Partner tab's stage picker, the
// partner and company list filters and the settings card all read it here.

export type PartnerStage = components["schemas"]["PartnerStage"];
export type PartnerStageMeaning = components["schemas"]["PartnerStageMeaning"];
export type PartnerKind = components["schemas"]["PartnerKind"];

export const PARTNER_STAGES_KEY = ["partner-stages"] as const;

export const MEANING_LABELS: Record<PartnerStageMeaning, MessageKey> = {
  candidate: "partnerStages.meaning.candidate",
  signed: "partnerStages.meaning.signed",
  ready: "partnerStages.meaning.ready",
  active: "partnerStages.meaning.active",
  dormant: "partnerStages.meaning.dormant",
  ended: "partnerStages.meaning.ended",
};

export const KIND_LABELS: Record<PartnerKind, MessageKey> = {
  hosting: "partner.kind.hosting",
  consulting: "partner.kind.consulting",
  strategic: "partner.kind.strategic",
  referral: "partner.kind.referral",
};

// The ordered lists come from the label maps, which the generated contract
// types hold complete: a value the contract adds and a map lacks fails the
// build, and backend/gates/partnervocabulary_test.go holds the maps both ways.
function keysOf<K extends string>(record: Record<K, unknown>): K[] {
  return Object.keys(record).filter((key): key is K => key in record);
}

export const PARTNER_KINDS: readonly PartnerKind[] = keysOf(KIND_LABELS);

export const PARTNER_STAGE_MEANINGS: readonly PartnerStageMeaning[] =
  keysOf(MEANING_LABELS);

export function usePartnerStages(enabled = true) {
  const t = useT();
  return useQuery({
    queryKey: PARTNER_STAGES_KEY,
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/partner-stages");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (stages) => withShippedLabels(stages, t),
  });
}

/**
 * The stages a partner may move to: the active ones in ladder order, plus the
 * stage it stands on when that one was retired, so the picker can show it.
 */
export function assignableStages(
  stages: readonly PartnerStage[] | undefined,
  current: string | undefined,
): PartnerStage[] {
  return (stages ?? []).filter(
    (stage) => stage.active || stage.key === current,
  );
}

/** The words a partner's stage reads as, in the reader's language. */
export function partnerStageLabel(
  partner: Readonly<{
    relationship_stage?: string;
    stage_label?: string;
    stage_label_key?: string | null;
  }>,
  t: Translator,
): string {
  const label = partner.stage_label ?? partner.relationship_stage ?? "";
  return shippedLabel({ label, label_key: partner.stage_label_key }, t);
}

/** A partner's kinds as one phrase, in ladder-independent order. */
export function partnerKindsLabel(
  kinds: readonly PartnerKind[] | undefined,
  t: Translator,
): string {
  return PARTNER_KINDS.filter((kind) => kinds?.includes(kind))
    .map((kind) => t(KIND_LABELS[kind]))
    .join(", ");
}

function onStagesChanged(qc: ReturnType<typeof useQueryClient>) {
  // A rename or a re-meaning changes how every partner reads, and a company
  // carries its partner's stage label in its marks and list row.
  for (const queryKey of [
    PARTNER_STAGES_KEY,
    ["partners"],
    ["partner"],
    ["companies"],
    ["company"],
    ["company360"],
  ]) {
    void qc.invalidateQueries({ queryKey });
  }
}

export function useCreatePartnerStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreatePartnerStageRequest"],
    ) => {
      const { data, error, response } = await api.POST("/partner-stages", {
        body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => onStagesChanged(qc),
  });
}

export function useUpdatePartnerStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (args: {
      id: string;
      body: components["schemas"]["UpdatePartnerStageRequest"];
    }) => patchPartnerStage(args.id, args.body),
    onSuccess: () => onStagesChanged(qc),
  });
}

export function useDeletePartnerStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/partner-stages/{id}", {
        params: { path: { id } },
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSuccess: () => onStagesChanged(qc),
  });
}

async function patchPartnerStage(
  id: string,
  body: components["schemas"]["UpdatePartnerStageRequest"],
) {
  const { data, error, response } = await api.PATCH("/partner-stages/{id}", {
    params: { path: { id } },
    body,
  });
  if (error || !response.ok) {
    throwProblem(error);
  }
  return data;
}

/** Moves a stage one place along the ladder by swapping its position with a neighbour's. */
export function useSwapPartnerStages() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (pair: {
      moving: PartnerStage;
      neighbour: PartnerStage;
    }) => {
      const { moving, neighbour } = pair;
      // Equal positions would swap into the same order, so the mover takes a
      // distinct one.
      const target =
        moving.sort_order === neighbour.sort_order
          ? neighbour.sort_order + (moving.sort_order > 0 ? -1 : 1)
          : neighbour.sort_order;
      await patchPartnerStage(neighbour.id, { sort_order: moving.sort_order });
      await patchPartnerStage(moving.id, { sort_order: target });
    },
    onSuccess: () => onStagesChanged(qc),
  });
}

/**
 * The partner companies a record may name, as picker options. It reads the
 * companies carrying the partner relationship type, which holds exactly the
 * live programmes, under the key the deal form's partner picker shares.
 */
export function usePartnerCompanyOptions(enabled = true) {
  return useQuery({
    queryKey: ["partners", "names"],
    enabled,
    staleTime: 60_000,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/companies", {
        params: { query: { relationship_type: "partner", limit: 200 } },
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (companies) =>
      companies.map((company) => ({
        value: company.id,
        label: company.display_name,
      })),
  });
}
