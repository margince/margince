// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import { createPortal } from "react-dom";
import { Disclosure } from "./atoms";

// Where a chart's table door is drawn. A card that holds one chart offers its
// foot, so the door shares a line with the coverage and the records link.
// With no card around it, the door stays under the chart.
const ChartTableSlot = createContext<HTMLElement | null>(null);
export const ChartTableSlotProvider = ChartTableSlot.Provider;

export function ChartTableDoor({
  summary,
  children,
}: Readonly<{ summary: ReactNode; children: ReactNode }>) {
  const slot = useContext(ChartTableSlot);
  const door = (
    <Disclosure className="report-chart-data" summary={summary}>
      {children}
    </Disclosure>
  );
  return slot ? createPortal(door, slot) : door;
}
