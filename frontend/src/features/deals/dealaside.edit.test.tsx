/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

// The deal's left column changes its fields through the details drawer's own
// fields and save.

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

function mount(
  grant: ("read" | "update")[],
  current = deal,
  locale: "en" | "de" = "en",
) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: grant }, { seat: "full" }),
    "GET /records/deal/d1/tags": () => jsonResponse({ data: [] }),
    "PATCH /deals/d1": (body) => {
      sent.push(body);
      return jsonResponse({ ...current, version: 5 });
    },
  });
  render(
    <StoryProviders locale={locale}>
      <DealAside deal={current} stages={[]} />
    </StoryProviders>,
  );
  return sent;
}

it("saves a changed value from the left column's money form as that field alone", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  await user.click(
    await screen.findByRole("button", { name: /^Change Value €48,000$/ }),
  );
  const dialog = await screen.findByRole("dialog", { name: "Value" });
  const amount = within(dialog).getByRole("textbox", { name: "Value" });
  await user.clear(amount);
  await user.type(amount, "52000");
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toEqual([{ amount_minor: 5200000 }]));
});

it("offers no change to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByText("Value");
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
  expect(document.querySelector(".editable-value")).toBeNull();
});

it("changes an open deal's forecast close day and leaves a won deal's close day alone", async () => {
  const open = { ...deal, expected_close_date: "2026-12-01" };
  mount(["read", "update"], open);
  // Named as the create form and Edit details name the field it changes.
  expect(
    await screen.findByRole("button", { name: /^Change Expected close/ }),
  ).toBeTruthy();
  cleanup();
  mount(["read", "update"], {
    ...open,
    status: "won",
    closed_at: "2026-10-01T00:00:00Z",
  });
  await screen.findByRole("button", { name: /^Change Value/ });
  expect(
    screen.queryByRole("button", { name: /^Change (Expected )?Close/ }),
  ).toBeNull();
  // A decided deal's day is no forecast, so it is not called one.
  expect(screen.getByText("Close")).toBeTruthy();
});

it("changes the amount of a deal whose ARR an offer set", async () => {
  mount(["read", "update"], { ...deal, arr_source_offer_id: "o1" });
  expect(
    await screen.findByRole("button", { name: /^Change Value/ }),
  ).toBeTruthy();
});

it("adds a tag from the Tags row and offers no add to a reader", async () => {
  const user = userEvent.setup();
  mount(["read", "update"]);
  await user.click(
    await screen.findByRole("button", { name: /^Show \d+ empty fields$/ }),
  );
  expect(await screen.findByRole("button", { name: "Add tag" })).toBeTruthy();
  cleanup();
  mount(["read"]);
  await user.click(
    await screen.findByRole("button", { name: /^Show \d+ empty fields$/ }),
  );
  await screen.findByText("Tags");
  expect(screen.queryByRole("button", { name: "Add tag" })).toBeNull();
});

// German groups thousands with a dot: "12,5" is twelve and a half, and
// "1.500" is refused because it also reads as fifteen hundred.
it.each([
  ["12,5", [{ amount_minor: 1250 }]],
  ["1.500", []],
])("in German, a typed %s saves %j", async (typed, want) => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"], deal, "de");
  await user.click(await screen.findByRole("button", { name: /^Wert/ }));
  const dialog = await screen.findByRole("dialog", { name: "Wert" });
  const amount = within(dialog).getByRole("textbox", { name: "Wert" });
  expect((amount as HTMLInputElement).value).toBe("48000,00");
  await user.clear(amount);
  await user.type(amount, typed);
  await user.click(within(dialog).getByRole("button", { name: "Speichern" }));
  if (want.length === 0) {
    expect(
      await within(dialog).findByText(
        "Gib den Betrag als 1500 oder 1500,00 ein.",
      ),
    ).toBeTruthy();
  }
  await waitFor(() => expect(sent).toEqual(want));
});

it("refuses a close date the server cannot store before sending it", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"], {
    ...deal,
    expected_close_date: "2026-09-01",
  });
  await user.click(
    await screen.findByRole("button", { name: /^Change Expected close/ }),
  );
  const dialog = await screen.findByRole("dialog");
  const day = within(dialog).getByRole("textbox");
  await user.clear(day);
  await user.type(day, "31/12/9999");
  await user.tab();
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
  expect(
    await within(dialog).findByText(
      "Expected close must be from 02/01/0001 to 30/12/9999.",
    ),
  ).toBeTruthy();
  expect(sent).toEqual([]);
});
