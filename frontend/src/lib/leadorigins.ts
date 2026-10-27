import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { importLabel } from "../design-system/provenance";
import { importProvenance } from "../design-system/provenanceof";
import { sourceSystemName } from "../format/sourcesystem";
import { useT } from "../i18n";
import { LEAD_LIST_KEY } from "./leadkeys";
import { throwProblem, unwrap } from "./problem";
import { withShippedLabels } from "./shippedlabels";

// A lead answers two questions, and each has one reader here.
//
// Origin says where the lead came from: a key from the administered list,
// beside a short note. A human, an importer or capture sets it, and it is the
// only lead fact the score and the origin filter read.
//
// Added via says how the row reached Margince: typed in, an import, a
// connector. The server writes it, it never scores, and it is read off the
// lead's `source` and `imported_from`.

export type LeadOrigin = components["schemas"]["LeadOrigin"];
export type LeadOriginIntent = components["schemas"]["LeadSourceIntent"];

export const LEAD_ORIGINS_KEY = ["lead-origins"] as const;

export function useLeadOrigins() {
  const t = useT();
  return useQuery({
    queryKey: LEAD_ORIGINS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/lead-origins");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (origins) => withShippedLabels(origins, t),
  });
}

// A payload that is not the contract's array is treated as empty. So is a row
// without a key. Neither crashes the render that reads it.
function listed(origins: readonly LeadOrigin[] | undefined): LeadOrigin[] {
  if (!Array.isArray(origins)) return [];
  return origins.filter(
    (o) => typeof o?.key === "string" && typeof o?.label === "string",
  );
}

/**
 * The words a lead's origin reads as. The list's label comes first, because
 * it is in the reader's language; the server's `origin_label` is the stored
 * English one.
 */
export function originLabelFor(
  lead: Readonly<{ origin?: string | null; origin_label?: string | null }>,
  origins: readonly LeadOrigin[] | undefined,
): string | undefined {
  if (!lead.origin) return undefined;
  return (
    listed(origins).find((o) => o.key === lead.origin)?.label ??
    lead.origin_label ??
    lead.origin
  );
}

/**
 * What a human may pick for a lead's origin: the active entries in the
 * administrator's order. A lead already carrying a retired key keeps it as an
 * option, so the control never shows a value it cannot name.
 */
export function originPickOptions(
  origins: readonly LeadOrigin[] | undefined,
  current: string | null | undefined,
): { value: string; label: string }[] {
  const all = listed(origins);
  const options = all
    .filter((o) => o.active)
    .map((o) => ({ value: o.key, label: o.label }));
  if (current && !options.some((o) => o.value === current)) {
    const held = all.find((o) => o.key === current);
    options.unshift({ value: current, label: held?.label ?? current });
  }
  return options;
}

/**
 * The filter chip's options. Retired entries stay, labelled, because leads
 * still carry them and the chip is the control that finds those leads.
 */
export function originFilterOptions(
  origins: readonly LeadOrigin[] | undefined,
  retiredSuffix: string,
): { value: string; text: string }[] {
  return listed(origins).map((o) => ({
    value: o.key,
    text: o.active ? o.label : `${o.label} ${retiredSuffix}`,
  }));
}

// The shipped values a row's `source` takes when nothing imported it. A
// connector's value names the connector.
const WRITTEN_BY: Readonly<
  Record<string, Parameters<ReturnType<typeof useT>>[0]>
> = {
  manual: "lead.source.manual",
  inbound: "lead.source.inbound",
  webform: "lead.source.webform",
  referral: "lead.source.referral",
  import: "lead.source.import",
  crawl: "lead.source.crawl",
  siteread: "lead.source.siteread",
};

/**
 * How the row got into Margince, in words. An import reads "Imported from
 * <upload>", a connector reads as its name, and a typed lead reads "Created
 * manually". The raw `connector:apollo:a-1` token and an import's stamp never
 * show.
 */
export function addedViaFor(
  lead: Readonly<{ source?: string | null; imported_from?: string | null }>,
  t: ReturnType<typeof useT>,
): string {
  if (lead.imported_from !== null && lead.imported_from !== undefined) {
    return importLabel(
      { kind: "import", uploadName: lead.imported_from || undefined },
      t,
    );
  }
  return writtenByLabel(lead.source, t);
}

/**
 * A stored `source` value in words, for a reader that has only the value (a
 * report's dimension). An import's stamp reads as an import, never as the
 * stamp.
 */
export function writtenByLabel(
  source: string | null | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (!source) return t("lead.shortfall.noSource");
  const imported = importProvenance(source);
  if (imported) return importLabel(imported, t);
  const shipped =
    WRITTEN_BY[source] ?? WRITTEN_BY[source.replace(/[_-]+/g, "")];
  if (shipped) return t(shipped);
  const parts = source.split(":").filter(Boolean);
  const meaningful = parts[0] === "connector" ? parts[1] : parts[0];
  const named = meaningful && sourceSystemName(meaningful);
  if (named && named !== meaningful) return named;
  const spoken = meaningful?.replace(/[_-]+/g, " ");
  return spoken
    ? spoken.charAt(0).toUpperCase() + spoken.slice(1)
    : t("lead.source.unknown");
}

export type OriginPatch = {
  id: string;
  label?: string;
  intent?: LeadOriginIntent;
  active?: boolean;
};
type NewOrigin = { label: string; key?: string; intent: LeadOriginIntent };

async function createOrigin(body: NewOrigin) {
  return unwrap(await api.POST("/lead-origins", { body }));
}

async function patchOrigin({ id, ...body }: OriginPatch) {
  return unwrap(
    await api.PATCH("/lead-origins/{id}", { params: { path: { id } }, body }),
  );
}

// One mutation per write, so a refusal speaks where it was asked.
export function useOriginMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: LEAD_ORIGINS_KEY });
    // The leads list and its create form render labels off this list.
    void queryClient.invalidateQueries({ queryKey: LEAD_LIST_KEY });
  };
  const create = useMutation({
    mutationFn: createOrigin,
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: patchOrigin,
    onSuccess: invalidate,
  });
  const rename = useMutation({
    mutationFn: patchOrigin,
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/lead-origins/{id}", { params: { path: { id } } }),
      );
    },
    onSuccess: invalidate,
  });
  return { create, update, rename, remove };
}
