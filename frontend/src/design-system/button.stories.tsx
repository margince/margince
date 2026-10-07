// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
import type { CSSProperties } from "react";
import { Button, TextInput } from "./atoms";
import { ProviderMark } from "./provider-mark";

const meta: Meta<typeof Button> = {
  title: "Components/Forms and input/Button",
  component: Button,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Button>;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

// Height, icon size, width floor and focus ring each look fine one variant at a
// time; only a row of every axis shows them disagreeing.
export const EveryAxis: Story = {
  render: () => (
    <div style={stack}>
      <div style={stack}>
        <span className="t-label">Variants</span>
        <div style={row}>
          <Button variant="primary">Save</Button>
          <Button variant="ghost">Cancel</Button>
          <Button variant="danger">Delete</Button>
          <Button variant="ai">Draft a reply</Button>
          <Button variant="aiQuiet">Shorter</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">The secondary text affordance</span>
        <div style={row}>
          <Button variant="primary">Save changes</Button>
          <a className="link-button" href="#link-button-story">
            Download the signed PDF
          </a>
          <button type="button" className="link-button">
            View existing
          </button>
          <button type="button" className="link-button">
            <Plus aria-hidden />
            Add another
          </button>
          <Button variant="link">Make private</Button>
          <Button variant="link" pending>
            Make private
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">With an icon</span>
        <div style={row}>
          <Button variant="primary">
            <Plus aria-hidden />
            Add contact
          </Button>
          <Button variant="ghost">
            <RefreshCw aria-hidden />
            Reconnect
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Icon only — square, and named for a reader
        </span>
        <div style={row}>
          <Button variant="primary" iconOnly aria-label="Add contact">
            <Plus aria-hidden />
          </Button>
          <Button variant="ghost" iconOnly aria-label="Reconnect">
            <RefreshCw aria-hidden />
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Short labels keep the width floor</span>
        <div style={row}>
          <Button variant="ghost">No</Button>
          <Button variant="primary">Yes</Button>
          <Button variant="ghost">Add</Button>
          <Button variant="primary">Save</Button>
          <Button variant="ghost">Disconnect this inbox</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Beside a field, which is the same box</span>
        <div style={row}>
          <TextInput
            aria-label="Address to invite"
            defaultValue="ops@example.com"
            style={{ inlineSize: "16rem" }}
          />
          <Button variant="primary">Invite</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Refused, disabled, and the icon affordance
        </span>
        <div style={row}>
          <Button variant="primary" reason="Connect an inbox first.">
            Send
          </Button>
          <Button variant="ghost" disabled>
            Cancel
          </Button>
          <button type="button" className="iconbtn" aria-label="Remove">
            <Trash2 aria-hidden />
          </button>
        </div>
      </div>
      {/* The label stays put while it works: "Saving…" would rename the
          control the reader is standing on. */}
      <div style={stack}>
        <span className="t-label">
          Working — which must not read as refused
        </span>
        <div style={row}>
          <Button variant="primary" pending>
            Save
          </Button>
          <Button variant="ghost" pending>
            Reconnect
          </Button>
          <Button variant="ghost" iconOnly pending aria-label="Reconnect">
            <RefreshCw aria-hidden />
          </Button>
          <Button
            variant="primary"
            pending
            busyLabel="Signing you in — this can take a moment."
          >
            Sign in
          </Button>
          <Button variant="primary" disabled>
            Save
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Refused wins over busy, both ways round</span>
        <div style={row}>
          <Button variant="primary" disabled pending>
            Save
          </Button>
          <Button variant="primary" pending reason="Connect an inbox first.">
            Send
          </Button>
        </div>
      </div>
      {/* No dim state greys a mark: another company's colours are not ours to
          recolour. */}
      <div style={stack}>
        <span className="t-label">
          Federated sign-in — offered, in flight, unavailable
        </span>
        <div style={{ ...stack, gap: "0.5rem", maxInlineSize: "20rem" }}>
          <Button variant="federated">
            <ProviderMark providerKey="google" />
            Continue with Google
          </Button>
          <Button variant="federated" disabled>
            <ProviderMark providerKey="google" />
            Continue with Google
          </Button>
          <Button variant="federated" unavailable>
            <ProviderMark providerKey="microsoft" />
            Continue with Microsoft
          </Button>
        </div>
      </div>
    </div>
  ),
};
