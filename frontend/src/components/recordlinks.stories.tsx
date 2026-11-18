// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { expectTextFits } from "../testing/unbrokentext";
import { RecordLinkChips } from "./recordlinks";

const meta: Meta<typeof RecordLinkChips> = {
  title: "Patterns/Record link chips",
  component: RecordLinkChips,
};
export default meta;
type Story = StoryObj<typeof RecordLinkChips>;

const LONG_CONTACT = "Maximilian Konstantinopoulou-Wertheimer";

const NAMES: Record<string, string> = {
  "deal-1": "Weber renewal 2027",
  "co-1": "Weber GmbH",
  "c-1": LONG_CONTACT,
};

// The task panel's records line: a deal by its icon, a company and a contact
// by their faces. The contact's long name wraps inside the line.
export const TaskRecordsPhone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/references": (_body, url) =>
        jsonResponse({
          data: url.searchParams.getAll("id").map((id) => ({
            id,
            name: NAMES[id] ?? null,
            archived: false,
            logo: null,
          })),
        }),
    });
    return (
      <StoryProviders>
        <div
          data-records
          style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}
        >
          <RecordLinkChips
            links={[
              { entity_type: "deal", entity_id: "deal-1" },
              { entity_type: "company", entity_id: "co-1" },
              { entity_type: "contact", entity_id: "c-1" },
            ]}
          />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const line = canvasElement.querySelector("[data-records]");
    const contact = await canvas.findByRole("link", { name: LONG_CONTACT });
    if (!line) throw new Error("no records line");
    await expectTextFits(contact, line.getBoundingClientRect());
    await expect(
      await canvas.findByRole("link", { name: "Weber renewal 2027" }),
    ).toBeVisible();
  },
};
