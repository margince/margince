/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { PublicOriginCard } from "./public-origin";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it.each([
  { reachable: true, label: "Reachable" },
  { reachable: false, label: "Unreachable" },
  { reachable: null, label: "Not checked" },
])(
  "renders the stored $label observation without a probe",
  async ({ reachable, label }) => {
    const fetcher = vi.fn();
    vi.stubGlobal("fetch", fetcher);
    render(
      <LocaleProvider initial="en">
        <PublicOriginCard
          status={{ origin: "https://crm.example.test", reachable }}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.getByText("https://crm.example.test")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Configuration" })).toBeNull();
    expect(fetcher).not.toHaveBeenCalled();
  },
);
it("leaves an unconfigured origin absent", () => {
  render(
    <LocaleProvider initial="en">
      <PublicOriginCard />
    </LocaleProvider>,
  );
  expect(screen.queryByTestId("public-origin")).toBeNull();
});
