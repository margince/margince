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

afterEach(cleanup);

function mount(grant: ("read" | "update")[], current: Deal = deal) {
  const sent: Sent[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: grant }, { seat: "full" }),
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
  render(
    <StoryProviders>
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
  expect(labels(registration)).toEqual([en["deal.acquisitionSource"]]);
});

it("saves a changed value as that field alone, in one PATCH against the version shown", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  const value = await openSection(user, "Value");
  const amount = within(value).getByRole("spinbutton", { name: "Value" });
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

it("shows the ARR an offer set in its currency, locked with the reason", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], { ...deal, arr_source_offer_id: "o1" });
  const value = await openSection(user, "Value");
  expect(within(value).getByRole("spinbutton", { name: "Value" })).toBeTruthy();
  expect(
    within(value).queryByRole("spinbutton", { name: "Expected ARR" }),
  ).toBeNull();
  const arr = within(value).getByRole("group", { name: "Expected ARR" });
  expect(arr).toHaveTextContent("€12,000");
  expect(arr).toHaveAccessibleDescription(en["deal.arrFromOffer"]);
});

it("keeps a figure the reader may not see locked as not shown", async () => {
  const user = userEvent.setup();
  mount(["read", "update"], { ...deal, masked_fields: ["amount_minor"] });
  const value = await openSection(user, "Value");
  expect(within(value).queryByRole("spinbutton", { name: "Value" })).toBeNull();
  expect(
    within(value).getByRole("group", { name: "Value" }),
  ).toHaveAccessibleDescription(en["record.notShown"]);
});
