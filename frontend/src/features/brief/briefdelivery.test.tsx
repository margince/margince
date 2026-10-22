/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefDeliveryPanel } from "./briefdelivery";

// What a reader may switch off, and what the control must not claim on their
// behalf. The server tells "never chose" from "chose none", and the screen has
// to keep that distinction honest in both directions.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderRows() {
  return render(
    <SavePageProvider>
      <BriefDeliveryPanel />
      <PageSaveBar />
    </SavePageProvider>,
  );
}

async function pickWeeklyNone(user: ReturnType<typeof userEvent.setup>) {
  const weekly = await screen.findByTestId("delivery-weekly");
  await user.click(within(weekly).getByRole("combobox"));
  await user.click(
    within(screen.getByRole("listbox")).getByRole("option", {
      name: en["delivery.none"],
    }),
  );
}

describe("the delivery settings", () => {
  it("shows what happens today for a reader who has never chosen", async () => {
    // No stored choice: the endpoint answers an empty object, and today's
    // behaviour for an unchosen setting is that the mail is sent. A blank
    // control would tell the reader nothing about what actually happens.
    stubApi({ "GET /me/brief-delivery": () => jsonResponse({}) });
    renderRows();

    const morning = await screen.findByTestId("delivery-morning");
    await waitFor(() =>
      expect(within(morning).getByRole("combobox").textContent).toContain(
        en["delivery.byEmail"],
      ),
    );
  });

  // Only the touched setting goes out. A body carrying the other too would turn
  // its standing default into a decision the reader never made.
  it("sends only the setting that was touched, once Save is pressed", async () => {
    const calls = stubApi({
      "GET /me/brief-delivery": () =>
        jsonResponse({
          morning_brief_delivery: "email",
          weekly_delivery: "email",
        }),
      "PUT /me/brief-delivery": () => jsonResponse({}),
    });
    renderRows();
    const user = userEvent.setup();
    await pickWeeklyNone(user);

    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: en["saveBar.save"] }));

    await waitFor(() => {
      const saves = calls.filter((call) => call.method === "PUT");
      expect(saves).toHaveLength(1);
      expect(saves[0].body).toEqual({ weekly_delivery: "none" });
    });
  });

  it("puts the stored choice back on Discard and sends nothing", async () => {
    const calls = stubApi({
      "GET /me/brief-delivery": () =>
        jsonResponse({ weekly_delivery: "email" }),
    });
    renderRows();
    const user = userEvent.setup();
    await pickWeeklyNone(user);
    await user.click(
      screen.getByRole("button", { name: en["saveBar.discard"] }),
    );

    const weekly = screen.getByTestId("delivery-weekly");
    expect(within(weekly).getByRole("combobox").textContent).toContain(
      en["delivery.byEmail"],
    );
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(0);
  });
});
