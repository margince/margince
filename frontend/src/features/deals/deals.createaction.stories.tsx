// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import type { ObjectCustomFields } from "../../components/customfields.form";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { Pipeline, Stage } from "./deals";
import { DealCreateAction } from "./deals.createaction";

const meta: Meta = {
  title: "Records/Deals/Create action",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const stages: Stage[] = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "s2",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 60,
  },
];

const pipeline: Pipeline = {
  id: "pl",
  name: "Sales",
  is_default: true,
  position: 0,
  stages,
};

const noCustomFields: ObjectCustomFields = {
  fields: [],
  loading: false,
  failed: false,
  retry: () => {},
  formFields: [],
  recordSlice: () => ({}),
  toBody: () => ({}),
  toPatch: () => ({}),
};

/** The create form open on the pipeline's open stages, as `C` opens it. */
export const FormOpen: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read", "create"] }) });
    return (
      <StoryProviders>
        <DealCreateAction
          pipeline={pipeline}
          cf={noCustomFields}
          openStages={stages}
          partnerOptions={[]}
          startOpen
        />
      </StoryProviders>
    );
  },
  play: async () => {
    await expect(
      await screen.findByRole("dialog", { name: "New deal" }),
    ).toBeInTheDocument();
  },
};
