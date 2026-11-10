// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  ImapConnectPanel,
  OAuthReturnPanel,
} from "./onboarding-connect-panels";
import { ConnectScene } from "./onboarding-conversation/connect-scene";
import type { MailProvider } from "./onboarding-conversation/connect-vendor";

// The onboarding connect panels for the fe-uat render gate (G-10). They show
// the idle IMAP form, its post-connect state (the "building itself" framing,
// no fabricated capture counts) and its rejected-login error. They also show
// the post-consent return view, for a mailbox and for a chained calendar.

const meta: Meta<typeof ImapConnectPanel> = {
  title: "Onboarding/Connect panels",
  component: ImapConnectPanel,
};
export default meta;
type Story = StoryObj<typeof ImapConnectPanel>;

function ConnectedScene({
  imap = false,
  returnPanel = null,
}: Readonly<{ imap?: boolean; returnPanel?: ReactNode }>) {
  const [vendor, setVendor] = useState<MailProvider | null>(
    imap ? "imap" : null,
  );
  return (
    <ConnectScene
      vendor={vendor}
      onVendorChange={setVendor}
      askBusy={false}
      onAskBusyChange={() => {}}
      onImapDone={() => setVendor(null)}
      returnPanel={returnPanel}
      onFinish={() => {}}
      finishing={false}
      wantsOvernight
      onWantsOvernightChange={() => {}}
      overnightFailed={false}
      linkedinStatus="pending"
      onLinkedinSave={() => {}}
      onLinkedinSkip={() => {}}
      linkedinPending={false}
      linkedinError={null}
    />
  );
}

async function fillAndSubmit(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.clear(await canvas.findByLabelText("IMAP host"));
  await userEvent.type(canvas.getByLabelText("IMAP host"), "mail.example.org");
  await userEvent.type(canvas.getByLabelText("Email"), "lars@example.org");
  await userEvent.type(canvas.getByLabelText("App password"), "app-password");
  await userEvent.click(
    canvas.getByRole("button", { name: /test and connect/i }),
  );
}

export const ImapIdle: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <ImapConnectPanel onDone={() => {}} onDismiss={() => {}} />
      </StoryProviders>
    );
  },
};

export const ImapConnected: Story = {
  render: () => {
    let connected = false;
    const connection = {
      id: "c1",
      provider: "imap",
      status: "connected",
      scopes: [],
      account_label: "lars@example.org",
    };
    installFetchStub({
      "GET /me": meRoute({ capture_settings: ["read"] }),
      "GET /connectors": () =>
        jsonResponse({ data: connected ? [connection] : [], providers: [] }),
      "POST /connectors/imap/connect": () => {
        connected = true;
        return jsonResponse({ connection });
      },
    });
    return (
      <StoryProviders>
        <ConnectedScene imap />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await fillAndSubmit(canvasElement);
    await within(canvasElement).findByText(
      en["ob.conv.connect.connectedCaptionMailOnly"],
    );
  },
};

export const ImapLoginRejected: Story = {
  render: () => {
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse(
          {
            code: "imap_login_rejected",
            detail: "The mailbox rejected these credentials.",
          },
          422,
        ),
    });
    return (
      <StoryProviders>
        <ImapConnectPanel onDone={() => {}} onDismiss={() => {}} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await fillAndSubmit(canvasElement);
    await within(canvasElement).findByText(/rejected these credentials/i);
  },
};

export const OAuthReturnLive: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ capture_settings: ["read"] }),
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              id: "g1",
              provider: "graph",
              status: "connected",
              scopes: ["read"],
              backfill: { state: "done" },
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <ConnectedScene
          returnPanel={<OAuthReturnPanel outcome="ok" onDone={() => {}} />}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(
      en["ob.conv.connect.connectedCaptionMailOnly"],
    );
  },
};

/** Mail and calendar asked together, the calendar refused: the mailbox is
 *  still live, and the calendar says it was not connected. */
export const CalendarDeclined: Story = {
  render: () => {
    installFetchStub({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              id: "g1",
              provider: "gmail",
              status: "connected",
              scopes: ["read"],
              backfill: { state: "done" },
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <OAuthReturnPanel outcome="denied" provider="gcal" onDone={() => {}} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/calendar was not connected/i);
  },
};

export const CalendarDeclinedDark: Story = {
  ...CalendarDeclined,
  globals: { theme: "dark" },
};
