// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MailX } from "lucide-react";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { Callout } from "./callout";
import { FactList } from "./factlist";

const meta: Meta = {
  title: "Design System/Callout",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

/** Several notices read together, at the interval a screen stacks them on. */
function Stack({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-3)",
      }}
    >
      {children}
    </div>
  );
}

/** One notice with the announcement it ends up with printed under it. */
function Derives({
  announces,
  children,
}: Readonly<{ announces: string; children: ReactNode }>) {
  return (
    <div>
      {children}
      <p className="t-caption" style={{ marginTop: "var(--space-1)" }}>
        {announces}
      </p>
    </div>
  );
}

/** The four tones together, which is the only way to judge that they differ.
 * No `icon` at any of them: the glyph comes from the tone, so shape carries the
 * claim as well as colour does. */
export const Tones: Story = {
  render: () => (
    <Stack>
      <Callout>Capture is reading this mailbox every five minutes.</Callout>
      <Callout tone="warn" title="Reindex needed">
        Search is answering from an index that is behind the records.
      </Callout>
      <Callout tone="danger" title="That did not save">
        The role changed while you were editing. Re-read it and try again.
      </Callout>
      <Callout tone="success">HubSpot is connected.</Callout>
    </Stack>
  ),
};

/**
 * The six reference states, in the order a reader meets them: a notice with
 * nothing to do about it, one with a verb, a failure that names what it
 * refused, a confirmation the reader can put away, a standing warning, and the
 * bare row that 108 of this primitive's call sites actually are.
 */
export const AlertAnatomy: Story = {
  render: () => (
    <Stack>
      <Callout title="Capture is running">
        Every message to and from this mailbox is read every five minutes.
      </Callout>

      <Callout
        title="The index is behind"
        actions={<Button small>Refresh</Button>}
      >
        Search is answering from the records as they stood an hour ago.
      </Callout>

      <Callout
        tone="danger"
        kind="outcome"
        title="That did not save"
        actions={
          <Button variant="primary" small>
            Retry
          </Button>
        }
      >
        <p>The record changed while you were editing it:</p>
        <ul>
          <li>Owner — you set Mara Voss, the record now says Piet de Wit</li>
          <li>Stage — you set Won, the record now says Negotiating</li>
        </ul>
      </Callout>

      <Callout
        tone="success"
        kind="outcome"
        dismiss={{ label: "Dismiss", onDismiss: () => {} }}
      >
        HubSpot is connected.
      </Callout>

      <Callout tone="warn" kind="standing" title="This licence is in grace">
        Seats stay writable until 31 March. After that the workspace keeps
        answering reads and refuses every write, including the ones the
        connectors make on their own.
      </Callout>

      <Callout tone="danger" live="alert">
        The invitation could not be sent.
      </Callout>
    </Stack>
  ),
};

/**
 * What `kind` buys: the caller says what the notice IS and the primitive
 * derives how loudly a screen reader hears it. The caption under each is the
 * role that lands on the element — the story's own annotation, not part of the
 * primitive.
 */
export const Kinds: Story = {
  render: () => (
    <Stack>
      <Derives announces='kind="outcome" + danger → role="alert"'>
        <Callout tone="danger" kind="outcome" title="That did not save">
          The role changed while you were editing.
        </Callout>
      </Derives>

      <Derives announces='kind="outcome" + any other tone → role="status"'>
        <Callout tone="success" kind="outcome">
          The invitation is on its way.
        </Callout>
      </Derives>

      <Derives announces='kind="event" → role="status"'>
        <Callout tone="warn" kind="event" title="The HubSpot connector stopped">
          It refused the last three reads. Nothing has been captured since
          09:12.
        </Callout>
      </Derives>

      <Derives announces='kind="standing" → no role at all'>
        <Callout kind="standing">
          A person is only visible here once a deal or a thread names them.
        </Callout>
      </Derives>
    </Stack>
  ),
};

/**
 * The titled-with-a-verb state at a phone's width, where the verb drops BELOW
 * the words instead of sitting in a column at their end.
 *
 * `uat-phone` is what drives the capture gate's browser to 390px: Storybook's
 * own viewport is applied by the manager, which the gate's bare `iframe.html`
 * never runs, so without the tag this would be captured at desktop width and
 * would picture the very layout it exists to rule out.
 */
export const Narrow: Story = {
  render: () => (
    // The card's own inset, so the callout folds where it would on a real
    // surface rather than against the viewport edge. `fullscreen` keeps the
    // catalog's 2rem frame off it: 390px less two frames is not a width any
    // reader has.
    <div style={{ padding: "var(--padCard)" }}>
      <Callout
        title="The index is behind"
        actions={<Button small>Refresh</Button>}
      >
        Search is answering from the records as they stood an hour ago.
      </Callout>
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** With an action, which is what most banners actually are. */
export const WithActions: Story = {
  render: () => (
    <Callout
      tone="warn"
      kind="event"
      title="Connection interrupted"
      actions={
        <Button variant="primary" small>
          Resume
        </Button>
      }
      dismiss={{ label: "Dismiss", onDismiss: () => {} }}
    >
      Finish connecting Claude to pick up where you left off.
    </Callout>
  ),
};

/** Prose over one line, so the paragraph rhythm inside the body is visible. */
export const Prose: Story = {
  render: () => (
    <Callout tone="danger" title="This cannot be undone">
      <p>Erasure removes the person and everything captured about them.</p>
      <p>A tombstone stays in the audit log. Nothing else survives.</p>
    </Callout>
  ),
};

/** Three paragraphs deep, where the icon has to stay on the heading's first
 * line rather than drifting to the middle of the box as the body grows. */
export const LongContent: Story = {
  render: () => (
    <Callout tone="danger" kind="standing" title="This workspace cannot import">
      <p>
        An import writes to every module that owns a record, and two of them are
        refusing writes while the licence sits in its grace period.
      </p>
      <p>
        The file itself is fine: 4,182 rows, 12 columns, every required column
        present, and the three columns nothing here maps to would have been
        dropped with a line in the report.
      </p>
      <p>
        Clear the licence and start the import again. Nothing from this attempt
        was written, so there is nothing to undo first.
      </p>
    </Callout>
  ),
};

/** A callout carrying facts rather than prose — the pairing this file is the
 * right home for, with an `icon` override, which is what that prop is for: a
 * notice about a nameable thing rather than about how bad the news is.
 * FactList's own states are catalogued under its own name in
 * `factlist.stories.tsx`; a primitive whose only story sits under a different
 * component's heading is a primitive nobody finds. */
export const WithFacts: Story = {
  render: () => (
    <Callout
      tone="warn"
      kind="standing"
      icon={MailX}
      title="Nobody has written back"
    >
      <FactList
        numeric
        facts={[
          { key: "in", term: "Last inbound", value: "3 Feb 2026" },
          { key: "out", term: "Last outbound", value: "Never" },
          {
            key: "spend",
            term: "Spend this month",
            value: "€1,204.00",
            note: "Partial — 12 of 28 days counted",
          },
        ]}
      />
    </Callout>
  ),
};
