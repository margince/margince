import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { searchCompanyTargets } from "../../components/companyform";
import type { CreateField } from "../../components/create";
import type { ProjectOwnerPicker } from "../../components/projectownerfield";
import {
  currencyOptions,
  type LeadingCurrencies,
} from "../../format/currencychoices";
import type { MessageKey } from "../../i18n/en";
import type { AcquisitionSource } from "../../lib/acquisitionsources.queries";
import { unwrap } from "../../lib/problem";
import type { Project } from "../projects/projects.form";
import {
  dealCommercialFields,
  MOTION_OPTIONS,
  PRIORITY_OPTIONS,
} from "./deal360/dealcommercialfields";
import { dealProjectFields } from "./dealproject";
import type { Deal } from "./deals";

const FORECAST_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "commit", label: "deal.fcCommit" },
  { value: "best_case", label: "deal.fcBestCase" },
  { value: "pipeline", label: "deal.fcPipeline" },
  { value: "omitted", label: "deal.fcOmitted" },
];

// What a partner did for the deal. Only "sourced" earns commission: a partner
// who helped a deal we already had is recorded, not paid.
//
// The empty value leads and says what leaving it unset means. The server
// defaults a named partner to "sourced". A bare "Unset" here would let a reader
// believe they had made no claim while the deal became commission-eligible.
// A field that offers the empty value in its own words suppresses the generic
// entry (create.tsx), so this one is spelled out rather than inherited.
const ATTRIBUTION_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "", label: "deal.attributionUnset" },
  { value: "sourced", label: "deal.attributionSourced" },
  { value: "influenced", label: "deal.attributionInfluenced" },
];

/**
 * The companies a deal may name as the partner who brought it.
 *
 * Only companies that are partners. Offering every company would let a deal be
 * attributed to an ordinary customer. An attribution to a company with no
 * partner row has no margin tier behind it: it looks attributed and never earns.
 *
 * An empty list also answers "is there a partner programme here". An
 * installation that never made a company a partner has no partner question to
 * ask. The two fields stay off the form entirely.
 */
