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

type Kind = "company" | "contact" | "deal" | "lead";
function catalog(
  fields: unknown[],
  kind: Kind = "contact",
  held?: Promise<void>,
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
    [`PATCH /${path}/record1`]: async (body) => {
      sent.push(body);
      await held;
      return jsonResponse({ id: "record1", version: 2 });
    },
  });
  return sent;
}

function renderDetails(kind: Kind, values: Record<string, unknown>) {
  return render(
    <StoryProviders>
      <RecordCustomFields
        kind={kind}
        record={{ id: "record1", version: 1, writable: true, ...values }}
      />
    </StoryProviders>,
  );
}

it.each(["company", "contact", "deal", "lead"] as const)(
  "reads money and a web address on a %s, and edits their raw values",
  async (kind) => {
    const user = userEvent.setup();
    const sent = catalog(
      [
        { ...budget, object: kind },
        { ...wiki, object: kind },
      ],
      kind,
    );
    renderDetails(kind, { cf_budget: 4_800_000, cf_wiki: WIKI });
    const change = await screen.findByRole("button", {
      name: "Change Annual budget",
    });
    expect(change.textContent).toBe("€48,000.00");
    expect(screen.getByRole("link", { name: WIKI }).getAttribute("href")).toBe(
      WIKI,
    );
    await user.click(change);
    const amount = screen.getByRole("spinbutton", { name: "Annual budget" });
    expect((amount as HTMLInputElement).value).toBe("48000");
    await user.clear(amount);
    await user.type(amount, "48000.5{Enter}");
    await waitFor(() => expect(sent).toEqual([{ cf_budget: 4_800_050 }]));

    screen.getByRole("link", { name: WIKI }).focus();
    await user.tab();
    const verb = screen.getByRole("button", { name: "Change Account wiki" });
    expect(document.activeElement).toBe(verb);
    await user.keyboard("{Enter}");
    expect(
      (
        screen.getByRole("textbox", {
          name: "Account wiki",
        }) as HTMLInputElement
      ).value,
    ).toBe(WIKI);
    expect(screen.queryByRole("link", { name: WIKI })).toBeNull();
    await user.keyboard("{Escape}");
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Change Account wiki" }),
    );
    expect(screen.getByRole("link", { name: WIKI })).toBeTruthy();
  },
);

it("reads and edits a zero-decimal currency in whole units", async () => {
  const user = userEvent.setup();
  const sent = catalog([{ ...budget, currency: "JPY" }]);
  renderDetails("contact", { cf_budget: 48_000 });
  const change = await screen.findByRole("button", {
    name: "Change Annual budget",
  });
  expect(change.textContent).toBe("JP¥48,000");
  await user.click(change);
  const amount = screen.getByRole("spinbutton", { name: "Annual budget" });
  expect((amount as HTMLInputElement).value).toBe("48000");
  await user.clear(amount);
  await user.type(amount, "48001{Enter}");
  await waitFor(() => expect(sent).toEqual([{ cf_budget: 48_001 }]));
});

it.each([
  ["EUR", "48000.555"],
  ["JPY", "48000.5"],
])(
  "refuses a %s amount finer than the currency, and sends nothing",
  async (currency, typed) => {
    const user = userEvent.setup();
    const sent = catalog([{ ...budget, currency }]);
    renderDetails("contact", { cf_budget: 4_800_000 });
    await user.click(
      await screen.findByRole("button", { name: "Change Annual budget" }),
    );
    const amount = screen.getByRole("spinbutton", { name: "Annual budget" });
    await user.clear(amount);
    await user.type(amount, `${typed}{Enter}`);
    expect(
      await screen.findByText(
        `This amount has more decimals than ${currency} allows.`,
      ),
    ).toBeTruthy();
    expect(amount.getAttribute("aria-invalid")).toBe("true");
    expect(sent).toEqual([]);
  },
);

it("keeps the edit verb in place, disabled, while another field saves", async () => {
  const user = userEvent.setup();
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const sent = catalog([budget, wiki], "contact", held);
  renderDetails("contact", { cf_budget: 4_800_000, cf_wiki: WIKI });
  await user.click(
    await screen.findByRole("button", { name: "Change Annual budget" }),
  );
  await user.type(
    screen.getByRole("spinbutton", { name: "Annual budget" }),
    "1{Enter}",
  );
  await waitFor(() => expect(sent).toHaveLength(1));
  const verb = screen.getByRole("button", { name: "Change Account wiki" });
  expect((verb as HTMLButtonElement).disabled).toBe(true);
  release();
  await waitFor(() => expect((verb as HTMLButtonElement).disabled).toBe(false));
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
  cf_number: 220,
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
