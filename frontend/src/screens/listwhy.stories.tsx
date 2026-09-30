// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  chosenWhy,
  LIVE_ID,
  liveList,
  liveWhy,
  MEMBER_ID,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import { ListWhy } from "./listwhy";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Why one record is on a list. A Live List answers clause by clause, with a
// value the reader may not see named as hidden; a Shortlist answers with who
// chose the record and why.
const meta: Meta = { title: "Patterns/List why" };
export default meta;

type Story = StoryObj;

const record = { id: MEMBER_ID, name: "MiTek" };

export const LiveListClauses: Story = {
  render: () => {
    installFetchStub({
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(liveWhy),
      "GET /filters/vocabulary": () =>
        jsonResponse({ resource: "company", fields: [] }),
    });
    return (
      <StoryProviders>
        <ListWhy list={liveList} record={record} onClose={() => undefined} />
      </StoryProviders>
    );
  },
};

export const ChosenForAShortlist: Story = {
  render: () => {
    installFetchStub({
      [`GET /lists/${SHORTLIST_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(chosenWhy),
    });
    return (
      <StoryProviders>
        <ListWhy list={shortlist} record={record} onClose={() => undefined} />
      </StoryProviders>
    );
  },
};
