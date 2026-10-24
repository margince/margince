import type { components } from "../api/schema";
import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { forReader } from "./collate";

export type IndustryCode = NonNullable<
  components["schemas"]["Company"]["industry"]
>;

// Typed as a full record over the contract's enum, so a code the server gains
// without a wording here fails the type check.
const INDUSTRY_LABEL_KEYS: Record<IndustryCode, MessageKey> = {
  agriculture: "industry.agriculture",
  automotive: "industry.automotive",
  aerospace_defense: "industry.aerospaceDefense",
  chemicals_materials: "industry.chemicalsMaterials",
  construction: "industry.construction",
  consumer_goods_retail: "industry.consumerGoodsRetail",
  education: "industry.education",
  energy_utilities: "industry.energyUtilities",
  entertainment_media: "industry.entertainmentMedia",
  financial_services: "industry.financialServices",
  food_beverage: "industry.foodBeverage",
  government_public_sector: "industry.governmentPublicSector",
  healthcare: "industry.healthcare",
  hospitality_tourism: "industry.hospitalityTourism",
  insurance: "industry.insurance",
  it_services: "industry.itServices",
  software: "industry.software",
  legal_professional_services: "industry.legalProfessionalServices",
  logistics_transport: "industry.logisticsTransport",
  manufacturing: "industry.manufacturing",
  marketing_advertising: "industry.marketingAdvertising",
  mining_metals: "industry.miningMetals",
  nonprofit: "industry.nonprofit",
  pharma_biotech: "industry.pharmaBiotech",
  real_estate: "industry.realEstate",
  telecommunications: "industry.telecommunications",
};

export const INDUSTRY_CODES: readonly IndustryCode[] = [
  "agriculture",
  "automotive",
  "aerospace_defense",
  "chemicals_materials",
  "construction",
  "consumer_goods_retail",
  "education",
  "energy_utilities",
  "entertainment_media",
  "financial_services",
  "food_beverage",
  "government_public_sector",
  "healthcare",
  "hospitality_tourism",
  "insurance",
  "it_services",
  "software",
  "legal_professional_services",
  "logistics_transport",
  "manufacturing",
  "marketing_advertising",
  "mining_metals",
  "nonprofit",
  "pharma_biotech",
  "real_estate",
  "telecommunications",
];

// The wording key of a stored industry, or undefined for a value that is no
// code (an older server, or a row not yet repaired).
export function industryLabelKey(value: string): MessageKey | undefined {
  const code = INDUSTRY_CODES.find((known) => known === value);
  return code ? INDUSTRY_LABEL_KEYS[code] : undefined;
}

// What a reader sees for a stored industry: the wording of a code, and the
// value as stored for anything else.
export function industryWords(
  t: (key: MessageKey) => string,
  value: string,
): string {
  const key = industryLabelKey(value);
  return key ? t(key) : value;
}

// The fixed list as picker options, in the reader's own order.
export function industryOptions(
  t: (key: MessageKey) => string,
  locale: Locale,
): { value: string; label: string }[] {
  return INDUSTRY_CODES.map((value) => ({
    value,
    label: industryWords(t, value),
  })).sort((a, b) => forReader(a.label, b.label, locale));
}
