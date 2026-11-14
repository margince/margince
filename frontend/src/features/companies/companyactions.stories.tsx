// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { pipelinesOptions } from "../../lib/pipelines.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  storyQueryClient,
} from "../../testing/story-utils";
import { NewDealAction } from "./companyactions";

type Pipeline = components["schemas"]["Pipeline"];

const page = { has_more: false, next_cursor: null };

const meta: Meta = {
  title: "Records/Record 360/New deal action",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// A default pipeline with two OPEN stages — populated enough that the deal
// form's stage select shows a real choice, not a single unavoidable option.
const openPipeline: Pipeline = {
  id: "p-1",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s-1",
      pipeline_id: "p-1",
      name: "Qualifying",
      position: 0,
      semantic: "open",
      win_probability: 20,
    },
    {
      id: "s-2",
      pipeline_id: "p-1",
      name: "Proposal",
      position: 1,
      semantic: "open",
      win_probability: 60,
    },
  ],
};

// A pipeline whose every stage is already won or lost — nowhere for a new
// deal to land, which is why NewDealAction renders nothing rather than a
// button that can only fail (see the docblock on the component itself).
const closedPipeline: Pipeline = {
  id: "p-2",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s-3",
      pipeline_id: "p-2",
      name: "Won",
      position: 0,
      semantic: "won",
      win_probability: 100,
    },
    {
      id: "s-4",
      pipeline_id: "p-2",
      name: "Lost",
      position: 1,
      semantic: "lost",
      win_probability: 0,
    },
  ],
};

function NewDeal({
  pipeline,
  submit,
  cached = false,
}: Readonly<{
  pipeline: Pipeline;
  cached?: boolean;
  submit?: (body: unknown) => Response | Promise<Response>;
}>) {
  const client = storyQueryClient();
  if (cached) {
    client.setQueryDefaults(pipelinesOptions.queryKey, { staleTime: Infinity });
    client.setQueryData(pipelinesOptions.queryKey, [pipeline]);
  }
  installFetchStub({
    // Where the deal may land, and the write that puts it there.
    "GET /me": meRoute({ pipeline: ["read"], deal: ["create"] }),
    "GET /pipelines": () =>
      cached
        ? jsonResponse({ title: "The cached catalog should be reused" }, 500)
        : jsonResponse({ data: [pipeline], page }),
    ...(submit ? { "POST /deals": submit } : {}),
  });
  return (
    <StoryProviders client={client}>
      <NewDealAction companyId="o-1" companyName="Brandt Automotive GmbH" />
    </StoryProviders>
  );
}

// Resting: the button alone, before the rep has asked for anything.
export const NewDealResting: Story = {
  render: () => <NewDeal pipeline={openPipeline} />,
};

export const NewDealFromCachedCatalog: Story = {
  render: () => <NewDeal pipeline={openPipeline} cached />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", { name: "New deal" });
  },
};

// No open stage anywhere in the default pipeline: the component returns
// null, so this story's canvas is intentionally empty — the honest render
// of "there is nowhere for this button to send a deal".
export const NewDealNoTarget: Story = {
  render: () => <NewDeal pipeline={closedPipeline} />,
};

// Open: the stage select carries both open stages, currency defaults to the
// first option, and the deal name is still blank — Save stays disabled on
// the one field the form cannot default.
//
// The trigger lives in canvasElement, but CreateAction's modal is Modal
// (design-system/atoms), which portals to document.body rather than
// rendering in place (so a dialog opened from a collapsed menu survives the
// menu's own collapse): every query for something the modal shows has to
// scope past canvasElement to the document it sits in, or it never sees the
// form at all.
export const NewDealOpen: Story = {
  render: () => <NewDeal pipeline={openPipeline} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New deal" }),
    );
    await body.findByRole("textbox", { name: "Deal name" });
  },
};

// Pending: the name is filled and Save clicked, and the create endpoint never
// resolves — the form sits mid-write the way a slow request actually looks,
// rather than a state nobody could otherwise catch on screen.
//
// The control keeps its own name throughout: a write in flight is announced
// through aria-busy and refused through aria-disabled, so the reader keeps both
// their focus and the word they pressed. Asserting the resting name is what
// makes this story fail if the button ever renames itself again.
async function expectCreating(body: ReturnType<typeof within>) {
  const submit = await body.findByRole("button", { name: "Create" });
  await waitFor(() => {
    expect(submit).toHaveAttribute("aria-busy", "true");
    expect(submit).toHaveAttribute("aria-disabled", "true");
  });
}
export const NewDealPending: Story = {
  render: () => (
    <NewDeal pipeline={openPipeline} submit={() => new Promise(() => {})} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New deal" }),
    );
    await userEvent.type(
      await body.findByRole("textbox", { name: "Deal name" }),
      "Fleet renewal",
    );
    await userEvent.click(body.getByRole("button", { name: "Create" }));
    await expectCreating(body);
  },
};

// Field refusals appear beside the input that can correct them.
export const NewDealFailed: Story = {
  render: () => (
    <NewDeal
      pipeline={openPipeline}
      submit={() =>
        jsonResponse(
          {
            code: "validation_error",
            title: "Unprocessable Entity",
            detail:
              "Amount and currency travel together or not at all on a deal.",
            details: {
              errors: [
                {
                  field: "currency",
                  code: "amount_currency_pair",
                  message:
                    "currency comes with amount_minor or expected_arr_minor, and neither figure comes without it",
                },
              ],
            },
          },
          422,
        )
      }
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New deal" }),
    );
    await userEvent.type(
      await body.findByRole("textbox", { name: "Deal name" }),
      "Fleet renewal",
    );
    await userEvent.click(body.getByRole("button", { name: "Create" }));
    await body.findByText(
      "Currency comes with amount_minor or expected_arr_minor, and neither figure comes without it.",
    );
  },
};

// The id the workspace mints for a name its catalog has never carried. The
// apply call is addressed to a RESOLVED tag, so a story that wants to answer
// it has to route the id the create handed back, not the contract's template.
