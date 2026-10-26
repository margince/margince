// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Locale } from "../../i18n";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { ReviewTemplate } from "./outcomereview.queries";
import { ReviewTemplatesCard } from "./reviewtemplates";

// The questions a closed deal is asked. The list an administrator meets shows
// a win and a loss template side by side, each badge in its own tone. A retired
// template stays listed under the badge saying it is no longer offered.
// Won is green and Lost grey: a loss is a result, not a fault.

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

function served(
  answer: () => Response | Promise<Response>,
  grants: GrantSpec = READER,
  locale: Locale = "en",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      "GET /activity-review-templates": answer,
    });
    return (
      <StoryProviders locale={locale}>
        <ReviewTemplatesCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Sales/Outcome reviews/Review questions",
  component: ReviewTemplatesCard,
  parameters: { layout: "padded" },
} satisfies Meta<typeof ReviewTemplatesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

/** A win and a loss template, and a retired one kept on the list. */
export const Templates: Story = {
  render: served(() => jsonResponse({ data: TEMPLATES })),
};

/** An editor sees each template's own Edit questions verb in its panel head. */
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

export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: served(() => jsonResponse({ data: TEMPLATES }), EDITOR),
};

// The shipped win review names catalog keys, so a Vietnamese reader meets it
// in Vietnamese; the question an administrator added reads as typed.
const SHIPPED_WIN = {
  ...TEMPLATES[0],
  label_key: "reviewTemplates.builtin.win_review",
  questions: [
    {
      key: "why_we_won",
      label: "Why did we win?",
      label_key: "reviewTemplates.builtin.question.why_we_won",
      type: "text",
      required: true,
    },
    {
      key: "pricing",
      label: "Giá có phải lý do?",
      type: "text",
      required: false,
    },
  ],
};

export const Vietnamese: Story = {
  render: served(() => jsonResponse({ data: [SHIPPED_WIN] }), READER, "vi"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Đánh giá khi thắng")).toBeVisible();
    await expect(canvas.getByText("Vì sao thắng deal này?")).toBeVisible();
    await expect(canvas.getByText("Giá có phải lý do?")).toBeVisible();
  },
};

/** No templates served: the card says so rather than drawing an empty list. */
export const Empty: Story = {
  render: served(() => jsonResponse({ data: [] })),
};

/** The read in flight: the card holds the skeleton under its title. */
export const Loading: Story = {
  render: served(() => new Promise<Response>(() => {})),
};

/** The read refused: the error and its retry, and no list. */
export const Failed: Story = {
  render: served(() =>
    jsonResponse({ title: "Server error", status: 500 }, 500),
  ),
};
