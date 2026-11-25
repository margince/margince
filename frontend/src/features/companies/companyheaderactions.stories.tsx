// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { useActivityGate, writableTarget } from "../../components/activitygate";
import { en } from "../../i18n/en";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyHeaderActions } from "./companyheaderactions";
// The bar class the stories wrap the verbs in is styled by the record view.
import "../../design-system/recordview.css";

// The account header's verbs on their own: write, then the pair that records
// what already happened. What the stories hold is where a refusal lands — a
// caption under the row, never a sentence that widens it.
const meta: Meta = {
  title: "Records/Company 360/Header actions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];

const company: Company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  writable: true,
  version: 1,
};

const WRITES: GrantSpec = {
  company: ["read", "update"],
  activity: ["create"],
};

function Row({
  record = company,
  grants = WRITES,
  seat = "full",
}: Readonly<{ record?: Company; grants?: GrantSpec; seat?: "full" | "read" }>) {
  installFetchStub({ "GET /me": meRoute(grants, { seat }) });
  return (
    <StoryProviders>
      {/* The head's own bar class, so the verbs and any caption sit at the
          measure the record page gives them. */}
      <div style={{ maxWidth: "18.75rem" }}>
        <div className="record-split-verbs">
          <GatedActions record={record} />
        </div>
      </div>
    </StoryProviders>
  );
}

// The gate is the page's, read inside the providers the way the page reads it.
function GatedActions({ record }: Readonly<{ record: Company }>) {
  const activity = useActivityGate("company", writableTarget(record));
  return (
    <CompanyHeaderActions
      company={record}
      composerOpen={false}
      onComposerOpen={() => undefined}
      drawer={null}
      onDrawer={() => undefined}
      activity={activity}
    />
  );
}

// Every verb in the strip, Book included, points at the one sentence.
async function refusedWith(canvasElement: HTMLElement, sentence: string) {
  const canvas = within(canvasElement);
  for (const name of ["Email", "Log activity", /Book/]) {
    const verb = await canvas.findByRole("button", { name });
    await waitFor(() => expect(verb).toHaveAccessibleDescription(sentence));
  }
}

export const Live: Story = { render: () => <Row /> };

// A role that may change neither the account nor an activity: the role only reads.
export const LogRefused: Story = {
  render: () => <Row grants={{ company: ["read"] }} />,
  play: async ({ canvasElement }) => {
    const why = en["record.roleReadsOnly"];
    await refusedWith(canvasElement, why);
    const email = within(canvasElement).getByRole("button", { name: "Email" });
    await expect(email).toBeDisabled();
    const host = email.closest(".icon-action");
    await expect(host).not.toBeNull();
    if (host) {
      await userEvent.hover(host);
    }
    await waitFor(() =>
      expect(screen.getByRole("tooltip")).toHaveTextContent(`Email. ${why}`),
    );
  },
};

// A read seat is told about its seat, whatever its role holds.
export const SeatRefused: Story = {
  render: () => <Row seat="read" />,
  play: ({ canvasElement }) =>
    refusedWith(canvasElement, en["settings.seatRefused"]),
};

// A custom role that may change the account but not write an activity.
export const ActionRefused: Story = {
  render: () => <Row grants={{ company: ["read", "update"] }} />,
  play: ({ canvasElement }) =>
    refusedWith(canvasElement, en["record.actionRefused"]),
};

// A writing role on a colleague's account: Book says what Log says.
export const NotYours: Story = {
  render: () => <Row record={{ ...company, writable: false }} />,
  play: ({ canvasElement }) =>
    refusedWith(canvasElement, en["record.attachRefused"]),
};

export const NotYoursPhone: Story = { ...NotYours, tags: ["uat-phone"] };

// An archived account: every verb here refuses on the record's state, and the
// sentence is the component's own when the page has not stated it.
export const Archived: Story = {
  render: () => (
    <Row record={{ ...company, archived_at: "2026-07-13T00:00:00Z" }} />
  ),
};
