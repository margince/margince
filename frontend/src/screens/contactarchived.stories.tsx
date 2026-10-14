// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../testing/story-utils";
import { archivedContactRoutes, refusedRead } from "./contactarchived.fixtures";
import { ContactPageV2 } from "./contactpage";

// The contact page on an archived row. The server refuses its 360, so the page
// draws the row, its refused verbs and the history that can undo the archive.
const meta: Meta = {
  title: "Records/Contact 360/Archived",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function ArchivedPage({ rowFails = false }: Readonly<{ rowFails?: boolean }>) {
  const routes = archivedContactRoutes();
  if (rowFails) {
    routes["GET /contacts/p-1"] = refusedRead;
  }
  installFetchStub(routes);
  return (
    <StoryProviders>
      <ContactPageV2 id="p-1" tab="overview" />
    </StoryProviders>
  );
}

export const Archived: Story = { render: () => <ArchivedPage /> };

export const ArchivedOnPhone: Story = {
  name: "Archived · on a phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <ArchivedPage />,
};

// The row itself refused: not visible to this reader, or gone.
export const RowNotOpened: Story = {
  name: "Row not opened",
  render: () => <ArchivedPage rowFails />,
};
