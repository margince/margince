// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { Fact, RecordFacts, RecordOriginFacts } from "./recordfacts";

const meta: Meta<typeof RecordFacts> = {
  title: "Components/Text and data display/Record facts",
  component: RecordFacts,
};
export default meta;
type Story = StoryObj<typeof RecordFacts>;

export const Strip: Story = {
  render: () => (
    <RecordFacts>
      <Fact label="Email">mareike.vollmer@nordwind-logistik.de</Fact>
      <Fact label="Phone">+49 40 3311 8842</Fact>
      <Fact label="Owner">Tim Rasche</Fact>
      <Fact label="Created">1 June 2026</Fact>
    </RecordFacts>
  ),
};

// Where a row came from, as the fields column shows it: an imported record
// names who wrote it in the system it came from.
export const Origin: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
    return (
      <StoryProviders>
        <RecordOriginFacts
          record={{
            created_at: "2026-06-01T08:00:00Z",
            captured_by: "human:u-admin",
            author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
            source: "hubspot",
          }}
        />
      </StoryProviders>
    );
  },
};
