import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { view } from "../../testing/contactpage.testkit";
import { StoryProviders } from "../../testing/story-utils";
import { ContactOpening } from "./contactopening";

const ID = "c-1";

/**
 * The component reads the name out of the list's cache, so a story has to put a
 * list there. `StoryProviders` owns its own client, so this nests a seeded one
 * inside it: the inner provider is what descendants see.
 */
function withCachedList(rows: { id: string; full_name: string }[]): Decorator {
  return (Story) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["contacts", "story-reader", {}], {
      pages: [{ data: rows }],
    });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

const meta: Meta<typeof ContactOpening> = {
  title: "Records/Contact 360/Opening head",
  component: ContactOpening,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ContactOpening>;

// Opened from a list: the name is on screen before the record's own read
// answers, which is the whole point of the component.
export const OpenedFromAList: Story = {
  args: { id: ID },
  decorators: [withCachedList([{ id: ID, full_name: "Anna Weber" }])],
};

// Opened with no list behind it — a pasted address, a reload, a link from mail.
// No confirmed row or cached list name has arrived yet.
export const OpenedWithNothingCached: Story = {
  args: { id: ID },
  decorators: [withCachedList([])],
};

// A cached list that holds OTHER contacts but not this one: the same case as an
// empty cache, and worth seeing separately because it is the one a reader
// expects to seed and it must not.
export const OpenedOnARecordTheListDoesNotHold: Story = {
  args: { id: ID },
  decorators: [withCachedList([{ id: "c-other", full_name: "Bo Lind" }])],
};

export const OpenedFromAListDark: Story = {
  args: { id: ID },
  decorators: [withCachedList([{ id: ID, full_name: "Anna Weber" }])],
  globals: { theme: "dark" },
};

export const OpenedWithNothingCachedDark: Story = {
  args: { id: ID },
  decorators: [withCachedList([])],
  globals: { theme: "dark" },
};

const confirmed = {
  ...view.contact,
  id: ID,
  full_name: "Anna Weber",
  title: "Buyer",
};

export const ConfirmedWhileSectionsLoad: Story = {
  args: { id: ID, contact: confirmed },
};

export const ConfirmedWithoutTitle: Story = {
  args: { id: ID, contact: { ...confirmed, title: null } },
};

export const ConfirmedWithProviderTitle: Story = {
  args: {
    id: ID,
    contact: {
      ...confirmed,
      bought_fields: [
        {
          target: "title",
          provider: "surfe",
          paid: true,
          trigger: "manual",
          applied_at: "2026-06-01T08:00:00Z",
        },
      ],
    },
  },
};

export const RefusedWhileRowResolves: Story = {
  args: { id: ID, hideIdentity: true },
  decorators: [withCachedList([{ id: ID, full_name: "Anna Weber" }])],
};

export const ConfirmedWhileSectionsLoadDark: Story = {
  ...ConfirmedWhileSectionsLoad,
  globals: { theme: "dark" },
};

export const ConfirmedWithoutTitleDark: Story = {
  ...ConfirmedWithoutTitle,
  globals: { theme: "dark" },
};

export const ConfirmedWithProviderTitleDark: Story = {
  ...ConfirmedWithProviderTitle,
  globals: { theme: "dark" },
};

export const RefusedWhileRowResolvesDark: Story = {
  ...RefusedWhileRowResolves,
  globals: { theme: "dark" },
};
