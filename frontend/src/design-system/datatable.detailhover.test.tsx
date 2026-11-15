/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { DataTable, type DataTableDetail } from "./datatable";

afterEach(cleanup);

const CALLS = [{ id: "a", task: "Triage", trace: "Two attempts" }];
type Call = (typeof CALLS)[number];
const TRACE = {
  header: "Detail",
  toggleLabel: (call) => `Show ${call.task}`,
  render: (call) => <p>{call.trace}</p>,
} satisfies DataTableDetail<Call>;

function CallLog() {
  return (
    <DataTable
      label="Calls"
      columns={[{ key: "task", header: "Task", render: (call) => call.task }]}
      rows={CALLS}
      rowKey={(call) => call.id}
      rowTestId={(call) => `call-${call.id}`}
      detail={TRACE}
    />
  );
}

it("gives an open row its hover ground while the pointer is on its detail", async () => {
  const user = userEvent.setup();
  render(<CallLog />);
  await user.click(screen.getByText("Triage"));
  const row = screen.getByTestId("call-a");
  const detail = screen.getByText("Two attempts").closest("tr");
  if (detail === null) throw new Error("the detail drew no row");

  fireEvent.pointerEnter(detail);
  expect(row.hasAttribute("data-detail-hover")).toBe(true);
  fireEvent.pointerLeave(detail);
  expect(row.hasAttribute("data-detail-hover")).toBe(false);

  fireEvent.pointerEnter(detail);
  await user.click(screen.getByRole("button", { name: "Show Triage" }));
  expect(row.hasAttribute("data-detail-hover")).toBe(false);
});

it("keeps the ground while a second pointer on the detail lifts away", async () => {
  const user = userEvent.setup();
  render(<CallLog />);
  await user.click(screen.getByText("Triage"));
  const row = screen.getByTestId("call-a");
  const detail = screen.getByText("Two attempts").closest("tr");
  if (detail === null) throw new Error("the detail drew no row");

  fireEvent.pointerEnter(detail, { pointerId: 1 });
  fireEvent.pointerEnter(detail, { pointerId: 2 });
  fireEvent.pointerLeave(detail, { pointerId: 2 });
  expect(row.hasAttribute("data-detail-hover")).toBe(true);
  fireEvent.pointerLeave(detail, { pointerId: 1 });
  expect(row.hasAttribute("data-detail-hover")).toBe(false);
});
