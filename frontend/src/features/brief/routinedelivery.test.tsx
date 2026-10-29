/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefDeliveryPanel } from "./briefdelivery";

// A pick is a draft that the page's one save bar sends.
function Page() {
  return (
    <SavePageProvider>
      <BriefDeliveryPanel />
      <PageSaveBar />
    </SavePageProvider>
  );
}

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
  render(<Page />);
  const user = userEvent.setup();
  const row = await screen.findByTestId("delivery-time");
  await user.click(within(row).getByRole("combobox"));
  await user.click(
    await screen.findByRole("option", { name: en["delivery.workingStart"] }),
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(
      calls.filter((call) => call.method === "PUT").map((call) => call.body),
    ).toEqual([{ routine_delivery_time: "working" }]),
  );
});

it("keeps a refused routine choice drafted and says so on the save bar", async () => {
  stubApi({
    "GET /me/brief-delivery": () => jsonResponse({}),
    "PUT /me/brief-delivery": () =>
      jsonResponse({ title: "Unavailable", status: 503 }, 503),
  });
  render(<Page />);
  const user = userEvent.setup();
  const row = await screen.findByTestId("delivery-combine");
  await user.click(within(row).getByRole("combobox"));
  await user.click(
    await screen.findByRole("option", { name: en["delivery.combined"] }),
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  const alerts = await screen.findAllByRole("alert");
  const refused = within(row).getByRole("combobox");
  expect(refused.textContent).toContain(en["delivery.combined"]);
  // The refused control points at the notice that names the failed write.
  const named = (refused.getAttribute("aria-describedby") ?? "").split(" ");
  expect(alerts.some((alert) => named.includes(alert.id))).toBe(true);
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

it("decides the team choices from /me and never assembles the worklist to do it", async () => {
  const calls = stubApi({
    "GET /me/brief-delivery": () =>
      jsonResponse({ routine_mail_scope: "team" }),
  });
  render(<Page />);
  await screen.findByTestId("delivery-time");
  await waitFor(() =>
    expect(calls.some((call) => call.path.startsWith("/me"))).toBe(true),
  );
  expect(calls.filter((call) => call.path.startsWith("/worklist"))).toEqual(
    [],
  );
});
