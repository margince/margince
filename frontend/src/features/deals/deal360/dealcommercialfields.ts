import type { components } from "../../../api/schema";
import type { CreateField } from "../../../components/create";
import {
  DEAL_MOTION_LABEL,
  DEAL_PRIORITY_LABEL,
  withUnset,
} from "../../../format/dealvocab";
import type { MessageKey } from "../../../i18n/en";
import { acquisitionOptions } from "../../../lib/acquisitionsources.queries";

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
      maxLength: 20000,
      label: "deal.brief",
      type: "textarea",
      hint: t("deal.briefHint"),
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

export const MOTION_OPTIONS = withUnset(DEAL_MOTION_LABEL, "deal.motionUnset");

export const PRIORITY_OPTIONS = withUnset(
  DEAL_PRIORITY_LABEL,
  "deal.priorityUnset",
);
