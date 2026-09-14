// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  AddProjectStakeholder,
  RemoveProjectStakeholder,
} from "./projectstakeholders";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The verbs on the project page's stakeholders card, drawn on their own so both
// dialogs can be read in either theme without the surrounding 360 read.
const meta: Meta = {
  title: "Records/ProjectStakeholders",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function stubContacts() {
  installFetchStub({
    "GET /contacts": () =>
      jsonResponse({
        data: [
          { id: "p-1", full_name: "Mai Trần" },
          { id: "p-2", full_name: "Jonas Weber" },
        ],
        page: { next_cursor: null, has_more: false },
      }),
  });
}

export const Add: Story = {
  render: () => {
    stubContacts();
    return (
      <StoryProviders>
        <AddProjectStakeholder projectId="pr-1" />
      </StoryProviders>
    );
  },
};

export const Remove: Story = {
  render: () => {
    stubContacts();
    return (
      <StoryProviders>
        <RemoveProjectStakeholder
          projectId="pr-1"
          contactId="p-1"
          contactName="Mai Trần"
        />
      </StoryProviders>
    );
  },
};

// A seat whose contact the reader may not read. The seat still counts, and is
// still removable, so the dialog names it as withheld rather than borrowing a
// name it was never given.
export const RemoveWithheldContact: Story = {
  render: () => {
    stubContacts();
    return (
      <StoryProviders>
        <RemoveProjectStakeholder
          projectId="pr-1"
          contactId="p-9"
          contactName={null}
        />
      </StoryProviders>
    );
  },
};
