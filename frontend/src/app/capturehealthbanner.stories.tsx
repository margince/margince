import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { CaptureHealthBanner } from "./capturehealthbanner";

type Kind = components["schemas"]["CaptureConcernKind"];
function story(concern?: Kind, extra?: Kind) {
  return () => {
    installFetchStub({
      "GET /connectors": () =>
        jsonResponse({
          data: concern
            ? [
                {
                  id: "0190b7a2-0000-7000-8000-000000000001",
                  provider: "gmail",
                  status: "connected",
                  scopes: [],
                  can_backfill: true,
                  account_label: "mailbox@example.test",
                  concern,
                  concern_since: "2026-10-01T09:00:00Z",
                },
                ...(extra
                  ? [
                      {
                        id: "connection-2",
                        provider: "graph",
                        status: "connected",
                        scopes: [],
                        can_backfill: true,
                        account_label: "other@example.test",
                        concern: extra,
                        concern_since: "2026-10-01T09:00:00Z",
                      },
                    ]
                  : []),
              ]
            : [],
        }),
    });
    return (
      <StoryProviders>
        <CaptureHealthBanner />
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Shell/Capture health banner",
  component: CaptureHealthBanner,
} satisfies Meta<typeof CaptureHealthBanner>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SyncStopped: Story = { render: story("sync_stalled") };
export const Silence: Story = { render: story("capture_silent") };
export const Reconnect: Story = { render: story("reauth_required") };
export const Healthy: Story = { render: story() };

export const SilenceDark: Story = { ...Silence, globals: { theme: "dark" } };
export const SyncStoppedDark: Story = {
  ...SyncStopped,
  globals: { theme: "dark" },
};

export const MultipleConcerns: Story = {
  render: story("capture_silent", "sync_stalled"),
};
export const MultipleConcernsDark: Story = {
  ...MultipleConcerns,
  globals: { theme: "dark" },
};
