// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorklistItem } from "../lib/worklisttypes";
import { taskRow } from "../testing/brief.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskMeta } from "./taskmeta";

// The chip line Home's task cards and the Worklist's rows share.
const meta: Meta<typeof TaskMeta> = {
  title: "Patterns/Task chip line",
  component: TaskMeta,
};
export default meta;

type Story = StoryObj<typeof TaskMeta>;

function line(item: WorklistItem) {
  return () => {
    installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
    return (
      <StoryProviders>
        <TaskMeta item={item} link={(href) => href} />
      </StoryProviders>
    );
  };
}

const withContact: WorklistItem = {
  ...taskRow("send-quote", "Send the retrofit quote"),
  task_type: "reach_out",
  contact: {
    id: "c-alice",
    label: "Alice Weber",
    employer: { company_id: "co-weber", company_name: "Weber GmbH" },
  },
};

/** The task's type, then the contact and their company as chips. */
export const TypedWithContacts: Story = { render: line(withContact) };

/** A late task's due day says by how much, in the danger tone. */
export const Late: Story = {
  render: line({
    ...withContact,
    due_at: "2026-08-28T08:00:00Z",
    due_has_time: true,
    overdue: true,
  }),
};

/** A row with no type of its own names its kind instead. */
export const KindWithoutType: Story = {
  render: line({
    ...taskRow("deal-quiet", "Acme Expansion has gone quiet"),
    source: "deal_at_risk",
    category: "deals_at_risk",
    subject: { type: "deal", id: "d-acme", label: "Acme Expansion" },
  }),
};

export const TypedWithContactsDark: Story = {
  ...TypedWithContacts,
  globals: { theme: "dark" },
};
