import type { components } from "../api/schema";
import { typedMinorOrNaN } from "../format/minorunits";
import type { Locale } from "../i18n";
import { commercialMotion, dealPriority } from "./dealcommercialvalues";
import { currencyBeside } from "./dealcurrency";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];
type CreateDealRequest = components["schemas"]["CreateDealRequest"];

export function partnerAttribution(
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

export function mapDealCreate(
  values: Record<string, unknown>,
  pipelineId: string,
  defaultCurrency: string,
  locale: Locale,
): CreateDealRequest {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const amount = str(values.amount);
  const expectedArr = str(values.expected_arr);
  const currency = str(values.currency) || defaultCurrency;
  return {
    name: str(values.name),
    pipeline_id: pipelineId,
    stage_id: str(values.stage_id),
    amount_minor: amount ? typedMinorOrNaN(amount, currency, locale) : null,
    expected_arr_minor: expectedArr
      ? typedMinorOrNaN(expectedArr, currency, locale)
      : null,
    currency: currencyBeside(currency, amount, expectedArr),
    company_id: str(values.company_id) || null,
    owner_id: str(values.owner_id) || null,
    partner_company_id: str(values.partner_company_id) || null,
    // A named partner with no claim defaults to sourced on the server.
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
