import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { isLocale } from "../../i18n";
import {
  callMessengersRoute,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactDetails } from "./contactdetails";

const meta: Meta<typeof ContactDetails> = {
  title: "Records/Contact 360/Details",
  component: ContactDetails,
  decorators: [
    (Story, { parameters }) => {
      const asked: unknown = parameters.locale;
      return (
        <StoryProviders
          locale={
            typeof asked === "string" && isLocale(asked) ? asked : undefined
          }
        >
          <Story />
        </StoryProviders>
      );
    },
  ],
  beforeEach: () =>
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /users": () => jsonResponse({ data: [] }),
      "GET /installation/settings": callMessengersRoute(),
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
  primary_email: "dana@bought.example",
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
  primary_email: longAddress("alexandra.konstantinopoulou"),
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

// Indexes into the card's handles: the four addresses, then the number.
type Placement = Readonly<{
  kindsBeneath: readonly number[];
  marksBeneath: readonly number[];
}>;
const allBeside: Placement = { kindsBeneath: [], marksBeneath: [] };

function maxContentWidth(element: HTMLElement): number {
  const probe = element.cloneNode(true);
  if (!(probe instanceof HTMLElement)) {
    throw new Error("a cloned element is no longer an element");
  }
  probe.style.position = "absolute";
  probe.style.width = "max-content";
  probe.style.maxWidth = "none";
  probe.style.whiteSpace = "nowrap";
  element.after(probe);
  const width = probe.getBoundingClientRect().width;
  probe.remove();
  return width;
}

async function expectHandlePlaced(
  handle: HTMLElement,
  index: number,
  placement: Placement,
  floor: number,
) {
  const address = handle.firstElementChild;
  const kind = handle.querySelector<HTMLElement>(":scope > .t-caption");
  if (!(address instanceof HTMLElement) || !kind) {
    throw new Error("a handle lost its address or its kind");
  }
  const lines = document.createRange();
  lines.selectNodeContents(kind);
  await expect(lines.getClientRects()).toHaveLength(1);
  const box = address.getBoundingClientRect();
  const gap = Number.parseFloat(getComputedStyle(handle).columnGap);
  const kindBox = kind.getBoundingClientRect();
  const mark = handle
    .querySelector(":scope > .evmark")
    ?.getBoundingClientRect();
  if (placement.kindsBeneath.includes(index)) {
    await expect(kindBox.top).toBeGreaterThanOrEqual(box.bottom - 0.5);
    await expect(kindBox.left).toBeCloseTo(box.left, 0);
  } else {
    await expect(kindBox.top).toBeLessThan(box.bottom);
    await expect(kindBox.left - box.right).toBeCloseTo(gap, 0);
    await expect(box.width).toBeGreaterThanOrEqual(
      Math.min(floor, maxContentWidth(address)) - 0.5,
    );
  }
  if (mark && placement.marksBeneath.includes(index)) {
    await expect(mark.top).toBeGreaterThanOrEqual(box.bottom - 0.5);
  } else if (mark) {
    await expect(mark.top).toBeLessThan(box.bottom);
    await expect(mark.left - kindBox.right).toBeCloseTo(gap, 0);
  }
  const dropped = [kindBox, mark]
    .flatMap((piece) => (piece ? [piece.top] : []))
    .filter((top) => top >= box.bottom - 0.5);
  const next = handle.nextElementSibling;
  if (dropped.length > 0 && next?.classList.contains("fieldgrid-handle")) {
    await expect(Math.min(...dropped) - box.bottom).toBeLessThan(
      next.getBoundingClientRect().top - handle.getBoundingClientRect().bottom,
    );
  }
}

async function expectHandlesInColumn(
  canvasElement: HTMLElement,
  placement: Placement,
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
  for (const [index, handle] of handles.entries()) {
    await expectHandlePlaced(handle, index, placement, floor);
  }
  const [firstHandle] = handles;
  const first = firstHandle?.firstElementChild?.getBoundingClientRect();
  const firstKind = firstHandle
    ?.querySelector(".t-caption")
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
  play: ({ canvasElement }) => expectHandlesInColumn(canvasElement, allBeside),
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
    await expectHandlesInColumn(canvasElement, allBeside);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
  },
};
// Too narrow for any German kind beside a 5rem address: every kind drops.
export const LongAddressesNarrowGerman: Story = {
  args: { contact: longFixture },
  parameters: { locale: "de" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 264 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) =>
    expectHandlesInColumn(canvasElement, {
      kindsBeneath: [0, 1, 2, 3, 4],
      marksBeneath: [0],
    }),
};
// Every kind stays beside its address; the bought one's mark drops beneath it.
export const LongAddressesGerman: Story = {
  ...LongAddressesNarrowGerman,
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 340 }}>
        <Story />
      </div>
    ),
  ],
  play: ({ canvasElement }) =>
    expectHandlesInColumn(canvasElement, {
      kindsBeneath: [],
      marksBeneath: [0],
    }),
};

const twoNumbers: components["schemas"]["Contact"] = {
  ...fixture,
  phones: [
    {
      ...manual,
      id: "p-1",
      phone: "+493012345678",
      phone_type: "mobile",
      is_primary: true,
      position: 0,
    },
    {
      ...manual,
      id: "p-2",
      phone: "+84 (0) 912 345 678 90",
      phone_type: "work",
      is_primary: false,
      position: 1,
    },
  ],
};

// Each number keeps one line, its kind beside it or (with no room) under it,
// and its app links under both.
async function expectNumbersOnOneLine(canvasElement: HTMLElement) {
  await within(canvasElement).findAllByRole("link", { name: /WhatsApp/ });
  const handles = [
    ...canvasElement.querySelectorAll<HTMLElement>(".fieldgrid-handle"),
  ].filter((handle) => handle.querySelector(".callaction-chats"));
  await expect(handles).toHaveLength(2);
  for (const handle of handles) {
    const number = handle.querySelector<HTMLElement>("a[href^='tel:']");
    const kind = handle.querySelector<HTMLElement>(":scope > .t-caption");
    const chats = handle.querySelector<HTMLElement>(".callaction-chats");
    if (!number || !kind || !chats) {
      throw new Error("a phone handle lost its number, kind or app links");
    }
    const line = document.createRange();
    line.selectNodeContents(number);
    await expect(line.getClientRects()).toHaveLength(1);
    const box = number.getBoundingClientRect();
    const kindBox = kind.getBoundingClientRect();
    await expect(chats.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      Math.max(box.bottom, kindBox.bottom) - 0.5,
    );
  }
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
    document.documentElement.clientWidth,
  );
}

export const TwoNumbers: Story = {
  args: { contact: twoNumbers },
  play: ({ canvasElement }) => expectNumbersOnOneLine(canvasElement),
};
export const TwoNumbersPhone: Story = {
  ...TwoNumbers,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const TwoNumbersPhoneGerman: Story = {
  ...TwoNumbersPhone,
  parameters: { layout: "fullscreen", locale: "de" },
};
export const TwoNumbersPhoneVietnamese: Story = {
  ...TwoNumbersPhone,
  parameters: { layout: "fullscreen", locale: "vi" },
};
