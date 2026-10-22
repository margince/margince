// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { SettingList } from "../../design-system/settingrow";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { HistoryBaselineRow } from "./connectors.historybaseline";
import "./connectors.css";

type CaptureConnection = components["schemas"]["CaptureConnection"];

const gmail: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000d1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
  account_label: "rep@example.de",
  mail_posture: "classified",
};

const withBaseline: CaptureConnection = {
  ...gmail,
  history_baseline: {
    date: "2026-03-01",
    timezone: "Europe/Berlin",
    set_at: "2026-03-02T09:00:00Z",
  },
};

function row(conn: CaptureConnection) {
  stubWithSession(
    {
      "PUT /connectors/gmail/history-baseline": () => jsonResponse(conn),
    },
    {},
  );
  return (
    <StoryProviders>
      <SettingList>
        <HistoryBaselineRow conn={conn} />
      </SettingList>
    </StoryProviders>
  );
}

const meta: Meta<typeof HistoryBaselineRow> = {
  title: "Settings/You/Connections/History baseline",
  component: HistoryBaselineRow,
};
export default meta;
type Story = StoryObj<typeof HistoryBaselineRow>;

export const NoBaseline: Story = { render: () => row(gmail) };

export const BaselineSet: Story = { render: () => row(withBaseline) };

export const BaselineSetDark: Story = {
  globals: { theme: "dark" },
  render: () => row(withBaseline),
};

export const BaselineSetPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => row(withBaseline),
};
