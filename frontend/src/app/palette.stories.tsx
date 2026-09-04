// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../screens/story-utils";
import { type Command, CommandPalette } from "./palette";

// The palette had no story at all. It was mounted CLOSED behind the top bar in
// the shell and top-bar stories — a live seam, so the button opens something —
// which documents the button and none of the surface behind it. Every state
// below is one a reader meets and none of them was reviewable.
const meta: Meta<typeof CommandPalette> = {
  title: "Shell/Command palette",
  component: CommandPalette,
};
export default meta;
type Story = StoryObj<typeof CommandPalette>;

// A representative command set rather than the real `useBuiltinCommands`, which
// would need the whole settings-visibility probe to say anything: these stories
// are about the PANEL, and the command list is its input.
const COMMANDS: Command[] = [
  {
    id: "screen:home",
    label: "Home",
    type: "screen",
    route: { screen: "home" },
  },
  {
    id: "screen:deals",
    label: "Pipeline",
    keywords: ["deals"],
    type: "screen",
    route: { screen: "deals" },
  },
  {
    id: "screen:settings-data-model",
    label: "Data model",
    keywords: ["products", "price list", "custom-fields"],
    type: "screen",
    route: { screen: "settings" },
  },
  {
    id: "action:new-deal",
    label: "New deal",
    type: "action",
    route: { screen: "deals", id: "new" },
  },
];

function palette(hits: () => Response | Promise<Response>) {
  installFetchStub({ "GET /search": hits });
  return (
    <StoryProviders>
      <CommandPalette open onClose={() => undefined} commands={COMMANDS} />
    </StoryProviders>
  );
}

const noHits = () =>
  jsonResponse({ data: [], page: { next_cursor: null, has_more: false } });

// Opened, nothing typed: the whole command list, which is what ⌘K shows first.
export const Default: Story = {
  render: () => palette(noHits),
};

// Live record hits under the commands. The second line names the KIND, in the
// reader's language — this row printed the raw wire word until #4026, so a
// German reader met "organization" here.
export const WithRecordHits: Story = {
  render: () =>
    palette(() =>
      jsonResponse({
        data: [
          { type: "person", id: "p1", title: "Dana Buyer" },
          { type: "organization", id: "o1", title: "Acme GmbH" },
          { type: "product", id: "pr1", title: "Kärcher floor scrubber" },
          { type: "tag", id: "t1", title: "Key account" },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    ),
  play: async ({ canvasElement }) => {
    const input = canvasElement.querySelector("input");
    if (input) {
      // Set through the native setter so React's onChange fires: assigning
      // `.value` alone updates the DOM and leaves the component's state behind.
      const setter = Object.getOwnPropertyDescriptor(
        globalThis.HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(input, "acme");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }
  },
};

// The wait, which is held back 300ms before it draws: a placeholder that
// flashed on every keystroke would report work already done. The story holds
// the read open so the bar is what the screenshot catches.
export const Searching: Story = {
  render: () => palette(() => new Promise<Response>(() => {}) as never),
  play: WithRecordHits.play,
};

// A record search that failed. It says so and keeps the commands usable beside
// it — it used to answer with an empty array, which is the same shape as "no
// matches" and told the reader the workspace holds nothing.
export const SearchFailed: Story = {
  render: () => palette(() => new Response("nope", { status: 500 })),
  play: WithRecordHits.play,
};

// Nothing matched, and nothing went wrong. The one state that may claim there
// is none.
export const NoMatches: Story = {
  render: () => palette(noHits),
  play: async ({ canvasElement }) => {
    const input = canvasElement.querySelector("input");
    if (input) {
      const setter = Object.getOwnPropertyDescriptor(
        globalThis.HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(input, "zzzzz");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }
  },
};
