import "./partialmark.css";
import { AppLink } from "../design-system/applink";
import { useT } from "../i18n";
import { RATE_SHEET, useCoverageWords } from "../lib/coveragewords";
import { hasGap, type MoneyCoverage } from "../lib/moneycoverage";

// The word that marks a figure adding only the deals that convert. It stands
// beside the raw and the weighted figure alike, since both leave the deals out.
export function PartialMark({
  coverage,
}: Readonly<{ coverage: MoneyCoverage | null | undefined }>) {
  const t = useT();
  return hasGap(coverage) ? (
    <span className="t-caption partial-mark"> {t("coverage.partial")}</span>
  ) : null;
}

// The two reasons a deal is left out of a figure, worded apart. The rate note
// links to the rates, the one cause a reader can fix.
export function CoverageNote({
  coverage,
}: Readonly<{ coverage: MoneyCoverage }>) {
  const { rate, value } = useCoverageWords()(coverage);
  return (
    <>
      {rate && <AppLink href={RATE_SHEET}>{rate}</AppLink>}
      {rate && value && " · "}
      {value}
    </>
  );
}
