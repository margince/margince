import {
  type AcquisitionSource,
  acquisitionFilterOptions,
} from "../acquisitionsources.queries";
import type { FilterSpec } from "../listquery";
import { MOTION_OPTIONS, PRIORITY_OPTIONS } from "./dealcommercialfields";

// The surface's own narrowing chips, beside the stage and company ones
// dealFilterChips builds.
//
// The owner and team dials are shared by every owner-scoped list, in
// ownerdials.ts.
//
// A function because two of these are conditional. A chip offered before its
// options are known reads as "clear this filter" to the table. A chip
// withdrawn while its filter is applied leaves the list narrowed with no dial
// to clear it.
export function dealSurfaceChips({
  partnerOptions,
  partnerApplied,
  acquisitionSources,
  retiredSuffix,
}: Readonly<{
  partnerOptions: { value: string; label: string }[];
  partnerApplied?: string;
  acquisitionSources?: AcquisitionSource[];
  // Already translated: the option's `text` arm takes a rendered string, and
  // this file has no translator at the point the options are built.
  retiredSuffix: string;
}>): FilterSpec[] {
  return [
    {
      key: "stalled",
      label: "deals.filterStalled",
      allLabel: "deals.filterStalledAll",
      options: [{ value: "true", label: "deals.filterStalled" }],
    },
    {
      key: "partner_sourced",
      label: "deals.filterPartnerSourced",
      allLabel: "deals.filterPartnerAll",
      options: [{ value: "true", label: "deals.filterPartnerSourced" }],
    },
    // The forecast's own buckets, in the order the tiles read them. Four, not
    // five: `slipped` is derived by the report from a claimed category and a
    // close date, so there is no column to filter on and a chip offering it
    // would narrow to nothing.
    //
    // Unconditional: the vocabulary is the schema's, so
    // there is no moment when the options are not yet known.
    {
      key: "forecast_category",
      label: "deals.filterForecast",
      allLabel: "deals.filterForecastAll",
      options: [
        { value: "commit", label: "deal.fcCommit" },
        { value: "best_case", label: "deal.fcBestCase" },
        { value: "pipeline", label: "deal.fcPipeline" },
        { value: "omitted", label: "deal.fcOmitted" },
      ],
    },
    {
      key: "commercial_motion",
      label: "deals.filterMotion",
      allLabel: "deals.filterMotionAll",
      options: MOTION_OPTIONS.filter((o) => o.value !== "").map((o) => ({
        value: o.value,
        label: o.label,
      })),
    },
    {
      key: "priority",
      label: "deals.filterPriority",
      allLabel: "deals.filterPriorityAll",
      options: PRIORITY_OPTIONS.filter((o) => o.value !== "").map((o) => ({
        value: o.value,
        label: o.label,
      })),
    },
    // The administered channels, offered only once the catalog is loaded: an
    // option whose value is still "" reads as "clear this filter".
    //
    // RETIRED entries are offered here though the form refuses them. Deals
    // still carry those keys, and a filter that omitted them would make
    // exactly those deals unreachable through the control that would find
    // them; the suffix says why a channel nobody can choose is listed.
    ...(acquisitionSources?.length
      ? [
          {
            key: "acquisition_source",
            label: "deals.filterAcquisition" as const,
            allLabel: "deals.filterAcquisitionAll" as const,
            options: acquisitionFilterOptions(
              acquisitionSources,
              retiredSuffix,
            ),
          },
        ]
      : []),
    // Which partner, not just whether there is one. Absent entirely
    // when the installation has made no company a partner: a picker
    // with nothing in it asks a question that has no answers, the same
    // rule the deal form's own partner fields follow.
    //
    // The options come from usePartnerOptions, so a partner whose
    // company this reader cannot open is not offered — picking it
    // would name a company the screen could not then show them.
    // Present whenever there are partners to pick OR one is already
    // applied. A saved view can restore a partner_company_id after the
    // programme was wound down or while the options are still in
    // flight, and hiding the chip then would leave the list narrowed
    // by a filter with no dial to see or clear it.
    ...(partnerOptions.length > 0 || partnerApplied
      ? [
          {
            key: "partner_company_id" as const,
            label: "deals.filterPartner" as const,
            allLabel: "deals.filterPartnerAnyOne" as const,
            // `text`, not `label`: a partner's name is the server's
            // data, not this screen's vocabulary, and FilterOption's
            // union exists for exactly that. Every other chip here
            // names a message key because its options are a fixed set
            // somebody wrote; a company name has nothing to translate.
            options: partnerOptions.map((option) => ({
              value: option.value,
              text: option.label,
            })),
          },
        ]
      : []),
  ];
}
