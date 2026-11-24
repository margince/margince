import { searchCompanyTargets } from "../../components/companyform";
import { type CreateField, dayOutOfRange } from "../../components/create";
import { amountField, currencyField } from "../../components/currencyfield";
import { partnerFields } from "../../components/dealpartnerfields";
import type { ProjectOwnerPicker } from "../../components/projectownerfield";
import type { LeadingCurrencies } from "../../format/currencychoices";
import type { Locale } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { AcquisitionSource } from "../../lib/acquisitionsources.queries";
import { percentLimit } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { winProbabilityFault } from "../../lib/wholenumber";
import type { Project } from "../projects/projects.form";
import {
  dealCommercialFields,
  MOTION_OPTIONS,
  PRIORITY_OPTIONS,
} from "./deal360/dealcommercialfields";
import { dealProjectFields } from "./dealproject";
import type { Deal } from "./deals";

// The deal's own probability. Empty follows the stage, which is also how a
// probability set on the deal is cleared. A won or lost deal has none to set:
// its stage fixes it, and the server refuses one.
function winProbabilityField(
  t: (k: MessageKey) => string,
  locale: Locale,
): CreateField {
  const label = {
    label: t("deal.winProbability"),
    limit: percentLimit(locale),
  };
  return {
    key: "win_probability",
    label: "deal.winProbability",
    type: "number",
    hint: t("deal.winProbability.hint"),
    validate: (value) => winProbabilityFault(value, locale, label),
  };
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

export const dealCloseDateField = {
  key: "expected_close_date",
  label: "create.expectedClose",
  type: "date",
} satisfies CreateField;

// A date field that refuses a day the server cannot store. Its own check, so
// an edit of the one field in the rail refuses it before sending, as the form does.
function storableDay(
  field: CreateField & { label: MessageKey },
  t: (k: MessageKey) => string,
  locale: Locale,
): CreateField {
  return {
    ...field,
    validate: (value) => dayOutOfRange(value, locale, t(field.label)),
  };
}

/** The deal's close date as every editor of it checks it. */
export function dealCloseDate(
  t: (k: MessageKey) => string,
  locale: Locale,
): CreateField {
  return storableDay(dealCloseDateField, t, locale);
}

export function dealEditFields(
  t: (k: MessageKey, params?: Record<string, string>) => string,
  opts: {
    partnerOptions: { value: string; label: string }[];
    // The partner this deal already names, when it names one. Keeps the field
    // (and its stored value) on a form whose pickable list does not reach it.
    attributedPartner?: { id: string; label: string };
    // The server has said no company is a partner yet.
    noPartnersYet?: boolean;
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
    // Won or lost: the stage fixes the probability.
    closed: boolean;
    me: string;
    currentOwner: string | null;
    currency: string;
    leadingCurrencies: LeadingCurrencies;
    locale: Locale;
  },
): CreateField[] {
  const ownerOptions = [
    ...(opts.currentOwner && opts.currentOwner !== opts.me
      ? [{ value: opts.currentOwner, label: t("deal.ownerKeep") }]
      : []),
    { value: opts.me, label: t("deal.ownerMe") },
    { value: "", label: t("deal.ownerUnassign") },
  ];
  // An inline edit checks one field without the form, so the deal's own
  // currency stands in for the form's.
  const money = {
    currency: (values: Record<string, string>) =>
      values.currency || opts.currency,
    locale: opts.locale,
    t,
    nonNegative: true,
  };
  return [
    {
      key: "name",
      label: "create.dealName",
      required: true,
      maxLength: TEXT_LIMITS.label,
    },
    amountField({ key: "amount", label: "create.amount" }, money),
    // The recurring half of the price, annual, in the same currency as the
    // amount above. Beside it, not in a section of its own: the two are one
    // money value on the row. They share the currency field below, and the
    // server refuses either one stranded without it.
    amountField({ key: "expected_arr", label: "deal.expectedArr" }, money),
    {
      ...currencyField(opts.leadingCurrencies, opts.locale, opts.currency),
      required: false,
      requiredWhen: (values) =>
        Boolean(values.amount?.trim() || values.expected_arr?.trim()),
    },
    {
      key: "owner_id",
      label: "deal.ownerMe",
      type: "select",
      options: ownerOptions,
    },
    companyEditField(t, opts),
    ...partnerFields(t, opts.partnerOptions, {
      attributed: opts.attributedPartner,
      withheld: opts.masked.includes("partner_company_id"),
      noneYet: opts.noPartnersYet,
    }),
    ...(opts.closed ? [] : [winProbabilityField(t, opts.locale)]),
    dealCloseDate(t, opts.locale),
    storableDay(
      { key: "wait_until", label: "deal.waitUntil", type: "date" },
      t,
      opts.locale,
    ),
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
