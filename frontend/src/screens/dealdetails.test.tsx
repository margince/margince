/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { DealDetails } from "./deals";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const deal: components["schemas"]["Deal"] = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 4,
  pipeline_id: "p1",
  stage_id: "s1",
  currency: "EUR",
  amount_minor: 4800000,
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
afterEach(cleanup);
function mount(current = deal) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] }, { seat: "full" }),
    "PATCH /deals/d1": (body) => {
      sent.push(body);
      return jsonResponse({ ...current, version: 5 });
    },
  });
  const view = render(
    <StoryProviders>
      <DealDetails deal={current} companies={[]} meId="u" />
    </StoryProviders>,
  );
  return { ...view, sent };
}
it("saves a brief verbatim without rewriting other commercial fields", async () => {
  const user = userEvent.setup();
  const { sent } = mount();
  await user.click(
    await screen.findByRole("button", { name: "Change Deal brief" }),
  );
  await user.type(
    screen.getByRole("textbox", { name: "Deal brief" }),
    "  Three warehouses.  ",
  );
  await user.keyboard("{Control>}{Enter}{/Control}");
  await waitFor(() =>
    expect(sent).toEqual([{ description: "  Three warehouses.  " }]),
  );
});
it("clears a brief with null", async () => {
  const user = userEvent.setup();
  const { sent } = mount({ ...deal, description: "Old words" });
  await user.click(
    await screen.findByRole("button", { name: "Change Deal brief" }),
  );
  await user.clear(screen.getByRole("textbox", { name: "Deal brief" }));
  await user.keyboard("{Control>}{Enter}{/Control}");
  await waitFor(() => expect(sent).toEqual([{ description: null }]));
});
it("saves fractional money and ARR together in the chosen currency", async () => {
  const user = userEvent.setup();
  const { sent } = mount();
  await user.click(await screen.findByRole("button", { name: "Change Value" }));
  const amount = screen.getByLabelText("Value");
  await user.clear(amount);
  await user.type(amount, "14.60");
  await user.type(screen.getByLabelText("Expected ARR"), "2.50");
  expect(sent).toEqual([]);
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(sent).toEqual([{ amount_minor: 1460, expected_arr_minor: 250 }]),
  );
});
it("does not write an unchanged money group", async () => {
  const user = userEvent.setup();
  const { sent } = mount();
  await user.click(await screen.findByRole("button", { name: "Change Value" }));
  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(sent).toEqual([]);
  expect(screen.queryByRole("spinbutton")).toBeNull();
});
it("locks offer-derived ARR and currency while permitting an amount correction", async () => {
  const user = userEvent.setup();
  const { sent } = mount({
    ...deal,
    expected_arr_minor: 120000,
    arr_source_offer_id: "offer1",
  });
  await user.click(await screen.findByRole("button", { name: "Change Value" }));
  const input = screen.getByRole("spinbutton", { name: "Value" });
  await user.clear(input);
  await user.type(input, "49000{Enter}");
  await waitFor(() => expect(sent).toEqual([{ amount_minor: 4900000 }]));
  expect(
    screen.queryByRole("button", { name: "Change Expected ARR" }),
  ).toBeNull();
});

it("names masked money as withheld while leaving unrelated fields editable", async () => {
  mount({ ...deal, amount_minor: undefined, masked_fields: ["amount_minor"] });
  expect(await screen.findByText("Not shown")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Change Value" })).toBeNull();
  expect(
    await screen.findByRole("button", { name: "Change Deal brief" }),
  ).toBeTruthy();
});
