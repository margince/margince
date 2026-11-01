// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure, TableScroll } from "./atoms";
import type { ChartProps } from "./report-charts.model";

// Every chart's numbers as a table under a fold, for a reader who wants the
// figures rather than the shapes.
export function ReadingTable({
  readings,
  label,
  dataLabel,
  valueLabel,
  secondaryLabel,
  secondary,
  onSelect,
}: ChartProps &
  Readonly<{
    secondaryLabel?: string;
    secondary?: "comparisonAmount" | "targetAmount" | "upperAmount";
  }>) {
  return (
    <Disclosure className="report-chart-data" summary={dataLabel}>
      <TableScroll className="report-chart-table-scroll" label={label}>
        <table>
          <caption className="sr-only">{label}</caption>
          <thead>
            <tr>
              <th scope="col">{label}</th>
              <th scope="col">{valueLabel}</th>
              {secondaryLabel && <th scope="col">{secondaryLabel}</th>}
            </tr>
          </thead>
          <tbody>
            {readings.map((reading) => (
              <tr key={reading.key}>
                <th scope="row">
                  {onSelect ? (
                    <Button
                      variant="link"
                      onClick={() => onSelect(reading.key)}
                    >
                      {reading.label}
                    </Button>
                  ) : (
                    reading.label
                  )}
                </th>
                <td>{reading.amount}</td>
                {secondaryLabel && (
                  <td>{secondary ? reading[secondary] : null}</td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
    </Disclosure>
  );
}
