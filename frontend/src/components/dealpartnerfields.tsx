import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { MessageKey } from "../i18n/en";
import { useCan } from "../lib/capability";
import { useHealthScope } from "../lib/healthscope";
import { unwrap } from "../lib/problem";
import type { CreateField } from "./create";

const ATTRIBUTION_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "", label: "deal.attributionUnset" },
  { value: "sourced", label: "deal.attributionSourced" },
  { value: "influenced", label: "deal.attributionInfluenced" },
];

function usePartners() {
  const readsCompanies = useCan("company", "read");
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["partners", "options", readerScope ?? ""],
    enabled: readsCompanies && readerScope !== undefined,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/partners", {
          params: { query: { limit: 200 } },
        }),
      );
      return data.data;
    },
    staleTime: 60_000,
  });
}

export function useNoPartnersYet(): boolean {
  const partners = usePartners();
  return partners.isSuccess && partners.data.length === 0;
}

export function usePartnerOptions(
  companies: { id: string; display_name: string }[],
): { value: string; label: string }[] {
  const readsCompanies = useCan("company", "read");
  const readerScope = useHealthScope();
  const partners = usePartners();
  const names = useQuery({
    queryKey: ["partners", "names", readerScope ?? ""],
    enabled: readsCompanies && readerScope !== undefined,
    staleTime: 60_000,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/companies", {
          params: { query: { relationship_type: "partner", limit: 200 } },
        }),
      );
      return data.data;
    },
  });
  // A failed names page must not hide partners whose keyed reads still work.
  const byId = usePartnerCompaniesById(
    names.isError ? (partners.data ?? []).map((p) => p.company_id) : [],
  );
  const named = new Map<string, string>(
    companies.map((company) => [company.id, company.display_name]),
  );
  for (const company of [...(names.data ?? []), ...byId]) {
    named.set(company.id, company.display_name);
  }
  return (partners.data ?? []).flatMap((partner) => {
    const label = named.get(partner.company_id);
    return label ? [{ value: partner.company_id, label }] : [];
  });
}

const COMPANY_ID_BATCH = 100;

function usePartnerCompaniesById(
  ids: readonly string[],
): { id: string; display_name: string }[] {
  const readerScope = useHealthScope();
  const batches: string[][] = [];
  for (let at = 0; at < ids.length; at += COMPANY_ID_BATCH) {
    batches.push(ids.slice(at, at + COMPANY_ID_BATCH));
  }
  const reads = useQueries({
    queries: batches.map((batch) => ({
      queryKey: ["partners", "names", "by-id", readerScope ?? "", batch],
      enabled: readerScope !== undefined,
      staleTime: 60_000,
      queryFn: async () =>
        unwrap(
          await api.GET("/companies", {
            params: { query: { id: batch, limit: batch.length } },
          }),
        ).data,
    })),
  });
  return reads.flatMap((read) => read.data ?? []);
}

export function partnerFields(
  t: (k: MessageKey) => string,
  partnerOptions: { value: string; label: string }[],
  state: Readonly<{
    attributed?: { id: string; label: string };
    withheld?: boolean;
    noneYet?: boolean;
  }> = {},
): CreateField[] {
  const { attributed, withheld = false, noneYet = false } = state;
  if (withheld) {
    return [onlyEntry(t("deal.partnerWithheld"))];
  }
  if (partnerOptions.length === 0 && !attributed) {
    return noneYet ? [onlyEntry(t("deal.partnerNoneYet"))] : [];
  }
  // Keep a stored partner even when it falls outside the pickable roster.
  const options =
    attributed && !partnerOptions.some((o) => o.value === attributed.id)
      ? [{ value: attributed.id, label: attributed.label }, ...partnerOptions]
      : partnerOptions;
  return [
    {
      key: "partner_company_id",
      label: "deal.partnerCompany",
      type: "select",
      options,
    },
    {
      key: "partner_attribution",
      label: "deal.partnerAttribution",
      type: "select",
      showWhen: (values) => Boolean(values.partner_company_id),
      options: ATTRIBUTION_OPTIONS.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
  ];
}

function onlyEntry(words: string): CreateField {
  return {
    key: "partner_company_id",
    label: "deal.partnerCompany",
    type: "select",
    options: [{ value: "", label: words }],
  };
}
