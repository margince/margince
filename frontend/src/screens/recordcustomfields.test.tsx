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
import { CustomFieldsPanel } from "./customfields.card";
import { RecordCustomFields } from "./recordcustomfields";
import { customFieldFixture } from "./recordcustomfields.stories";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(cleanup);
const budget = {
  ...customFieldFixture,
  id: "f3",
  label: "Annual budget",
  slug: "budget",
  type: "currency" as const,
  currency: "EUR",
  column_name: "cf_budget",
};
const wiki = {
  ...customFieldFixture,
  id: "f4",
  label: "Account wiki",
  slug: "wiki",
  column_name: "cf_wiki",
};
const WIKI = "https://wiki.example.com/globex";

function catalog(
  fields: unknown[],
  kind: "company" | "contact" | "deal" | "lead" = "contact",
) {
  const path = kind === "company" ? "companies" : `${kind}s`;
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute(
      { [kind]: ["read", "update"] as const },
      { seat: "full" },
    ),
    "GET /custom-fields": () =>
      jsonResponse({ data: fields, page: { has_more: false } }),
    [`PATCH /${path}/record1`]: (body) => {
      sent.push(body);
      return jsonResponse({ id: "record1", version: 2 });
    },
  });
  return sent;
}

it.each(["company", "contact", "deal", "lead"] as const)(
  "reads money in the field's currency and a web address as a link on a %s",
  async (kind) => {
    catalog(
      [
        { ...budget, object: kind },
        { ...wiki, object: kind },
      ],
      kind,
    );
    render(
      <StoryProviders>
        <RecordCustomFields
          kind={kind}
          record={{
            id: "record1",
            version: 1,
            writable: true,
            cf_budget: 4_800_000,
            cf_wiki: WIKI,
          }}
        />
      </StoryProviders>,
    );
    expect(
      (await screen.findByRole("button", { name: "Change Annual budget" }))
        .textContent,
    ).toBe("€48,000.00");
    expect(screen.getByRole("link", { name: WIKI }).getAttribute("href")).toBe(
      WIKI,
    );
    expect(
      screen.getByRole("button", { name: "Change Account wiki" }),
    ).toBeTruthy();
  },
);

it("edits the major-unit amount and the address itself, not their readings", async () => {
  const user = userEvent.setup();
  const sent = catalog([budget, wiki]);
  render(
    <StoryProviders>
      <RecordCustomFields
        kind="contact"
        record={{
          id: "record1",
          version: 1,
          writable: true,
          cf_budget: 4_800_000,
          cf_wiki: WIKI,
        }}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Annual budget" }),
  );
  const amount = screen.getByRole("spinbutton", { name: "Annual budget" });
  expect((amount as HTMLInputElement).value).toBe("48000");
  await user.clear(amount);
  await user.type(amount, "48000.5{Enter}");
  await waitFor(() => expect(sent).toEqual([{ cf_budget: 4_800_050 }]));

  screen.getByRole("link", { name: WIKI }).focus();
  await user.tab();
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Change Account wiki" }),
  );
  await user.keyboard("{Enter}");
  expect(
    (screen.getByRole("textbox", { name: "Account wiki" }) as HTMLInputElement)
      .value,
  ).toBe(WIKI);
  expect(screen.queryByRole("link", { name: WIKI })).toBeNull();
  await user.keyboard("{Escape}");
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Change Account wiki" }),
  );
  expect(screen.getByRole("link", { name: WIKI })).toBeTruthy();
});

it("reads an unset money or address field as unset, with nothing to follow", async () => {
  catalog([budget, wiki]);
  render(
    <StoryProviders>
      <RecordCustomFields
        kind="contact"
        record={{ id: "record1", version: 1, writable: true }}
      />
    </StoryProviders>,
  );
  expect(
    (await screen.findByRole("button", { name: "Change Annual budget" }))
      .textContent,
  ).toBe("Not set");
  expect(
    screen.getByRole("button", { name: "Change Account wiki" }).textContent,
  ).toBe("Not set");
  expect(screen.queryByRole("link")).toBeNull();
});

