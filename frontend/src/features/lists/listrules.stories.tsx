// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { OverflowMenu } from "../../design-system/atoms";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { useT } from "../../i18n";
import { en } from "../../i18n/en";
import type { List } from "../../lib/lists.queries";
import { listsMe, liveList, MEMBER_ID } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ArchiveListAction } from "./listrules";
import { ListSettingsAction } from "./listsettings";

// Archive list, last in the menu beside a list's name and drawn in danger ink.
// A list automation rules depend on first names the rules the archive pauses.
const meta: Meta = {
  title: "Records/List archive",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** The menu as the list's head holds it: Edit list first, Archive list last. */
function ListMenu({ list }: Readonly<{ list: List }>) {
  const t = useT();
  return (
    <OverflowMenu label={t("filters.library.rowMore", { name: list.name })}>
      <ListSettingsAction list={list} />
      <ArchiveListAction list={list} />
    </OverflowMenu>
  );
}

function menu(list: List) {
  return () => {
    installFetchStub({ "GET /me": listsMe(true) });
    return (
      <StoryProviders>
        <ListMenu list={list} />
      </StoryProviders>
    );
  };
}

/** The menu's items are portalled to the body, outside the story's root. */
async function openMenu(canvasElement: HTMLElement, list: List) {
  const user = userEvent.setup();
  await user.click(
    await within(canvasElement).findByRole("button", {
      name: en["filters.library.rowMore"].replace("{name}", list.name),
    }),
  );
  return { user, body: within(canvasElement.ownerDocument.body) };
}

const watched: List = {
  ...liveList,
  dependencies: [
    {
      kind: "automation",
      occurred_at: "2026-09-20T08:00:00Z",
      blocking: false,
      role: "watches",
      automation_id: MEMBER_ID,
      automation_name: "Follow up on quiet manufacturers",
    },
    // A rule this reader may not open is still counted.
    {
      kind: "automation",
      occurred_at: "2026-09-21T08:00:00Z",
      blocking: false,
      role: "writes",
    },
  ],
};

export const ArchiveList: Story = {
  render: menu(liveList),
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, liveList);
  },
};

export const ArchiveNamesItsAutomations: Story = {
  render: menu(watched),
  play: async ({ canvasElement }) => {
    const { user, body } = await openMenu(canvasElement, watched);
    await user.click(
      await body.findByRole("button", { name: en["lists.archive"] }),
    );
    await body.findByRole("dialog");
  },
};

// No rule uses the list, so it is archived at once and the message offers Undo.
export const ArchivedWithUndo: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`DELETE /lists/${liveList.id}`]: () =>
        jsonResponse({ ...liveList, archived_at: "2026-09-30T08:00:00Z" }),
    });
    return (
      <StoryProviders>
        <ToastProvider>
          <ListMenu list={liveList} />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const { user, body } = await openMenu(canvasElement, liveList);
    await user.click(
      await body.findByRole("button", { name: en["lists.archive"] }),
    );
    await body.findByRole("button", { name: en["common.undo"] });
  },
};

// Danger ink on the dark menu, with its hover tint.
export const ArchiveListDark: Story = {
  ...ArchiveList,
  globals: { theme: "dark" },
};
