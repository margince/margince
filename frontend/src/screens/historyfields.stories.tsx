// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { FieldHistoryTimeline } from "./historyfields";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The per-field change view on its own, rather than behind RecordHistoryTab's
// toggle: history.stories.tsx reaches it only after a click, and this is the
// surface historyfields.tsx exports. Each row's attribution is a
// ProvenanceTag badge, so these stories are where the human arm, the agent
// arm and the agent's passport and evidence chips are seen side by side.

function seedWorkspace() {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
}

const renamedByHuman = {
  id: "f0",
  entity_type: "deal",
  entity_id: "d1",
  field: "name",
  old_value: "Globex",
  new_value: "Globex Renewal",
  changed_at: "2026-07-13T10:00:00Z",
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Demo Admin",
};
const repricedByAgent = {
  id: "f1",
  entity_type: "deal",
  entity_id: "d1",
  field: "amount_minor",
  old_value: "2500000",
  new_value: "4150000",
  changed_at: "2026-07-14T10:00:00Z",
  actor_type: "agent",
  actor_id: "sdr",
  passport_id: "psp_7Q3fa91",
  evidence: { snippet: "renewal signed at 41.5k", source: "email#42" },
};
const rescheduledByHuman = {
  id: "f2",
  entity_type: "deal",
  entity_id: "d1",
  field: "expected_close_date",
  old_value: "2026-08-31",
  new_value: "2026-09-30",
  changed_at: "2026-07-15T09:30:00Z",
  actor_type: "human",
  actor_id: "u2",
  actor_name: "Lena Fischer",
};

function Panel() {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <FieldHistoryTimeline kind="deal" id="d1" currency="EUR" />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Panel> = {
  title: "Records/Record history/Field changes",
  component: Panel,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Panel>;

// Three fields, two colleagues and an agent: each group heads its own field,
// the repricing reads at the deal's currency scale, and only the agent's row
// carries the AI tone with its passport and evidence beside it.
export const Changes: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /field-history": () =>
        jsonResponse({
          data: [rescheduledByHuman, repricedByAgent, renamedByHuman],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return <Panel />;
  },
};

// The same rows in dark, where the agent tone's tint and hairline have to
// stay distinct from the human badge against the dark card.
export const ChangesDark: Story = {
  ...Changes,
  globals: { theme: "dark" },
};

// No field was ever edited: the honest empty state rather than a card with
// filters over nothing.
export const Empty: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return <Panel />;
  },
};
