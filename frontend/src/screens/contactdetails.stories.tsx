import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../api/schema";
import { ContactDetails } from "./contactdetails";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const meta: Meta<typeof ContactDetails> = {
  title: "Records/Contact 360/Details",
  component: ContactDetails,
  decorators: [
    (Story, { parameters }) => (
      <StoryProviders locale={parameters.locale === "de" ? "de" : "en"}>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () =>
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /users": () => jsonResponse({ data: [] }),
    }),
};
export default meta;
type Story = StoryObj<typeof ContactDetails>;
const fixture: components["schemas"]["Contact"] = {
  id: "c1",
  full_name: "Dana Buyer",
  title: "Director",
  writable: true,
  version: 1,
  visibility: "workspace",
  emails: [],
  phones: [],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
export const Editable: Story = {
  args: {
    contact: fixture,
  },
};
export const ReadOnly: Story = {
  args: {
    contact: {
      ...fixture,
      id: "c1",
      full_name: "Dana Buyer",
      writable: false,
    },
  },
};

// A contact a purchase filled: the title, profile link, one address and one
// number carry the "bought" mark beside them, the typed address does not, and
// the location and departments the provider sold stand in their own row.
const boughtFixture: components["schemas"]["Contact"] = {
  ...fixture,
  title: "Head of Revenue",
  social: { linkedin: "https://www.linkedin.com/in/dana" },
  emails: [
    {
      id: "e-bought",
      email: "dana@bought.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "surfe",
      captured_by: "connector:surfe",
    },
    {
      id: "e-typed",
      email: "dana@typed.example",
      email_type: "personal",
      is_primary: false,
      position: 1,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  bought_fields: [
    { target: "title", provider: "surfe", applied_at: "2026-06-02T12:00:00Z" },
    {
      target: "linkedin",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
    {
      target: "email:e-bought",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
  ],
};
const boughtProfiles: components["schemas"]["ContactProviderProfile"][] = [
  {
    provider: "surfe",
    state: "completed",
    categories_not_requested: [],
    emails: [],
    mobile_phones: [],
    job_history: [],
    departments: ["Sales"],
    seniorities: [],
    location: "Munich, Germany",
    attributes: [
      {
        kind: "location",
        value: "Munich, Germany",
        retrieved_at: "2026-06-02T12:00:00Z",
      },
      {
        kind: "department",
        value: "Sales",
        retrieved_at: "2026-03-02T12:00:00Z",
      },
    ],
  },
];
export const BoughtValues: Story = {
  args: { contact: boughtFixture, profiles: boughtProfiles },
};
export const BoughtValuesDark: Story = {
  args: { contact: boughtFixture, profiles: boughtProfiles },
  globals: { theme: "dark" },
};

const longAddress = (local: string) =>
  `${local}@very-long-company-domain-example.com`;
const manual = { source: "manual", captured_by: "human:u1" };
const longFixture: components["schemas"]["Contact"] = {
  ...fixture,
  emails: [
    {
      ...manual,
      id: "e-1",
      email: longAddress("alexandra.konstantinopoulou"),
      email_type: "work",
      is_primary: true,
      position: 0,
    },
    {
      ...manual,
      id: "e-2",
      email: longAddress("alexandra.k.private.mailbox"),
      email_type: "personal",
      is_primary: false,
      position: 1,
    },
    {
      ...manual,
      id: "e-3",
      email: longAddress("a.konstantinopoulou.assistant"),
      email_type: "other",
      is_primary: false,
      position: 2,
    },
    {
      ...manual,
      id: "e-4",
      email: longAddress("konstantinopoulou.alexandra"),
      email_type: "work",
      is_primary: false,
      position: 3,
    },
  ],
  phones: [
    {
      ...manual,
      id: "p-1",
      phone: "+4915112345678",
      phone_type: "mobile",
      is_primary: true,
      position: 0,
    },
  ],
  bought_fields: [
    {
      target: "email:e-1",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
  ],
};

async function expectHandlesInColumn(
  canvasElement: HTMLElement,
  kinds: "beside" | "mixed",
) {
  await within(canvasElement).findByText(
    longAddress("alexandra.konstantinopoulou"),
  );
  const handles = [
    ...canvasElement.querySelectorAll<HTMLElement>(".fieldgrid-handle"),
  ];
  await expect(handles).toHaveLength(5);
  for (const handle of handles) {
    const column = handle.closest(".fieldgrid-value");
    await expect(handle.getBoundingClientRect().right).toBeLessThanOrEqual(
      (column?.getBoundingClientRect().right ?? 0) + 0.5,
    );
  }
  const floor =
    5 * Number.parseFloat(getComputedStyle(document.documentElement).fontSize);
  let dropped = 0;
  let grouped = 0;
  for (const handle of handles) {
    const value = handle.firstElementChild?.getBoundingClientRect();
    const kind = handle.querySelector<HTMLElement>(":scope > .t-caption");
    if (!value || !kind) {
      throw new Error("a handle lost its value or its kind");
    }
    const lines = document.createRange();
    lines.selectNodeContents(kind);
    await expect(lines.getClientRects()).toHaveLength(1);
    const placed = kind.getBoundingClientRect();
    if (placed.top < value.bottom) {
      await expect(placed.left - value.right).toBeCloseTo(
        Number.parseFloat(getComputedStyle(handle).columnGap),
        0,
      );
      await expect(value.width).toBeGreaterThanOrEqual(floor - 0.5);
      continue;
    }
    dropped += 1;
    const next = handle.nextElementSibling;
    if (next?.classList.contains("fieldgrid-handle")) {
      grouped += 1;
      await expect(placed.top - value.bottom).toBeLessThan(
        next.getBoundingClientRect().top -
          handle.getBoundingClientRect().bottom,
      );
    }
  }
  if (kinds === "beside") {
    await expect(dropped).toBe(0);
  } else {
    await expect(grouped).toBeGreaterThan(0);
  }
  const first = handles[0].firstElementChild?.getBoundingClientRect();
  const firstKind = handles[0]
    .querySelector(".t-caption")
    ?.getBoundingClientRect();
  await expect(first?.height).toBeGreaterThan((firstKind?.height ?? 0) * 1.5);
}

export const LongAddresses: Story = {
  args: { contact: longFixture },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) => expectHandlesInColumn(canvasElement, "beside"),
};
export const LongAddressesDark: Story = {
  ...LongAddresses,
  globals: { theme: "dark" },
};
export const LongAddressesPhone: Story = {
  args: { contact: longFixture },
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    await expectHandlesInColumn(canvasElement, "beside");
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
  },
};
// The card a 320px phone leaves, where "Geschäftlich" and "gekauft" outgrow
// the value column unless they drop beneath the address.
export const LongAddressesNarrowGerman: Story = {
  args: { contact: longFixture },
  parameters: { locale: "de" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 288 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) => expectHandlesInColumn(canvasElement, "mixed"),
};
// A column that holds "Geschäftlich gekauft" beside an address only a few
// letters wide.
export const LongAddressesGerman: Story = {
  ...LongAddressesNarrowGerman,
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 330 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) => expectHandlesInColumn(canvasElement, "beside"),
};
