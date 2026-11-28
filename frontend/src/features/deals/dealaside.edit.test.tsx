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
import "@testing-library/jest-dom/vitest";
import type { components } from "../../api/schema";
import { FORM_DIALOG_MAX_FIELDS } from "../../design-system/modal";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

// Each details section of the deal offers one Edit, which opens only the
// fields that section shows and saves them through the deal's one write.

type Deal = components["schemas"]["Deal"];

const deal: Deal = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 4,
  pipeline_id: "p1",
  stage_id: "s1",
  currency: "EUR",
  amount_minor: 4800000,
  expected_arr_minor: 1200000,
  expected_close_date: "2026-12-01",
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

type Sent = Readonly<{ ifMatch: string | null; body: unknown }>;

let view: ReturnType<typeof render>;

afterEach(cleanup);

function mount(
  grant: ("read" | "update")[],
  current: Deal = deal,
  locale: "en" | "de" = "en",
  tags: () => Response = () => jsonResponse({ data: [] }),
  seats: readonly {
    id: string;
    display_name: string;
    departed: boolean;
  }[] = [],
) {
  const sent: Sent[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: grant }, { seat: "full" }),
    "GET /records/deal/d1/tags": tags,
    "GET /users/names": () => jsonResponse({ data: seats }),
    "GET /users": () =>
      jsonResponse({
        data: seats.filter((seat) => !seat.departed),
        page: { has_more: false, next_cursor: null },
      }),
  });
  const stub = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    const request = input instanceof Request ? input : null;
    if ((request?.method ?? init?.method) !== "PATCH") return stub(input, init);
    const headers = request?.headers ?? new Headers(init?.headers);
    const text = request ? await request.text() : String(init?.body);
    sent.push({ ifMatch: headers.get("If-Match"), body: JSON.parse(text) });
    return jsonResponse({ ...current, version: 5 });
  };
  view = render(
    <StoryProviders locale={locale}>
      <DealAside deal={current} stages={[]} />
    </StoryProviders>,
  );
  return sent;
}

async function openSection(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(await screen.findByRole("button", { name: `Edit ${name}` }));
  return screen.findByRole("dialog", { name });
}

function labels(dialog: HTMLElement): string[] {
  return (
    [...dialog.querySelectorAll("label, legend")]
      // A required field's label ends in its marker, which is not its name.
      .map((label) => label.textContent?.replace(/\s*\*$/, "").trim() ?? "")
      .filter(Boolean)
  );
}

it("offers one Edit per section to a reader who may change the deal", async () => {
  mount(["read", "update"]);
  for (const section of ["About", "Value", "Ownership", "Registration"]) {
    expect(
      await screen.findByRole("button", { name: `Edit ${section}` }),
    ).toHaveTextContent(en["record.edit"]);
  }
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
  expect(document.querySelector(".editable-value")).toBeNull();
});

it("offers no Edit to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByRole("region", { name: "Value" });
  expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
});

it("opens About and Value each with only the fields that section shows", async () => {
  const user = userEvent.setup();
  mount(["read", "update"]);
  const about = await openSection(user, "About");
  expect(labels(about)).toEqual([
    en["create.expectedClose"],
    en["deal.winProbability"],
    en["deal.waitUntil"],
    en["deal.motion"],
    en["deal.priority"],
  ]);
  await user.click(within(about).getByRole("button", { name: "Cancel" }));
  const value = await openSection(user, "Value");
  expect(labels(value)).toEqual([
    en["create.amount"],
    en["deal.expectedArr"],
    en["create.currency"],
  ]);
});

it("keeps every section within a small dialog's fields", async () => {
  const user = userEvent.setup();
  mount(["read", "update"]);
  for (const section of ["About", "Value", "Ownership", "Registration"]) {
    const dialog = await openSection(user, section);
    expect(labels(dialog).length).toBeLessThanOrEqual(FORM_DIALOG_MAX_FIELDS);
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  }
});

it("opens Ownership with the owner and Registration with the acquisition source", async () => {
  const user = userEvent.setup();
  mount(["read", "update"]);
  const ownership = await openSection(user, "Ownership");
  expect(labels(ownership)).toEqual([en["list.owner"]]);
  await user.click(within(ownership).getByRole("button", { name: "Cancel" }));
  const registration = await openSection(user, "Registration");
  expect(labels(registration)).toEqual([
    en["deal.acquisitionSource"],
    en["tags.panelTitle"],
  ]);
});