export function usePartnerOptions(
  companies: { id: string; display_name: string }[],
): { value: string; label: string }[] {
  const partners = useQuery({
    queryKey: ["partners", "options"],
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
  // The names come from a read of the partner companies rather than from
  // whatever page of companies the screen happens to hold.
  //
  // The caller's `companies` is one capped page (50 on the deals screen).
  // Intersecting the two would drop any partner whose company fell outside it,
  // with no sign, and differently per screen. A form picker survives that,
  // since it injects the deal's own stored partner when it is missing. A
  // filter has no such fallback: a partner it cannot name is one nobody can
  // narrow by.
  const names = useQuery({
    queryKey: ["partners", "names"],
    staleTime: 60_000,
    queryFn: async () => {
      // relationship_type=partner reads the same set from the other side, so
      // the page is partner companies rather than the first N companies of
      // any kind. The cap matches /partners' own.
      const data = unwrap(
        await api.GET("/companies", {
          params: { query: { relationship_type: "partner", limit: 200 } },
        }),
      );
      return data.data;
    },
  });
  // A failed names read falls back to reading the partners' own companies by
  // id. Without it a working /partners answer names nobody, and both partner
  // fields leave the form.
  const byId = usePartnerCompaniesById(
    names.isError ? (partners.data ?? []).map((p) => p.company_id) : [],
  );
  // Still the caller's page as a fallback while the keyed read is in flight,
  // so the options do not blink empty on every mount.
  const named = new Map<string, string>(
    companies.map((company) => [company.id, company.display_name]),
  );
  for (const company of [...(names.data ?? []), ...byId]) {
    named.set(company.id, company.display_name);
  }
  return (partners.data ?? []).flatMap((partner) => {
    const label = named.get(partner.company_id);
    // A partner whose company the caller cannot read is left out, not offered
    // as a bare id. Picking it would name a company the picker could not show.
    return label ? [{ value: partner.company_id, label }] : [];
  });
}

// The id filter takes at most this many ids per request.
const COMPANY_ID_BATCH = 100;

function usePartnerCompaniesById(
  ids: readonly string[],
): { id: string; display_name: string }[] {
  const batches: string[][] = [];
  for (let at = 0; at < ids.length; at += COMPANY_ID_BATCH) {
    batches.push(ids.slice(at, at + COMPANY_ID_BATCH));
  }
  const reads = useQueries({
    queries: batches.map((batch) => ({
      queryKey: ["partners", "names", "by-id", batch],
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

/**
 * The two fields that record a deal coming through a partner. The create form
 * and the edit form share this one spelling, so the two cannot drift.
 *
 * Nothing at all when the installation has no partners and the deal names
 * none. An empty picker asks a question with no answers in it. A form that
 * shows partner controls to a company running no partner programme claims a
 * feature it does not have.
 *
 * `attributed` is what keeps that from eating data. A form omits nothing a
 * record already carries. The pickable list is one capped page of companies,
 * so a deal's own partner can be missing from it. Dropping the field would
 * blank the stored partner on the next edit of anything else on the deal, and
 * the commission attribution with it.
 *
 * The attribution follows the partner rather than standing beside it. What a
 * partner did is a question about a partner, so it appears once one is named
 * and disappears when the choice is cleared. Asking it first offers a claim
 * with nobody to attach it to.
 */
export function partnerFields(
  t: (k: MessageKey) => string,
  partnerOptions: { value: string; label: string }[],
  attributed?: { id: string; label: string },
  // The reader was not shown this deal's partner (`masked_fields` names it).
  // Checked before the empty-list exit. A withheld partner is a fact about
  // this deal, so it is stated whether or not the installation has partners
  // left to offer.
  withheld = false,
): CreateField[] {
  if (withheld) {
    // One entry, and it names the field as withheld. A picker seeded blank
    // over a partner the reader never saw invites them to choose a different
    // one. The attribution travels with the partner, so there is nothing here
    // to attribute either. `mapDealUpdate` leaves both out of the patch.
    return [
      {
        key: "partner_company_id",
        label: "deal.partnerCompany",
        type: "select",
        options: [{ value: "", label: t("deal.partnerWithheld") }],
      },
    ];
  }
  if (partnerOptions.length === 0 && !attributed) {
    return [];
  }
  // The deal's own partner is always among the choices, even when the capped
  // page of partners this form could offer does not include it. A select whose
  // stored value is not an option shows blank, and saving that blank clears the
  // partner nobody meant to touch.
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
    // Commission accrues on "sourced" only, so this is the field that decides
    // whether a win pays the partner.
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

/**
 * The partner a deal already names, ready to stand as its own option.
 *
 * Falls back to the raw id while the name is unknown (still loading, or an
 * unreadable company). Ugly, and correct: the alternative is a blank select
 * whose save clears an attribution the reader never touched.
 */
export function attributedPartner(
  deal: Deal,
  name: string | null,
): { id: string; label: string } | undefined {
  if (!deal.partner_company_id) {
    return undefined;
  }
  return {
    id: deal.partner_company_id,
    label: name ?? deal.partner_company_id,
  };
}

/**
 * The company picker on a deal's edit form, in the three readings a stored
 * company has.
 *
 * Withheld is the reading that offers no company at all. The reader was not
 * shown which company this is. A picker seeded blank invites them to pick one
 * and re-point the deal away from the company a colleague linked. So its single
 * entry names the field as withheld, and `mapDealUpdate` leaves it out of the patch.
 *
 * Otherwise the company is searched, since no page a form could load up front
 * reaches every company. The stored company rides along as the one option, so
 * the picker names it rather than showing its id.
 */
function companyEditField(
  t: (k: MessageKey) => string,
  opts: {
    masked: readonly string[];
    currentCompany?: { id: string; label: string };
  },
): CreateField {
  if (opts.masked.includes("company_id")) {
    return {
      key: "company_id",
      label: "create.relatedCompany",
      type: "select",
      options: [{ value: "", label: t("deal.companyWithheld") }],
    };
  }
  const current = opts.currentCompany;
  return {
    key: "company_id",
    label: "create.relatedCompany",
    searchTargets: searchCompanyTargets,
    options: current ? [{ value: current.id, label: current.label }] : [],
  };
}

export function dealEditFields(
  t: (k: MessageKey) => string,
  opts: {
    partnerOptions: { value: string; label: string }[];
    // The partner this deal already names, when it names one. Keeps the field
    // (and its stored value) on a form whose pickable list does not reach it.
    attributedPartner?: { id: string; label: string };
    // The company this deal already names, so the searched picker can name it.
    currentCompany?: { id: string; label: string };
    // The administered acquisition-source catalog, and the key this deal
    // already carries. That key may name a retired entry the picker must still
    // offer, or the next save clears a value nobody touched.
    acquisitionSources?: AcquisitionSource[];
    currentAcquisitionSource?: string | null;
    // The fields of this deal the reader was not shown, as `masked_fields`
    // named them. A withheld reference is offered as withheld rather than as
    // an empty picker.
    masked: readonly string[];
    me: string;
    currentOwner: string | null;
    currency: string;
    leadingCurrencies: LeadingCurrencies;
  },
): CreateField[] {
  const ownerOptions = [
    ...(opts.currentOwner && opts.currentOwner !== opts.me
      ? [{ value: opts.currentOwner, label: t("deal.ownerKeep") }]
      : []),
    { value: opts.me, label: t("deal.ownerMe") },
    { value: "", label: t("deal.ownerUnassign") },
  ];
  return [
    { key: "name", label: "create.dealName", required: true },
    { key: "amount", label: "create.amount", type: "number", step: "any" },
    // The recurring half of the price, annual, in the same currency as the
    // amount above. Beside it, not in a section of its own: the two are one
    // money value on the row. They share the currency field below, and the
    // server refuses either one stranded without it.
    {
      key: "expected_arr",
      label: "deal.expectedArr",
      type: "number",
      step: "any",
    },
    {
      key: "currency",
      label: "create.currency",
      type: "select",
      required: true,
      options: currencyOptions(opts.leadingCurrencies, opts.currency),
    },
    {
      key: "owner_id",
      label: "deal.ownerMe",
      type: "select",
      options: ownerOptions,
    },
    companyEditField(t, opts),
    // Both partner fields are absent where no company has been made a partner.
    // There is no partner programme to attribute anything to, and an empty
    // picker is a question the reader cannot answer.
    ...partnerFields(
      t,
      opts.partnerOptions,
      opts.attributedPartner,
      opts.masked.includes("partner_company_id"),
    ),
    {
      key: "forecast_category",
      label: "deal.forecastCategory",
      type: "select",
      options: FORECAST_OPTIONS.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
    { key: "expected_close_date", label: "create.expectedClose", type: "date" },
    { key: "wait_until", label: "deal.waitUntil", type: "date" },
    {
      key: "__commercial__",
      labelText: t("deal.commercialContext"),
      divider: true,
    },
    ...dealCommercialFields(t, {
      motionOptions: MOTION_OPTIONS,
      priorityOptions: PRIORITY_OPTIONS,
      sources: opts.acquisitionSources ?? [],
      currentSource: opts.currentAcquisitionSource,
    }),
  ];
}

export function editProjectFields(
  t: (key: MessageKey) => string,
  opts: Readonly<{
    masked: readonly string[];
    openProjects: readonly Project[];
    currentProject?: { id: string; label: string };
    company?: string;
    owner?: ProjectOwnerPicker;
  }>,
): CreateField[] {
  if (opts.masked.includes("project_id")) {
    // One entry, naming the field as withheld, as the company and partner
    // pickers do. A form with no project row reads as a deal on no project,
    // not one this reader was not shown. `mapDealUpdate` leaves the field out
    // of the patch. Neither a project to pick nor "start a new one" is offered:
    // both would re-point the deal off a project the reader never saw.
    return [
      {
        key: "project_id",
        label: "deal.project",
        type: "select",
        options: [{ value: "", label: t("deal.projectWithheld") }],
      },
    ];
  }
  return dealProjectFields(
    t,
    opts.openProjects,
    opts.currentProject,
    opts.company,
    opts.owner,
  );
}
