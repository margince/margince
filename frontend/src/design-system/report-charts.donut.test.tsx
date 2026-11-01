/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DonutChart } from "./report-charts.donut";

afterEach(cleanup);

const stages = [
  { key: "discovery", label: "Discovery", value: 340, amount: "€340k" },
  { key: "proposal", label: "Proposal", value: 580, amount: "€580k" },
  { key: "empty", label: "Lost interest", value: 0, amount: "€0" },
  { key: "negotiation", label: "Negotiation", value: 80, amount: "€80k" },
];

function draw(onSelect?: (key: string) => void) {
  return render(
    <LocaleProvider initial="en">
      <DonutChart
        label="Open pipeline by stage"
        parts={stages}
        total="€1m"
        totalLabel="Total"
        onSelect={onSelect}
      />
    </LocaleProvider>,
  );
}

it("draws an arc and a legend row for each part above zero, with its share", () => {
  const { container } = draw();
  expect(container.querySelectorAll(".report-donut-arc")).toHaveLength(3);
  for (const [label, share] of [
    ["Discovery", "34%"],
    ["Proposal", "58%"],
    ["Negotiation", "8%"],
  ]) {
    expect(screen.getByText(label)).toBeTruthy();
    expect(screen.getByText(share)).toBeTruthy();
  }
  expect(screen.queryByText("Lost interest")).toBeNull();
  expect(screen.getByText("€1m")).toBeTruthy();
});

it("opens a part from its legend row", () => {
  const select = vi.fn();
  draw(select);
  fireEvent.click(screen.getByRole("button", { name: /Proposal/ }));
  expect(select).toHaveBeenCalledWith("proposal");
});

it("steps the other parts back while one is focused", () => {
  const { container } = draw(vi.fn());
  fireEvent.focus(screen.getByRole("button", { name: /Discovery/ }));
  expect(
    container.querySelector(".report-donut")?.getAttribute("data-active"),
  ).toBe("discovery");
  expect(
    container.querySelectorAll(".report-donut-arc:not([data-on])"),
  ).toHaveLength(2);
  expect(
    container.querySelectorAll(".report-donut-legend > li:not([data-on])"),
  ).toHaveLength(2);
});