it("saves a changed value as that field alone, in one PATCH against the version shown", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  const value = await openSection(user, "Value");
  const amount = within(value).getByRole("textbox", { name: "Value" });
  await user.clear(amount);
  await user.type(amount, "52000");
  await user.click(within(value).getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(sent).toEqual([
      {
        ifMatch: expect.stringContaining("4"),
        body: { amount_minor: 5200000 },
      },
    ]),
  );
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "Value" })).toBeNull(),
  );
});

it("leaves a closed deal's close day and probability out of About", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], {
    ...deal,
    status: "won",
    closed_at: "2026-10-01T00:00:00Z",
  });
  const about = await openSection(user, "About");
  expect(labels(about)).not.toContain(en["create.expectedClose"]);
  expect(labels(about)).not.toContain(en["deal.winProbability"]);
  expect(labels(about)).toContain(en["deal.waitUntil"]);
});

it("names an open deal's close day as the forecast field and a decided one's as its close", async () => {
  mount(["read", "update"], { ...deal, expected_close_date: "2026-12-01" });
  expect(await screen.findByText(en["deals.close"])).toBeTruthy();
  cleanup();
  mount(["read", "update"], {
    ...deal,
    status: "won",
    closed_at: "2026-10-01T00:00:00Z",
  });
  // A decided deal's day is no forecast, so it is not called one.
  expect(await screen.findByText(en["deal.standing.close"])).toBeTruthy();
  expect(screen.queryByText(en["deals.close"])).toBeNull();
});

it("shows the ARR an offer set in its currency, locked with the reason", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], { ...deal, arr_source_offer_id: "o1" });
  const value = await openSection(user, "Value");
  expect(within(value).getByRole("textbox", { name: "Value" })).toBeTruthy();
  expect(
    within(value).queryByRole("textbox", { name: en["deal.expectedArr"] }),
  ).toBeNull();
  const arr = within(value).getByRole("group", {
    name: en["deal.expectedArr"],
  });
  expect(arr).toHaveTextContent("€12,000");
  expect(arr).toHaveAccessibleDescription(en["deal.arrFromOffer"]);
});

// A masked currency goes alone, and a figure means nothing without the currency of its minor units.
it("offers no money to change beside a masked currency", async () => {
  const shown: Deal = { ...deal, masked_fields: ["currency"] };
  delete shown.currency;
  mount(["read", "update"], shown);
  await screen.findByRole("button", { name: "Edit About" });
  expect(screen.queryByRole("button", { name: "Edit Value" })).toBeNull();
});

const fleet = () =>
  jsonResponse({
    data: [
      {
        tag_id: "t-1",
        name: "Fleet",
        archived: false,
        assigned_at: "2026-01-02T00:00:00Z",
      },
    ],
    withheld: false,
  });

it("changes tags through Registration's Edit, leaving the row a reading", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], deal, "en", fleet);
  const registration = await screen.findByRole("region", {
    name: en["record.registration"],
  });
  expect(await within(registration).findByText("Fleet")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Add tag" })).toBeNull();
  const dialog = await openSection(user, "Registration");
  await user.click(within(dialog).getByRole("button", { name: "Change Tags" }));
  const editor = await screen.findByRole("dialog", {
    name: en["tags.panelTitle"],
  });
  expect(within(editor).getByRole("button", { name: "Add tag" })).toBeTruthy();
  expect(
    screen.queryByRole("dialog", { name: en["record.registration"] }),
  ).toBeNull();
});

it("offers a reader without the update grant no tag verb", async () => {
  mount(["read"], deal, "en", fleet);
  const registration = await screen.findByRole("region", {
    name: en["record.registration"],
  });
  expect(await within(registration).findByText("Fleet")).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Edit Registration" }),
  ).toBeNull();
  expect(screen.queryByRole("button", { name: "Add tag" })).toBeNull();
});

it("says withheld tags are withheld rather than none", async () => {
  mount(["read", "update"], deal, "en", () =>
    jsonResponse({ data: [], withheld: true }),
  );
  const registration = await screen.findByRole("region", {
    name: en["record.registration"],
  });
  expect(
    await within(registration).findByText(en["tags.withheld"]),
  ).toBeTruthy();
});

