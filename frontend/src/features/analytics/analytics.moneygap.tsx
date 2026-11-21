import { CoverageNote } from "../../components/partialmark";
import type { useT } from "../../i18n";
import { hasGap, type MoneyCoverage } from "../../lib/moneycoverage";

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
