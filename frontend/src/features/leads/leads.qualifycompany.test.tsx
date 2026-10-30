/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { LeadScreen } from "./leads";
import { jsonResponse, lead, render, stubFetch } from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const anna = {
  id: "p-1",
  full_name: "Anna Weber",
  captured_by: "human:u-1",
  source: "manual",
  version: 1,
};

describe("the qualify dialog and a typed company", () => {
  it("the qualify dialog asks about a typed company the email cannot place, and sends the rep's choice", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({
          outcome: "create",
          typed_company: "Nordwind Labs",
        });
      }
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    const choice = await screen.findByTestId("lead-qualify-create-company");
    expect(
      screen.getByText(
        en["lead.qualify.companyUnresolved"].replace(
          "{company}",
          "Nordwind Labs",
        ),
      ),
    ).toBeTruthy();
    await user.click(choice);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({ create_company: true });
  });

  it("a company choice ticked and cancelled is not sent by the next opening", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    let cancelled = false;
    stubFetch(async (url, method, request) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse(
          cancelled
            ? { outcome: "create" }
            : { outcome: "create", typed_company: "Nordwind Labs" },
        );
      }
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    await user.click(await screen.findByTestId("lead-qualify-create-company"));
    cancelled = true;
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /cancel/i,
      }),
    );
    await user.click(await screen.findByTestId("lead-qualify"));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({ create_company: false });
  });

  it("the qualify dialog asks nothing about the company when the preview names none to place", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({ outcome: "create" });
      }
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    expect(await screen.findByText(en["lead.previewCreate"])).toBeTruthy();
    expect(screen.queryByTestId("lead-qualify-create-company")).toBeNull();
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({ create_company: false });
  });
});
