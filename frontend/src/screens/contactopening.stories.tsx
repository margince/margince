import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ContactOpening } from "./contactopening";
import { StoryProviders } from "./story-utils";

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
    client.setQueryData(["contacts", {}], { pages: [{ data: rows }] });
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
// The placeholder it always was, because there is no identity to show.
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
