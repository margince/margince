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
