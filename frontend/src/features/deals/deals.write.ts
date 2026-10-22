import type { components } from "../../api/schema";
import { toMajorUnits, toMinorUnits } from "../../format/minorunits";
import { currencyBeside } from "../../lib/dealcurrency";
import { commercialMotion, dealPriority } from "./deal360/dealcommercialfields";
import type { Deal } from "./deals";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];

type CreateDealRequest = components["schemas"]["CreateDealRequest"];

// Missing money stays blank; present figures use their currency's minor-unit scale.
// This projection seeds both the controls and their update comparison.
function moneyFieldValue(
  minor: number | null | undefined,
  currency?: string | null,
): string {
  return minor != null && currency ? String(toMajorUnits(minor, currency)) : "";
}

export function dealEditRecord(deal: Deal): Record<
  string,
  string | number | undefined
> & {
  id: string;
  version?: number;
} {
  return {
    id: deal.id,
    version: deal.version,
    name: deal.name,
    amount: moneyFieldValue(deal.amount_minor, deal.currency),
    expected_arr: moneyFieldValue(deal.expected_arr_minor, deal.currency),
    currency: deal.currency ?? "",
    owner_id: deal.owner_id ?? "",
    company_id: deal.company_id ?? "",
    partner_company_id: deal.partner_company_id ?? "",
    partner_attribution: deal.partner_attribution ?? "",
    forecast_category: deal.forecast_category ?? "",
    expected_close_date: deal.expected_close_date ?? "",
    wait_until: deal.wait_until ?? "",
    project_id: deal.project_id ?? "",
    description: deal.description ?? "",
    commercial_motion: deal.commercial_motion ?? "",
    priority: deal.priority ?? "",
    acquisition_source: deal.acquisition_source ?? "",
  };
}

// The edit form's values are typed `unknown` per key; the project resolver
// reads the three it needs as strings, and anything else is not one.
export function stringValues(
  values: Record<string, unknown>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(values)) {
    if (typeof value === "string") {
      out[key] = value;
    }
  }
  return out;
}

// The attribution the form may send, narrowed as the forecast category is. The
// wire type is a closed vocabulary, and a free string from a form input is not
// one until something checks it.
function partnerAttribution(
  v: string,
): UpdateDealRequest["partner_attribution"] {
  switch (v) {
    case "sourced":
      return "sourced";
    case "influenced":
      return "influenced";
    default:
      return null;
  }
}

function forecastCategory(v: string): UpdateDealRequest["forecast_category"] {
  switch (v) {
    case "commit":
      return "commit";
    case "best_case":
      return "best_case";
    case "pipeline":
      return "pipeline";
    case "omitted":
      return "omitted";
    default:
      return null;
  }
}

/**
 * The edit form's values as a deal patch: a diff, not a snapshot.
 *
 * `seeded` is the record the form opened on (`dealEditRecord`). A key appears
 * only where the form's value differs from it. A full body would resubmit
 * `null` for a value the form does not render (`partner_attribution` with no
 * partners). The API reads an explicit null as "forget this" and refuses,
 * naming a field the reader never saw. Nothing the reader did not touch travels.
 *
 * A blank over a stored value is still a change, and still travels as null. The
 * pickers offer "Unset" in words, so clearing a company or partner is a choice
 * somebody made, not a gap in the form.
 *
 * `amount` arrives in major units from the form and the wire is minor units
 * (deal creation applies the same conversion above). The two money halves are
 * compared as the form spells them, because that is where the reader's edit is.
 *
 * `masked` names the fields this reader was not shown. A withheld reference
 * arrives as null with `masked_fields` naming it, so a reader can tell "you may
 * not see this" from "there is nothing here". The form has only the null.
 * Sending it back would ask the server to clear a partner the reader never saw.
 * So a masked field is left out of the patch: omitted means unchanged.
 */
