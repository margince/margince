// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { CompanyMarks } from "./companymarks";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The pills under an account's name: archived, lifecycle, what it is to us,
// and who may read it — one line of one height, nothing revealed on hover but
// the pressed chip's own edge.
const meta: Meta = {
  title: "Records/Company 360/Marks",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];

const company: Company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  relationship_types: ["customer", "partner"],
  owner_id: "u-1",
  visibility: "workspace",
  writable: true,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  version: 1,
};

function Marks({ record = company }: Readonly<{ record?: Company }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <CompanyMarks company={record} />
    </StoryProviders>
  );
}

export const Live: Story = { render: () => <Marks /> };

// Archived leads the row: a state of the record, read with its name.
export const Archived: Story = {
  render: () => (
    <Marks record={{ ...company, archived_at: "2026-07-13T00:00:00Z" }} />
  ),
};

// A private account open to a colleague it was shared with: the chip says
// "Private", and the answer behind it names whose it is.
export const PrivateToAColleague: Story = {
  render: () => (
    <Marks record={{ ...company, visibility: "owner", writable: false }} />
  ),
  play: async () => {
    // The panel portals to the body, so it is reached through `screen`.
    await userEvent.click(
      await screen.findByRole("button", { name: /Who can see this company/ }),
    );
    await screen.findByText(/Private to Mira Voss/);
  },
};
