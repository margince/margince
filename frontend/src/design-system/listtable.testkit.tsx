import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import type { ListColumn } from "./listtable";

export function render(ui: ReactNode): RenderResult {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

export type Row = { id: string; name: string; value: number; region: string };

export function testRows(count: number): Row[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `r${index + 1}`,
    name: `Row ${String(index + 1).padStart(2, "0")}`,
    value: index + 1,
    region: "EU",
  }));
}

export const columns: readonly ListColumn<Row>[] = [
  {
    key: "name",
    header: "Name",
    cell: (row) => row.name,
    sort: "name",
    fixed: true,
  },
  {
    key: "value",
    header: "Value",
    cell: (row) => String(row.value),
    sort: "value",
    numeric: true,
  },
  { key: "note", header: "Note", cell: () => "-" },
  { key: "region", header: "Region", cell: (row) => row.region },
];
