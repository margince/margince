import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Readings = components["schemas"]["ForecastReadings"];

// The figure the customer still supports, under the baseline it is a part of.
// Left-out and unknown deals are said apart. An unknown deal is a gap in what
// was checked, and folding it into "left out" would blame the pipeline for it.
// A deal left out with no review is said apart too: its fix is on the deal.
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
  const underReview = qualified.excluded_count - qualified.unsupported_count;
  return (
    <>
      <p>
        {t("forecast.qualifiedAnswer", {
          evidence: money(readings.won_minor + qualified.evidence_minor),
          weighted: money(qualified.weighted_minor),
        })}
      </p>
      {underReview > 0 && (
        <p className="t-caption">
          {plural("forecast.qualifiedLeftOut", underReview, {
            count: formatNumber(underReview, locale),
            amount: money(
              qualified.excluded_minor - qualified.unsupported_minor,
            ),
          })}
        </p>
      )}
      {qualified.unsupported_count > 0 && (
        <p className="t-caption">
          {plural(
            "forecast.qualifiedUnsupported",
            qualified.unsupported_count,
            {
              count: formatNumber(qualified.unsupported_count, locale),
              amount: money(qualified.unsupported_minor),
            },
          )}
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
      {qualified.amounts_partial && (
        <p className="t-caption">{t("forecast.qualifiedPartial")}</p>
      )}
    </>
  );
}
