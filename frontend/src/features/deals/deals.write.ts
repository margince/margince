import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { saveRecordEdit } from "../../components/recordedit";
import { type FieldRecord, rawRecord } from "../../components/recordfieldtypes";
import { toMajorUnits, toMinorUnits } from "../../format/minorunits";
import { commercialMotion, dealPriority } from "../../lib/dealcommercialvalues";
import { partnerAttribution } from "../../lib/dealcreate";
import { currencyBeside } from "../../lib/dealcurrency";
import { unwrap } from "../../lib/problem";
import type { Deal } from "./deals";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];

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
    // Empty while the deal follows its stage: the field holds only a
    // probability somebody set on the deal.
    win_probability:
      deal.win_probability_source && deal.win_probability_source !== "stage"
        ? String(deal.win_probability ?? "")
        : "",
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

// Empty sends null, which puts the deal back on its stage's probability. A
// value that is not a whole percent is not sent; the field refuses it first.
function winProbability(v: string): UpdateDealRequest["win_probability"] {
  if (v === "") return null;
  const percent = Number(v);
  return Number.isInteger(percent) && percent >= 0 && percent <= 100
    ? percent
    : undefined;
}

function dealPriceUpdate(
  values: Readonly<{
    currency: string;
    amount: string;
    expectedArr: string;
    previousCurrency: string;
  }>,
  moved: (formKey: string) => boolean,
): Pick<UpdateDealRequest, "amount_minor" | "expected_arr_minor" | "currency"> {
  const { currency, amount, expectedArr, previousCurrency } = values;
  const patch: Pick<
    UpdateDealRequest,
    "amount_minor" | "expected_arr_minor" | "currency"
  > = {};
  // The money pair travels together whenever either half moves, because the
  // scale is part of the amount. amount_minor counts the currency's own minor
  // units (a dong has none, a dinar has three). A currency moving alone
  // re-denominates the stored figure: 10000 JPY re-saved as EUR becomes 10000
  // minor EUR (€100). The FX freeze and the forecast history record that price.
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
  const pricedCurrency = currencyBeside(currency, amount, expectedArr);
  if (pricedCurrency !== (previousCurrency || null)) {
    patch.currency = pricedCurrency;
  }
  return patch;
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
  Object.assign(
    patch,
    dealPriceUpdate(
      { currency, amount, expectedArr, previousCurrency: str(seeded.currency) },
      moved,
    ),
  );
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
  onMove("win_probability", "win_probability", () =>
    winProbability(str(values.win_probability)),
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

/** The deal as an editor opens on it: the form's values beside the wire's. */
export function openDealEdit(deal: Deal): FieldRecord {
  return { ...dealEditRecord(deal), original: deal };
}

/**
 * The one write of a deal's fields, wherever they are edited: the form's
 * `values` against the deal as `openDealEdit` opened it, pinned to its version.
 */
export function saveDealEdit(
  opened: FieldRecord,
  values: Record<string, unknown>,
  masked: readonly string[],
) {
  return saveRecordEdit(
    "deal",
    rawRecord(opened),
    mapDealUpdate({ ...opened, ...values }, opened, masked),
  );
}

// Pinned to the version the reader saw, so an archive never lands on a deal
// somebody changed since.
export async function archiveDeal(deal: Pick<Deal, "id" | "version">) {
  return unwrap(
    await api.DELETE("/deals/{id}", {
      params: {
        path: { id: deal.id },
        ...ifMatch(requireVersion(deal.version)),
      },
    }),
  );
}
