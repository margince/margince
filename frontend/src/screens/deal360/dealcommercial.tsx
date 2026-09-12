import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatMoney } from "../../format/format";
import { monthlyEquivalent } from "../../format/recurring";
import { useLocale, useT } from "../../i18n";
import {
  type AcquisitionSource,
  acquisitionLabel,
} from "../acquisitionsources.queries";
import { MOTION_OPTIONS, PRIORITY_OPTIONS } from "./dealcommercialfields";

type Deal = components["schemas"]["Deal"];

/**
 * The deal's commercial context in the record's side pane: why it exists, how
 * much a colleague says it matters, which channel brought it, and what it is
 * worth per year on a recurring basis.
 *
 * Renders nothing when all of them are unset. An empty panel would say the deal
 * has no commercial context when what is true is that nobody has recorded one,
 * and the difference matters on a page whose whole job is to show what is
 * known.
 */
export function DealCommercial({
  deal,
  sources,
}: Readonly<{ deal: Deal; sources?: AcquisitionSource[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const motion = MOTION_OPTIONS.find(
    (o) => o.value && o.value === deal.commercial_motion,
  );
  const priority = PRIORITY_OPTIONS.find(
    (o) => o.value && o.value === deal.priority,
  );
  const source = acquisitionLabel(deal.acquisition_source, sources);
  // BOTH halves or neither. A figure without its currency cannot be scaled,
  // and the read mask withholds the money reading as one unit, so an ARR
  // arriving without a code is a withheld field rather than a priced deal.
  const arr =
    deal.expected_arr_minor != null && deal.currency
      ? { minor: deal.expected_arr_minor, currency: deal.currency }
      : null;
  const monthly = arr ? monthlyEquivalent(arr.minor) : null;
  if (!motion && !priority && !source && !arr) {
    return null;
  }
  return (
    <Panel title={t("deal.commercialContext")}>
      <PanelBody>
        {/* The same labelled-pair list the custom-field card and the company
            rail use, so a reader meets one shape for "field: value" across
            every record page. */}
        <dl className="firmo">
          {motion && (
            <div>
              <dt className="t-eyebrow">{t("deal.motion")}</dt>
              <dd>{t(motion.label)}</dd>
            </div>
          )}
          {/* The WORD, never a colour alone: a reader who cannot tell the
              tones apart still reads the value. */}
          {priority && (
            <div>
              <dt className="t-eyebrow">{t("deal.priority")}</dt>
              <dd>{t(priority.label)}</dd>
            </div>
          )}
          {source && (
            <div>
              <dt className="t-eyebrow">{t("deal.acquisitionSource")}</dt>
              <dd>{source}</dd>
            </div>
          )}
          {arr && (
            <div>
              <dt className="t-eyebrow">{t("deal.expectedArr")}</dt>
              <dd>{formatMoney(arr.minor, arr.currency, locale)}</dd>
            </div>
          )}
          {/* The monthly reading is DERIVED, and says so when the division
              lost something: twelve of an approximate figure do not add back
              to the year, and a reader who multiplies it should not be
              surprised. */}
          {arr && monthly && (
            <div>
              <dt className="t-eyebrow">{t("deal.monthlyEquivalent")}</dt>
              <dd>
                {monthly.approximate && `${t("deal.monthlyApproximate")} `}
                {formatMoney(monthly.monthlyMinor, arr.currency, locale)}
              </dd>
            </div>
          )}
        </dl>
      </PanelBody>
    </Panel>
  );
}