it("claims nothing about the tags when their read fails", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], deal, "en", () =>
    jsonResponse({ detail: "down" }, 500),
  );
  const dialog = await openSection(user, "Registration");
  await waitFor(() =>
    expect(labels(dialog)).toEqual([en["deal.acquisitionSource"]]),
  );
  expect(
    screen.queryByText(en["tags.panelTitle"], { selector: "dt" }),
  ).toBeNull();
});

// German groups thousands with a dot: "12,5" is twelve and a half, and
// "1.500" is refused because it also reads as fifteen hundred.
it.each([
  ["12,5", [{ amount_minor: 1250 }]],
  ["1.500", []],
])("in German, a typed %s saves %j", async (typed, want) => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"], deal, "de");
  await user.click(
    await screen.findByRole("button", { name: "Wert bearbeiten" }),
  );
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
  await waitFor(() => expect(sent.map((one) => one.body)).toEqual(want));
});

it("refuses a close date the server cannot store before sending it", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"], {
    ...deal,
    expected_close_date: "2026-09-01",
  });
  const dialog = await openSection(user, "About");
  const day = within(dialog).getByRole("textbox", {
    name: en["create.expectedClose"],
  });
  await user.clear(day);
  await user.type(day, "31/12/9999");
  await user.tab();
  expect(
    await within(dialog).findByText(
      "Expected close must be from 02/01/0001 to 30/12/9999.",
    ),
  ).toBeTruthy();
  expect(day).toHaveAttribute("aria-invalid", "true");
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
  expect(sent).toEqual([]);
});

const gone = [{ id: "u-gone", display_name: "Shay Left", departed: true }];
const here = [{ id: "u-here", display_name: "Ada Here", departed: false }];
const leftNote = "Shay Left left. This needs a new owner.";

it("says the owner left and opens Ownership from the note's action", async () => {
  const user = userEvent.setup();
  mount(
    ["read", "update"],
    { ...deal, owner_id: "u-gone" },
    "en",
    undefined,
    gone,
  );
  expect(await screen.findByText(leftNote)).toBeTruthy();
  await user.click(
    screen.getByRole("button", { name: en["deal.owner.choose"] }),
  );
  expect(await screen.findByRole("dialog", { name: "Ownership" })).toBeTruthy();
});

it("gives a reader who may not edit the sentence and no action", async () => {
  mount(
    ["read"],
    { ...deal, owner_id: "u-gone", writable: false },
    "en",
    undefined,
    gone,
  );
  expect(await screen.findByText(leftNote)).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: en["deal.owner.choose"] }),
  ).toBeNull();
});

it("says nothing for an owner who is still here or a deal with no owner", async () => {
  mount(
    ["read", "update"],
    { ...deal, owner_id: "u-here" },
    "en",
    undefined,
    here,
  );
  await screen.findByRole("region", { name: "Ownership" });
  await screen.findByText("Ada Here");
  expect(screen.queryByText(/This needs a new owner/)).toBeNull();
  cleanup();
  mount(["read", "update"], deal, "en", undefined, gone);
  await screen.findByRole("region", { name: "Ownership" });
  expect(screen.queryByText(/This needs a new owner/)).toBeNull();
});

it("shows no note on an archived deal, which cannot be handed over", async () => {
  mount(
    ["read", "update"],
    { ...deal, owner_id: "u-gone", archived_at: "2026-10-01T00:00:00Z" },
    "en",
    undefined,
    gone,
  );
  await screen.findByRole("region", { name: "Ownership" });
  await screen.findByText(/Shay Left/);
  expect(screen.queryByText(/This needs a new owner/)).toBeNull();
});

it("drops the note once the deal is saved with a new owner", async () => {
  const user = userEvent.setup();
  const sent = mount(
    ["read", "update"],
    { ...deal, owner_id: "u-gone" },
    "en",
    undefined,
    [...gone, ...here],
  );
  await user.click(
    await screen.findByRole("button", { name: en["deal.owner.choose"] }),
  );
  const dialog = await screen.findByRole("dialog", { name: "Ownership" });
  await user.click(within(dialog).getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: /Ada Here/ }));
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]?.body).toEqual({ owner_id: "u-here" });
  view.rerender(
    <StoryProviders locale="en">
      <DealAside
        deal={{ ...deal, owner_id: "u-here", version: 5 }}
        stages={[]}
      />
    </StoryProviders>,
  );
  await waitFor(() =>
    expect(screen.queryByText(/This needs a new owner/)).toBeNull(),
  );
});
