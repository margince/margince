import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Readings = components["schemas"]["ForecastReadings"];

// The figure the customer still supports, under the baseline it is a part of.
// Left-out and unknown deals are said apart. An unknown deal is a gap in what
// was checked, and folding it into "left out" would blame the pipeline for it.
export function QualifiedAnswer({
  readings,
  money,
}: Readonly<{
  readings: Readings;
  money: (minor: number | null | undefined) => string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const qualified = readings.qualified;
  if (!qualified) {
    return null;
  }
  return (
    <>
      <p>
        {t("forecast.qualifiedAnswer", {
          evidence: money(readings.won_minor + qualified.evidence_minor),
          weighted: money(qualified.weighted_minor),
        })}
      </p>
      {qualified.excluded_count > 0 && (
        <p className="t-caption">
          {plural("forecast.qualifiedLeftOut", qualified.excluded_count, {
            count: formatNumber(qualified.excluded_count, locale),
            amount: money(qualified.excluded_minor),
          })}
        </p>
      )}
      {qualified.unknown_count > 0 && (
        <p className="t-caption">
          {plural("forecast.qualifiedUnknown", qualified.unknown_count, {
            count: formatNumber(qualified.unknown_count, locale),
            amount: money(qualified.unknown_minor),
          })}
        </p>
      )}
    </>
  );
}
