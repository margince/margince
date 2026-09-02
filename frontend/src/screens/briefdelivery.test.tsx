/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefDeliveryRows } from "./briefdelivery";
import { jsonResponse, render, stubApi } from "./home.testkit";

// What a reader may switch off, and what the control must not claim on their
// behalf. The server tells "never chose" from "chose none", and the screen has
// to keep that distinction honest in both directions.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the delivery settings", () => {
  it("shows what happens today for a reader who has never chosen", async () => {
    // No stored choice: the endpoint answers an empty object, and today's
    // behaviour for an unchosen setting is that the mail is sent. A blank
    // control would tell the reader nothing about what actually happens.
    stubApi({ "GET /me/brief-delivery": () => jsonResponse({}) });
    render(<BriefDeliveryRows />);

    const morning = await screen.findByTestId("delivery-morning");
    await waitFor(() =>
      expect(within(morning).getByRole("combobox").textContent).toContain(
        en["delivery.byEmail"],
      ),
    );
  });

  // Each control sends only ITSELF. A row that sent the whole form would let a
  // stale render of one dropdown overwrite a choice just made in another.
  it("sends only the setting that was touched", async () => {
    const calls = stubApi({
      "GET /me/brief-delivery": () =>
        jsonResponse({
          morning_brief_delivery: "email",
          weekly_delivery: "email",
        }),
      "PUT /me/brief-delivery": () => jsonResponse({}),
    });
    render(<BriefDeliveryRows />);

    const weekly = await screen.findByTestId("delivery-weekly");
    const user = userEvent.setup();
    await user.click(within(weekly).getByRole("combobox"));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: en["delivery.none"],
      }),
    );

    await waitFor(() => {
      const saves = calls.filter((call) => call.method === "PUT");
      expect(saves).toHaveLength(1);
      // Only the weekly. A body carrying the morning setting too would let a
      // stale render of that dropdown overwrite a choice made elsewhere.
      expect(saves[0].body).toEqual({ weekly_delivery: "none" });
    });
  });
});
