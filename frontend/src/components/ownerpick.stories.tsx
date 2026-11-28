// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en as englishCopy } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { OwnerPick } from "./ownerpick";

// One record handed to a colleague in one pick. The panel is portalled to
// document.body, so each play finds it there after pressing the trigger.

const meta: Meta = {
  title: "Patterns/Change owner",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const VIEWER = meFixture().user.id;

const COLLEAGUES = [
  { id: "u-42", display_name: "Jane Doe", email: "jane@example.test" },
  { id: "u-7", display_name: "Omar Haddad", email: "omar@example.test" },
  { id: VIEWER, display_name: "Lena Brandt", email: "lena@example.test" },
];

const LONG_NAMES = [
  {
    id: "u-42",
    display_name:
      "Maximiliane Josefine von Hohenzollern-Sigmaringen-Wittelsbach",
    email: "maximiliane.vonhohenzollern-sigmaringen@example.test",
  },
  {
    id: "u-7",
    display_name: "Nguyễn Thị Phương Thảo Hoàng Bảo Ngọc Trâm Anh",
    email: "thao.nguyen@example.test",
  },
];

type Roster = "colleagues" | "long" | "empty" | "refused" | "partial";
type Write = "lands" | "holds" | "refused";

function rosterAnswer(roster: Roster, url: URL): Response {
  if (roster === "refused") {
    return jsonResponse(
      {
        title: "Forbidden",
        status: 403,
        detail: "You may not list the workspace’s members.",
      },
      403,
    );
  }
  const listed = {
    colleagues: COLLEAGUES,
    partial: COLLEAGUES,
    long: LONG_NAMES,
    empty: [],
  }[roster];
  return jsonResponse({
    data: url.searchParams.has("cursor") ? [] : listed,
    page: { next_cursor: roster === "partial" ? "more" : null },
  });
}

async function assign(write: Write): Promise<void> {
  if (write === "holds") {
    await new Promise<never>(() => undefined);
  }
  if (write === "refused") {
    throw new ProblemError({
      title: "Conflict",
      status: 409,
      detail: "Someone changed this record while you were reading it.",
    });
  }
}

function Picker({
  roster = "colleagues",
  write = "lands",
  ownerId = "u-7",
  queue = false,
}: Readonly<{
  roster?: Roster;
  write?: Write;
  ownerId?: string | null;
  /** The record may go back to the unassigned queue, as a lead may. */
  queue?: boolean;
}>) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /users": (_, url) => rosterAnswer(roster, url),
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <OwnerPick
          record={{ id: "p-1", version: 3, owner_id: ownerId }}
          assign={() => assign(write)}
          unassign={queue ? () => assign(write) : undefined}
          invalidate="projects"
          recordKey="project"
        />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

async function openList(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: englishCopy["ownerPick.change"],
    }),
  );
  return within(await screen.findByRole("dialog"));
}

async function pickJane(canvasElement: HTMLElement) {
  const list = await openList(canvasElement);
  await userEvent.click(await list.findByRole("option", { name: "Jane Doe" }));
  return list;
}

/** The trigger at rest. */
export const Default: Story = { render: () => <Picker /> };

/** A lead's list: "Unassigned" first, which hands the record back to the queue. */
export const WithUnassigned: Story = {
  render: () => <Picker queue />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await list.findByRole("option", { name: "Unassigned" });
  },
};

/** Opened on the roster: the viewer first, the current owner marked. */
export const Open: Story = {
  render: () => <Picker />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await expect(
      await list.findByRole("option", { name: "Omar Haddad" }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(list.getAllByRole("option")[0]).toHaveTextContent(
      "Lena Brandt",
    );
  },
};
export const OpenDark: Story = { ...Open, globals: { theme: "dark" } };
export const OpenPhone: Story = {
  ...Open,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A colleague picked and the write still out: every row refuses a second pick. */
export const Pending: Story = {
  render: () => <Picker write="holds" />,
  play: async ({ canvasElement }) => {
    const list = await pickJane(canvasElement);
    await expect(list.getByRole("listbox")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  },
};

/** The write landed: the list closed and the toast names who holds it now. */
export const Assigned: Story = {
  render: () => <Picker />,
  play: async ({ canvasElement }) => {
    await pickJane(canvasElement);
    await screen.findByText("Assigned to Jane Doe");
  },
};

/** The server refused the write: the list stays open over the reason. */
export const Refused: Story = {
  render: () => <Picker write="refused" />,
  play: async ({ canvasElement }) => {
    const list = await pickJane(canvasElement);
    await list.findByRole("alert");
  },
};
export const RefusedDark: Story = { ...Refused, globals: { theme: "dark" } };
export const RefusedPhone: Story = {
  ...Refused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The colleague list could not be read: the reason stands in for the rows. */
export const RosterFailed: Story = {
  render: () => <Picker roster="refused" />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await list.findByText("You may not list the workspace’s members.");
  },
};

/** Nobody the record may be handed to. */
export const EmptyRoster: Story = {
  render: () => <Picker roster="empty" ownerId={null} />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await list.findByText("No match");
  },
};

/** The walk stopped short of the workspace, and the list says so. */
export const PartialRoster: Story = {
  render: () => <Picker roster="partial" />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await list.findByText(englishCopy["state.partial"]);
  },
};

/** Names long enough to wrap inside the panel. */
export const LongNames: Story = {
  render: () => <Picker roster="long" ownerId={null} />,
  play: async ({ canvasElement }) => {
    const list = await openList(canvasElement);
    await list.findByRole("option", { name: /^Maximiliane/ });
  },
};
export const LongNamesPhone: Story = {
  ...LongNames,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

function CallerOpenedPicker() {
  const [open, setOpen] = useState(false);
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /users": (_, url) => rosterAnswer("colleagues", url),
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <Button variant="link" onClick={() => setOpen(true)}>
          Edit
        </Button>
        <OwnerPick
          opener="caller"
          open={open}
          onClose={() => setOpen(false)}
          record={{ id: "p-1", version: 3, owner_id: "u-7" }}
          assign={() => assign("lands")}
          invalidate="projects"
          recordKey="project"
        />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

/** Opened by a section's Edit link: no trigger of its own, the sheet at every
 *  width, and focus back on the link once the pick has landed. */
export const OpenedByCaller: Story = {
  render: () => <CallerOpenedPicker />,
  play: async ({ canvasElement }) => {
    const edit = await within(canvasElement).findByRole("button", {
      name: "Edit",
    });
    await userEvent.click(edit);
    const list = within(
      await screen.findByRole("dialog", {
        name: englishCopy["ownerPick.change"],
      }),
    );
    await userEvent.click(
      await list.findByRole("option", { name: "Jane Doe" }),
    );
    await screen.findByText("Assigned to Jane Doe");
    await waitFor(() => expect(edit).toBe(document.activeElement));
  },
};
export const OpenedByCallerPhone: Story = {
  ...OpenedByCaller,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
