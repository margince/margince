// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import type { ReviewTemplate } from "./outcomereview.queries";
import { ReviewTemplatesCard } from "./reviewtemplates";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The questions a closed deal is asked. The picture worth keeping is the list
// an administrator actually meets: a win and a loss template side by side, each
// outcome badge in its own tone, and a retired template still listed under the
// badge that says it is no longer offered.

function template(over: Partial<ReviewTemplate>): ReviewTemplate {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    key: "win_review",
    label: "Win review",
    outcome: "won",
    questions: [
      { key: "why", label: "Why did we win?", type: "text", required: true },
      {
        key: "else",
        label: "Who else were they considering?",
        type: "text",
        required: false,
      },
    ],
    version: 1,
    active: true,
    system: true,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  };
}

const TEMPLATES: ReviewTemplate[] = [
  template({}),
  template({
    id: "22222222-2222-4222-8222-222222222222",
    key: "loss_review",
    label: "Loss review",
    outcome: "lost",
    questions: [
      {
        key: "why_lost",
        label: "What decided it against us?",
        type: "text",
        required: true,
      },
    ],
  }),
  template({
    id: "33333333-3333-4333-8333-333333333333",
    key: "loss_review_2025",
    label: "Loss review (2025)",
    outcome: "lost",
    active: false,
  }),
];

const READER: GrantSpec = { custom_field: ["read"] };
const EDITOR: GrantSpec = { custom_field: ["read", "update"] };

function served(answer: () => Response, grants: GrantSpec = READER) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      "GET /activity-review-templates": answer,
    });
    return (
      <StoryProviders>
        <ReviewTemplatesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ReviewTemplatesCard> = {
  title: "Settings/Sales/Outcome reviews/Review questions",
  component: ReviewTemplatesCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ReviewTemplatesCard>;

/** A win and a loss template, and a retired one kept on the list. */
export const Templates: Story = {
  render: served(() => jsonResponse({ data: TEMPLATES })),
};

/** An editor sees each template's own Edit questions verb in its head. */
export const Editable: Story = {
  render: served(() => jsonResponse({ data: TEMPLATES }), EDITOR),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findAllByRole("button", {
      name: "Edit questions",
    });
  },
};

/** The editor's view at phone width: head verbs wrap under the name. */
export const EditablePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: served(() => jsonResponse({ data: TEMPLATES }), EDITOR),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findAllByRole("list");
  },
};

/** No templates served: the card says so rather than drawing an empty list. */
export const Empty: Story = {
  render: served(() => jsonResponse({ data: [] })),
};
