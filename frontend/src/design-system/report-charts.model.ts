export type ChartReading = Readonly<{
  key: string;
  label: string;
  value: number | null;
  amount: string;
  comparison?: number | null;
  comparisonAmount?: string;
  target?: number | null;
  targetAmount?: string;
  upper?: number | null;
  upperAmount?: string;
}>;

export type ChartProps = Readonly<{
  readings: readonly ChartReading[];
  label: string;
  dataLabel: string;
  valueLabel: string;
  onSelect?: (key: string) => void;
}>;

export type GroupedBarsProps = ChartProps &
  Readonly<{
    comparisonLabel?: string;
    /** `compact` is shorter columns, for weeks beside a lead figure. */
    size?: "default" | "compact";
  }>;

// The top of a chart's axis: the largest value any reading draws, never below 1.
export function readingScale(readings: readonly ChartReading[], reference = 0) {
  return Math.max(
    1,
    reference,
    ...readings.flatMap((reading) => [
      reading.value ?? 0,
      reading.comparison ?? 0,
      reading.target ?? 0,
      reading.upper ?? 0,
    ]),
  );
}

/**
 * An axis that ends on a round figure. The top and its steps are 1, 2, 2.5 or
 * 5 times a power of ten, so a gridline reads "€500k", never "€453.6k".
 */
export function niceAxis(largest: number, steps = 4) {
  const rough = Math.max(largest, 1) / steps;
  const power = 10 ** Math.floor(Math.log10(rough));
  const step =
    [1, 2, 2.5, 5, 10]
      .map((factor) => factor * power)
      .find((s) => s >= rough) ?? 10 * power;
  const top = Math.ceil(Math.max(largest, 1) / step) * step;
  const ticks = Array.from(
    { length: Math.round(top / step) + 1 },
    (_, index) => index * step,
  );
  return { top, ticks };
}
