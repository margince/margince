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
