// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { company, company360 } from "./company.fixtures";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";

// The head of an account's Overview, read off its 360. An archived account
// has no 360, so its page draws none of this; the page story shows that.
const meta: Meta = {
  title: "Records/Company 360/Reading",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const props: CompanyOverviewProps = {
  company,
  view: company360,
  offerResearch: false,
  loading: false,
  failed: false,
  onOpenHistory: () => undefined,
  onOpenRecord: () => undefined,
  onOpenReceipt: () => undefined,
  onOpenEmail: () => undefined,
  onOpenTasks: () => undefined,
  onOpenTask: () => undefined,
  onLogActivity: () => undefined,
  onPrepareMeeting: () => undefined,
  onDraftTo: () => undefined,
  onOpenTab: () => undefined,
  onPerform: () => undefined,
};

function Reading(overrides: Partial<CompanyOverviewProps>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read", "update"] }) });
  return (
    <StoryProviders>
      <div className="co-overview-stack">
        <CompanyReading {...props} {...overrides} />
      </div>
    </StoryProviders>
  );
}

export const Assembled: Story = { render: () => <Reading /> };

export const Assembling: Story = {
  render: () => <Reading view={undefined} loading />,
};