it.each(["company", "contact", "deal", "lead"] as const)(
  "shows an unset custom field and sends a sparse %s patch",
  async (kind) => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    const path = kind === "company" ? "companies" : `${kind}s`;
    installFetchStub({
      "GET /me": meRoute(
        { [kind]: ["read", "update"] as const },
        { seat: "full" },
      ),
      "GET /custom-fields": () =>
        jsonResponse({
          data: [
            { ...customFieldFixture, object: kind },
            {
              ...customFieldFixture,
              id: "f2",
              column_name: "cf_other",
              label: "Other",
            },
          ],
          page: { has_more: false },
        }),
      [`PATCH /${path}/record1`]: (body) => {
        sent.push(body);
        return jsonResponse({ id: "record1", version: 2 });
      },
    });
    render(
      <StoryProviders>
        <RecordCustomFields
          kind={kind}
          record={{
            id: "record1",
            version: 1,
            writable: true,
            cf_other: "Keep",
          }}
        />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Change Customer tier" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Customer tier" }),
      "Strategic{Enter}",
    );
    await waitFor(() => expect(sent).toEqual([{ cf_tier: "Strategic" }]));
  },
);
it("clears an existing value and never offers a masked field for editing", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /custom-fields": () =>
      jsonResponse({
        data: [
          customFieldFixture,
          {
            ...customFieldFixture,
            id: "f2",
            column_name: "cf_hidden",
            label: "Hidden",
          },
        ],
        page: { has_more: false },
      }),
    "PATCH /contacts/c1": (body) => {
      sent.push(body);
      return jsonResponse({ id: "c1", version: 2 });
    },
  });
  render(
    <StoryProviders>
      <RecordCustomFields
        kind="contact"
        record={{
          id: "c1",
          version: 1,
          writable: true,
          cf_tier: "Strategic",
          masked_fields: ["cf_hidden"],
        }}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Customer tier" }),
  );
  await user.clear(screen.getByRole("textbox", { name: "Customer tier" }));
  await user.keyboard("{Enter}");
  await waitFor(() => expect(sent).toEqual([{ cf_tier: null }]));
  expect(screen.queryByRole("button", { name: "Change Hidden" })).toBeNull();
  expect(screen.getByText("Hidden").nextElementSibling?.textContent).toBe(
    "Not shown",
  );
});

function typed(
  type: (typeof customFieldFixture)["type"],
  label: string,
  options?: string[],
) {
  return {
    ...customFieldFixture,
    id: `f-${type}`,
    label,
    type,
    options,
    column_name: `cf_${type}`,
  };
}
const EVERY_TYPE = [
  customFieldFixture,
  wiki,
  budget,
  typed("number", "Fleet"),
  typed("date", "Renewal"),
  typed("picklist", "Region", ["North", "West"]),
  typed("multiselect", "Depots", ["North", "West"]),
  typed("boolean", "Framework"),
].map((field) => ({ ...field, object: "deal" as const }));
// One value per type in the shape the record API sends it.
const WIRE = {
  cf_tier: "Strategic",
  cf_wiki: WIKI,
  cf_budget: 4_800_000,
  cf_number: "220",
  cf_date: "2026-03-01",
  cf_picklist: "North",
  cf_multiselect: ["North", "West"],
  cf_boolean: true,
};

it.each([false, true])(
  "reads every type's wire value as the projects panel does (writable: %s)",
  async (writable) => {
    catalog(EVERY_TYPE, "deal");
    render(
      <StoryProviders>
        <div data-testid="details">
          <RecordCustomFields
            kind="deal"
            record={{ id: "record1", version: 1, writable, ...WIRE }}
          />
        </div>
        <div data-testid="panel">
          <CustomFieldsPanel object="deal" record={WIRE} />
        </div>
      </StoryProviders>,
    );
    const details = within(screen.getByTestId("details"));
    const panel = within(screen.getByTestId("panel"));
    await details.findByText("Customer tier");
    await panel.findByText("Customer tier");
    const readings = EVERY_TYPE.map(({ label }) => {
      const cell = details.getByText(label).nextElementSibling;
      return writable
        ? cell?.firstElementChild?.textContent
        : cell?.textContent;
    });
    expect(readings).toEqual(
      EVERY_TYPE.map(
        ({ label }) => panel.getByText(label).nextElementSibling?.textContent,
      ),
    );
    expect(readings.slice(1, 3)).toEqual([WIKI, "€48,000.00"]);
  },
);
