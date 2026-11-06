// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { MemberRef, Members, Rule } from "./reporting.initiativegoal";
import { MemberFields } from "./reporting.initiativemembers";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

// The initiative drawer's members step: a rule, and the records picked by
// name, each found by asking the server.

const meta: Meta = {
  title: "Records/Reports/Analytics/Initiative members",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const members: Members = {
  data: [
    {
      record_type: "deal",
      record_id: "d-fleet",
      label: "Fleet renewal",
      company_label: "Nordhafen Logistik",
      reason: "picked",
      included: true,
    },
    {
      record_type: "lead",
      record_id: "l-ines",
      label: "Ines Duarte",
      reason: "picked",
      included: true,
    },
  ],
  leads: 1,
  deals: 1,
  companies: 1,
  truncated: false,
};

function Step() {
  const [rule, setRule] = useState<Rule>({
    product: "",
    industry: "",
    country: "DE",
  });
  const [picked, setPicked] = useState<MemberRef[]>([
    { record_type: "deal", record_id: "d-fleet" },
    { record_type: "lead", record_id: "l-ines" },
  ]);
  return (
    <MemberFields
      rule={rule}
      onRule={setRule}
      members={members}
      pending={false}
      picked={picked}
      onPick={setPicked}
      pickNames={[]}
      onPickName={() => undefined}
      excludedSet={new Set()}
      onToggle={() => undefined}
      includeFuture
      onIncludeFuture={() => undefined}
    />
  );
}

/** A saved initiative: its picks keep their names, whatever page they are on. */
export const SavedPicks: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /deals": () =>
        jsonResponse({
          data: [{ id: "d-harbour", name: "Harbour retrofit" }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <Step />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const deals = within(canvasElement).getByRole("group", {
      name: "Pick deals",
    });
    await userEvent.click(within(deals).getByRole("button", { name: /Add/ }));
    const body = within(document.body);
    await userEvent.type(body.getByRole("searchbox"), "harbour");
    await userEvent.click(
      await body.findByRole("checkbox", { name: /Harbour retrofit/ }),
    );
    await expect(within(deals).getByText("Harbour retrofit")).toBeVisible();
  },
};
