/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { AutomationSetup } from "./automations.setup";

afterEach(cleanup);

it("keeps a failed run separate from healthy configuration", () => {
  render(
    <LocaleProvider initial="en">
      <AutomationSetup
        automation={{
          id: "rule",
          key: "route_lead",
          name: "Reply",
          status: "enabled",
          params: {},
          created_at: "2026-07-01T00:00:00Z",
          setup_state: "on",
          last_run_outcome: "failed",
        }}
      />
    </LocaleProvider>,
  );
  expect(screen.getByText("On")).toBeInTheDocument();
  expect(screen.queryByText("Needs attention")).toBeNull();
});

it("shows the verified inactive-authorizer reason", () => {
  render(
    <LocaleProvider initial="en">
      <AutomationSetup
        automation={{
          id: "rule",
          key: "route_lead",
          name: "Reply",
          status: "enabled",
          params: {},
          created_at: "2026-07-01T00:00:00Z",
          setup_state: "needs_attention",
          setup_reason: "authorizer_inactive",
        }}
      />
    </LocaleProvider>,
  );
  expect(screen.getByText("Needs attention")).toBeInTheDocument();
  expect(
    screen.getByText(
      "The authorizer’s account is inactive. Choose another person.",
    ),
  ).toBeInTheDocument();
});
