// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkAsLeadAction } from "./contactleadaction";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The contact page's "Work as a lead" row, as it sits in the More menu: one
// press creates the reader's lead from the contact and opens it.
const meta: Meta = {
  title: "Records/Contact 360/Work as a lead",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const MayCreateLeads: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "create"] }),
      "POST /leads": () => jsonResponse({ id: "l-new" }, 201),
    });
    return (
      <StoryProviders>
        <WorkAsLeadAction contactId="p-1" />
      </StoryProviders>
    );
  },
};

// A seat without lead create sees no row at all: a verb it cannot press is
// not offered in a menu of things to do.
export const MayNotCreateLeads: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ lead: ["read"] }) });
    return (
      <StoryProviders>
        <WorkAsLeadAction contactId="p-1" />
      </StoryProviders>
    );
  },
};