export function mapDealUpdate(
  values: Record<string, unknown>,
  seeded: Record<string, unknown> = {},
  masked: readonly string[] = [],
): UpdateDealRequest {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const currency = str(values.currency);
  const amount = str(values.amount);
  const expectedArr = str(values.expected_arr);
  const patch: UpdateDealRequest = {};
  // One reading of "the reader moved this field", over the form's spelling of
  // the value rather than the wire's. A select left alone holds the string it
  // was seeded with. The wire shape (minor units, a narrowed vocabulary) is
  // derived from that string afterwards.
  //
  // The wire key and the form key are named together at every call. The one
  // way this goes wrong is a field compared under one name and sent under
  // another. That sends an untouched field or swallows a real edit.
  const moved = (formKey: string) =>
    str(values[formKey]) !== str(seeded[formKey]);
  function onMove<K extends keyof UpdateDealRequest>(
    formKey: string,
    wireKey: K,
    value: () => UpdateDealRequest[K],
  ) {
    if (moved(formKey)) {
      patch[wireKey] = value();
    }
  }
  // A required field: an emptied name is a form the reader has not finished,
  // not an instruction to erase the deal's name.
  onMove("name", "name", () => str(values.name) || undefined);
  // The money pair travels together whenever either half moves, because the
  // scale is part of the amount. amount_minor counts the currency's own minor
  // units (a dong has none, a dinar has three). A currency moving alone
  // re-denominates the stored figure: 10000 JPY re-saved as EUR becomes 10000
  // minor EUR (€100). The FX freeze and the forecast history record that price.
  //
  // The exception is a deal with no figure at all. Naming a currency there is
  // half a money value. The server's own pair refusal says so better than an
  // amount_minor this form would have to invent.
  if (moved("amount") || (moved("currency") && amount)) {
    patch.amount_minor = amount ? toMinorUnits(Number(amount), currency) : null;
  }
  // ARR carries its currency's scale and travels on a currency move, as the
  // amount does. The code is what says how many minor units the figure counts in.
  if (moved("expected_arr") || (moved("currency") && expectedArr)) {
    patch.expected_arr_minor = expectedArr
      ? toMinorUnits(Number(expectedArr), currency)
      : null;
  }
  onMove("currency", "currency", () => currency || undefined);
  onMove("company_id", "company_id", () => str(values.company_id) || null);
  onMove("owner_id", "owner_id", () => str(values.owner_id) || null);
  onMove(
    "partner_company_id",
    "partner_company_id",
    () => str(values.partner_company_id) || null,
  );
  // Only alongside a partner. Clearing the partner clears what they did with it:
  // the two are one fact, stored under one CHECK. Sending the attribution's own
  // null beside it would name a claim with nobody to credit, which the API refuses.
  if (str(values.partner_company_id)) {
    onMove("partner_attribution", "partner_attribution", () =>
      partnerAttribution(str(values.partner_attribution)),
    );
  }
  onMove("forecast_category", "forecast_category", () =>
    forecastCategory(str(values.forecast_category)),
  );
  onMove(
    "expected_close_date",
    "expected_close_date",
    () => str(values.expected_close_date) || null,
  );
  onMove("wait_until", "wait_until", () => str(values.wait_until) || null);
  // Resolved by the screen before mapping: the "new project" answer has
  // already become an id by the time the patch is built.
  onMove("project_id", "project_id", () => str(values.project_id) || null);
  if (values.description !== seeded.description)
    patch.description =
      typeof values.description === "string" && values.description.trim()
        ? values.description
        : null;
  onMove("commercial_motion", "commercial_motion", () =>
    commercialMotion(str(values.commercial_motion)),
  );
  onMove("priority", "priority", () => dealPriority(str(values.priority)));
  onMove(
    "acquisition_source",
    "acquisition_source",
    () => str(values.acquisition_source) || null,
  );
  return withoutMasked(patch, masked);
}

/**
 * The patch minus every field the reader was not shown.
 *
 * `partner_company_id` carries its attribution: the server withholds the pair
 * together, so returning one half of it would decide what a partner nobody
 * could see is owed.
 */
function withoutMasked(
  patch: UpdateDealRequest,
  masked: readonly string[],
): UpdateDealRequest {
  if (masked.length === 0) {
    return patch;
  }
  const out: Record<string, unknown> = { ...patch };
  for (const field of masked) {
    delete out[field];
    if (field === "partner_company_id") {
      delete out.partner_attribution;
    }
    if (field === "amount_minor" || field === "expected_arr_minor") {
      // The server withholds the whole money reading as one unit. A patch that
      // kept either figure would write back a half it was never shown.
      delete out.currency;
      delete out.amount_minor;
      delete out.expected_arr_minor;
    }
  }
  return out as UpdateDealRequest;
}

/**
 * The create form's values as the deal-birth body.
 *
 * A deal names its partner at birth rather than only through a later edit. The
 * win that pays the partner can land before anybody revisits the record, and
 * commission accrues on a `sourced` attribution alone. Both partner fields
 * travel here, as in the update body. A create that dropped them would report
 * success while the partner was gone.
 */
export function mapDealCreate(
  values: Record<string, unknown>,
  pipelineId: string,
  // The first code the currency picker offered, sent when the field is empty.
  defaultCurrency: string,
): CreateDealRequest {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const amount = str(values.amount);
  const expectedArr = str(values.expected_arr);
  const currency = str(values.currency) || defaultCurrency;
  return {
    name: str(values.name),
    pipeline_id: pipelineId,
    stage_id: str(values.stage_id),
    // The UI takes major units; the wire is minor units, at the scale this
    // currency carries. A dong has none, so a fixed factor of a hundred would
    // price the deal a hundredfold, and nothing downstream could tell.
    amount_minor: amount ? toMinorUnits(Number(amount), currency) : null,
    expected_arr_minor: expectedArr
      ? toMinorUnits(Number(expectedArr), currency)
      : null,
    currency: currencyBeside(currency, amount, expectedArr),
    company_id: str(values.company_id) || null,
    partner_company_id: str(values.partner_company_id) || null,
    // The empty option means the caller made no claim, and null is how that
    // travels. The server then reads a named partner as `sourced`, as the
    // option says. An attribution naming no partner is refused 422 rather than
    // defaulted, since there would be nobody to credit.
    partner_attribution: partnerAttribution(str(values.partner_attribution)),
    expected_close_date: str(values.expected_close_date) || null,
    project_id: str(values.project_id) || null,
    description: str(values.description) || null,
    commercial_motion: commercialMotion(str(values.commercial_motion)),
    priority: dealPriority(str(values.priority)),
    acquisition_source: str(values.acquisition_source) || null,
    source: "manual",
  };
}
