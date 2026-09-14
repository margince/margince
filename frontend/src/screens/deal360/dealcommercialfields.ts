import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import type { CreateField } from "../create";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];

type AcquisitionSource = components["schemas"]["AcquisitionSource"];

/**
 * The deal's commercial context, as form fields: the human brief, why the deal
 * exists, how much it matters, and which channel brought it.
 *
 * Its own file because `deals.tsx` sits at the tree's file-length ceiling, and
 * this is a self-contained block of field declarations with one caller.
 *
 * Every field is optional and every select leads with the empty option, which
 * carries its own label rather than a blank line: "not set" is a real answer
 * here — nobody has said — and it is different from any value the list offers.
 */
export function dealCommercialFields(
  t: (k: MessageKey) => string,
  opts: {
    motionOptions: { value: string; label: MessageKey }[];
    priorityOptions: { value: string; label: MessageKey }[];
    /**
     * The catalog, active entries first. A RETIRED entry appears only when
     * this deal already carries it: the value has to stay readable and
     * re-savable through an unrelated edit, and offering it to every other
     * deal would let a withdrawn channel back into circulation.
     */
    sources: AcquisitionSource[];
    currentSource?: string | null;
  },
): CreateField[] {
  return [
    {
      key: "description",
      label: "deal.brief",
      type: "textarea",
      hint: "deal.briefHint",
    },
    {
      key: "commercial_motion",
      label: "deal.motion",
      type: "select",
      options: opts.motionOptions.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
    {
      key: "priority",
      label: "deal.priority",
      type: "select",
      options: opts.priorityOptions.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
    {
      key: "acquisition_source",
      label: "deal.acquisitionSource",
      type: "select",
      options: acquisitionOptions(t, opts.sources, opts.currentSource),
    },
  ];
}

/**
 * The pickable channels: the empty answer, every active entry, and the deal's
 * own stored entry when that one has since been retired.
 */
export function acquisitionOptions(
  t: (k: MessageKey) => string,
  sources: AcquisitionSource[],
  current?: string | null,
): { value: string; label: string }[] {
  const options = [{ value: "", label: t("deal.acquisitionUnset") }];
  for (const source of sources) {
    if (source.active || source.key === current) {
      options.push({ value: source.key, label: source.label });
    }
  }
  // A key the catalog no longer lists at all — deleted out from under the deal,
  // or not yet loaded. Shown as itself rather than dropped, because dropping it
  // makes the select read as empty and the next save clears a value the reader
  // never touched.
  if (current && !sources.some((source) => source.key === current)) {
    options.push({ value: current, label: current });
  }
  return options;
}

// The two commercial enums the form may send, narrowed exactly as the partner
// attribution above is: the wire types are closed vocabularies, and a string
// off a select is not one until something checks it. An unrecognised value —
// including the empty option — becomes null, which is the wire's way of saying
// nobody answered.
export function commercialMotion(
  v: string,
): UpdateDealRequest["commercial_motion"] {
  switch (v) {
    case "new_business":
      return "new_business";
    case "renewal":
      return "renewal";
    case "upsell":
      return "upsell";
    case "cross_sell":
      return "cross_sell";
    case "expansion":
      return "expansion";
    case "existing_business":
      return "existing_business";
    default:
      return null;
  }
}

export function dealPriority(v: string): UpdateDealRequest["priority"] {
  switch (v) {
    case "low":
      return "low";
    case "medium":
      return "medium";
    case "high":
      return "high";
    default:
      return null;
  }
}

export const MOTION_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "", label: "deal.motionUnset" },
  { value: "new_business", label: "deal.motionNewBusiness" },
  { value: "renewal", label: "deal.motionRenewal" },
  { value: "upsell", label: "deal.motionUpsell" },
  { value: "cross_sell", label: "deal.motionCrossSell" },
  { value: "expansion", label: "deal.motionExpansion" },
  { value: "existing_business", label: "deal.motionExistingBusiness" },
];

export const PRIORITY_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "", label: "deal.priorityUnset" },
  { value: "high", label: "deal.priorityHigh" },
  { value: "medium", label: "deal.priorityMedium" },
  { value: "low", label: "deal.priorityLow" },
];
