import { AppLink } from "../../design-system/applink";
import { useT } from "../../i18n";
import { RATE_SHEET, useCoverageWords } from "../../lib/coveragewords";
import { hasGap, type MoneyCoverage } from "../../lib/moneycoverage";

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

// The word that marks a figure adding only the deals that convert. It stands
// beside the raw and the weighted figure alike, since both leave the deals out.
export function PartialMark({
  coverage,
}: Readonly<{ coverage: MoneyCoverage | null | undefined }>) {
  const t = useT();
  return hasGap(coverage) ? (
    <span className="t-caption analytics-partial">
      {" "}
      {t("coverage.partial")}
    </span>
  ) : null;
}

// The same mark for a figure a stat card holds as text.
export function withPartial(
  figure: string,
  coverage: MoneyCoverage | null | undefined,
  t: ReturnType<typeof useT>,
): string {
  return hasGap(coverage) ? `${figure} ${t("coverage.partial")}` : figure;
}

// The footnote under a money figure that leaves deals out: how many, and why.
// Nothing when every deal is in the figure.
export function CoverageFootnote({
  coverage,
}: Readonly<{ coverage: MoneyCoverage | null }>) {
  if (!hasGap(coverage)) {
    return null;
  }
  return (
    <span className="t-caption">
      <CoverageNote coverage={coverage} />
    </span>
  );
}
