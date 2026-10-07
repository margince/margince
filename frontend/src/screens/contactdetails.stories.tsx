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
    (Story) => (
      <StoryProviders>
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

const longEmail = (
  local: string,
  id: string,
  type: "work" | "personal" | "other",
) => ({
  id,
  email: `${local}@very-long-company-domain-example.com`,
  email_type: type,
  is_primary: id === "e-1",
  position: Number(id.slice(2)) - 1,
  source: "manual",
  captured_by: "human:u1",
});
const longFixture: components["schemas"]["Contact"] = {
  ...fixture,
  emails: [
    longEmail("alexandra.konstantinopoulou", "e-1", "work"),
    longEmail("alexandra.k.private.mailbox", "e-2", "personal"),
    longEmail("a.konstantinopoulou.assistant", "e-3", "other"),
    longEmail("konstantinopoulou.alexandra", "e-4", "work"),
  ],
  phones: [
    {
      id: "p-1",
      phone: "+4915112345678",
      phone_type: "mobile",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  bought_fields: [
    {
      target: "email:e-4",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
  ],
};

// Each kind stays one line beside its value, and the value wraps inside the
// column instead of pushing the row past the card.
async function expectKindsBesideWrappedValues(canvasElement: HTMLElement) {
  await within(canvasElement).findByText(
    "alexandra.konstantinopoulou@very-long-company-domain-example.com",
  );
  const handles = [
    ...canvasElement.querySelectorAll<HTMLElement>(".fieldgrid-handle"),
  ];
  await expect(handles).toHaveLength(5);
  for (const handle of handles) {
    const value = handle.firstElementChild;
    const kind = handle.querySelector<HTMLElement>(":scope > .t-caption");
    const column = handle.closest(".fieldgrid-value");
    if (!value || !kind || !column) {
      throw new Error("a handle lost its value, its kind or its column");
    }
    const lines = document.createRange();
    lines.selectNodeContents(kind);
    await expect(lines.getClientRects()).toHaveLength(1);
    await expect(kind.getBoundingClientRect().left).toBeGreaterThanOrEqual(
      value.getBoundingClientRect().right,
    );
    await expect(handle.getBoundingClientRect().right).toBeLessThanOrEqual(
      column.getBoundingClientRect().right + 0.5,
    );
  }
  const first = handles[0].firstElementChild?.getBoundingClientRect();
  const firstKind = handles[0]
    .querySelector(".t-caption")
    ?.getBoundingClientRect();
  await expect(first?.height).toBeGreaterThan((firstKind?.height ?? 0) * 1.5);
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
    document.documentElement.clientWidth,
  );
}

export const LongAddresses: Story = {
  args: { contact: longFixture },
  // A rail-width card, so a long address outruns its value column.
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) => expectKindsBesideWrappedValues(canvasElement),
};
export const LongAddressesDark: Story = {
  ...LongAddresses,
  globals: { theme: "dark" },
};
export const LongAddressesPhone: Story = {
  args: { contact: longFixture },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: ({ canvasElement }) => expectKindsBesideWrappedValues(canvasElement),
};
