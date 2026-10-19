/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "./brief.testkit";
import { BriefDeliveryPanel } from "./briefdelivery";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("lets a chosen personal time return to working hours without resetting other choices", async () => {
  const calls = stubApi({
    "GET /me/brief-delivery": () =>
      jsonResponse({
        routine_delivery_time: "23:00",
        morning_brief_delivery: "none",
      }),
    "PUT /me/brief-delivery": () =>
      jsonResponse({
        routine_delivery_time: "working",
        morning_brief_delivery: "none",
      }),
  });
  render(<BriefDeliveryPanel />);
  const user = userEvent.setup();
  const row = await screen.findByTestId("delivery-time");
  await user.click(within(row).getByRole("combobox"));
  await user.click(
    await screen.findByRole("option", { name: en["delivery.workingStart"] }),
  );
  await waitFor(() =>
    expect(
      calls.filter((call) => call.method === "PUT").map((call) => call.body),
    ).toEqual([{ routine_delivery_time: "working" }]),
  );
});

it("connects a refused routine choice to its own control", async () => {
  stubApi({
    "GET /me/brief-delivery": () => jsonResponse({}),
    "PUT /me/brief-delivery": () =>
      jsonResponse({ title: "Unavailable", status: 503 }, 503),
  });
  render(<BriefDeliveryPanel />);
  const user = userEvent.setup();
  const row = await screen.findByTestId("delivery-combine");
  await user.click(within(row).getByRole("combobox"));
  await user.click(
    await screen.findByRole("option", { name: en["delivery.combined"] }),
  );
  const refusal = await screen.findByRole("alert");
  expect(
    within(row).getByRole("combobox").getAttribute("aria-describedby"),
  ).toContain(refusal.id);
  expect(
    within(screen.getByTestId("delivery-weekday"))
      .getByRole("combobox")
      .getAttribute("aria-describedby"),
  ).not.toContain(refusal.id);
});

it("does not offer inbox choices until the stored choices are known", async () => {
  let resolve: ((response: Response) => void) | undefined;
  stubApi({
    "GET /me/brief-delivery": () =>
      new Promise<Response>((answer) => {
        resolve = answer;
      }),
  });
  render(<BriefDeliveryPanel />);
  expect(screen.queryByRole("combobox")).toBeNull();
  await waitFor(() => expect(resolve).toBeDefined());
  resolve?.(jsonResponse({ morning_brief_delivery: "none" }));
  const morning = await screen.findByTestId("delivery-morning");
  expect(within(morning).getByRole("combobox").textContent).toContain(
    en["delivery.none"],
  );
});
