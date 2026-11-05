export type ChartReading = Readonly<{
  key: string;
  label: string;
  value: number | null;
  amount: string;
  tone?: "success" | "warning" | "danger" | "neutral";
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

export function lineSegments(
  readings: readonly ChartReading[],
  field: "value" | "comparison",
  width: number,
  y: (value: number) => number,
) {
  const lines: string[] = [];
  let points: string[] = [];
  readings.forEach((reading, index) => {
    const value = reading[field];
    if (value == null) {
      if (points.length) lines.push(points.join(" "));
      points = [];
    } else points.push(`${chartX(index, readings.length, width)},${y(value)}`);
  });
  if (points.length) lines.push(points.join(" "));
  return lines;
}

// A line's points closed down to the baseline, for the fill beneath it.
export function areaUnder(points: string, baseline: number): string {
  const xs = points.split(" ").map((point) => point.split(",")[0]);
  return `${xs[0]},${baseline} ${points} ${xs.at(-1)},${baseline}`;
}

export function chartX(index: number, count: number, width: number) {
  return 56 + (index / Math.max(1, count - 1)) * (width - 76);
}
