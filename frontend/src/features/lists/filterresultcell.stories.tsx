// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useCompanyMarksByID } from "../../components/companymarks";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ReferenceCell } from "./filterresultcell";

// A cell that names a record shows only the name. One whose record is gone
// says so, and keeps the id as its tooltip.
const meta: Meta = {
  title: "Patterns/Filters and views/Reference cell",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

function stubWorkspace() {
  installFetchStub({
    "GET /me": meRoute({ company: ["read"], deal: ["read"] }),
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-1", display_name: "Priya Shah" }] }),
    "GET /records/references": (_body, url) =>
      jsonResponse({
        data: url.searchParams
          .getAll("id")
          .filter((id) => id === "c-1")
          .map((id) => ({ id, name: "Brandt GmbH", archived: false })),
      }),
  });
}

function Cells({ ids }: Readonly<{ ids: { user: string; company: string } }>) {
  const companies = useCompanyMarksByID([ids.company]);
  return (
    <dl>
      <dt>Owner</dt>
      <dd>
        <ReferenceCell
          reference="app_user"
          id={ids.user}
          companies={companies}
        />
      </dd>
      <dt>Company</dt>
      <dd>
        <ReferenceCell
          reference="company"
          id={ids.company}
          companies={companies}
        />
      </dd>
    </dl>
  );
}

export const Named: Story = {
  render: () => {
    stubWorkspace();
    return <Cells ids={{ user: "u-1", company: "c-1" }} />;
  },
};

export const Gone: Story = {
  render: () => {
    stubWorkspace();
    return <Cells ids={{ user: "u-removed", company: "c-removed" }} />;
  },
};
