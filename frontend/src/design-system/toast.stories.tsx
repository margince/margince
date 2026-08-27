// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import {
  type ToastOptions,
  ToastProvider,
  ToastRegion,
  useToast,
} from "./toast";

/**
 * The transient confirmation, fixed to the foot of the viewport.
 *
 * ## When a write gets one
 *
 * The bar is whether the reader can otherwise tell it worked. A toast for
 * something already visible on screen is noise, and noise is what teaches people
 * to stop reading the region that will one day carry something they need.
 *
 * **Show one** when the write succeeded and its result is NOT visible: a setting
 * saved, a background job queued, a bulk action over rows the reader cannot all
 * see, a value written from a control that already showed the new value before
 * the server agreed to it.
 *
 * **Show nothing** when the surface already answers: a row leaves the list, a
 * field updates in place, a modal closes onto the record it just created, the
 * reader is navigated somewhere that proves it.
 *
 * **Errors go inline where they have a home** — a field, a form, the card that
 * failed. A toast is for a refusal with nowhere else to land, and it takes
 * `mark: false`, because a green completion dot beside a failure says the
 * opposite of what the sentence says.
 *
 * **An action is offered only where an inverse write actually exists.** Most of
 * this product's destructive verbs have none: every `DELETE` is a soft archive
 * with no restore endpoint, and the record-history put-back refuses an archive
 * outright (`not_a_replayable_verb`). An Undo with nothing behind it is worse
 * than no Undo, because the reader stops looking for the real way back.
 */
const meta: Meta<typeof ToastRegion> = {
  title: "Design System/Toast",
  component: ToastRegion,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <ToastProvider>
          <Story />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ToastRegion>;

/**
 * Live, because the region's whole subject is timing: a confirmation withdraws
 * itself, one carrying a verb does not, and hovering either stops the clock.
 * None of that is visible in a static frame.
 */
function Bench({
  label,
  message,
  options,
  extra,
}: Readonly<{
  label: string;
  message: string;
  options?: ToastOptions;
  extra?: ReactNode;
}>) {
  const toast = useToast();
  return (
    <>
      <Button onClick={() => toast.show(message, options)}>{label}</Button>
      {extra}
    </>
  );
}

/**
 * A completion. The green dot belongs to the MESSAGE rather than to the region:
 * the same region carries a save that worked and a save that was refused, and a
 * tick beside a refusal says the opposite of what the sentence says.
 *
 * It withdraws itself after 3.5 seconds — unless a pointer is over it or focus
 * is inside it, which stops the clock and hands the full life back when the
 * reader moves away.
 */
export const Completion: Story = {
  render: () => <Bench label="Save" message="Signature saved." />,
};

/** A refusal takes no mark, and stays until it is put down. */
export const Refusal: Story = {
  render: () => (
    <Bench
      label="Save without a stage"
      message="A deal needs a stage before it can be saved."
      options={{ mark: false, sticky: true }}
    />
  ),
};

/**
 * The verb, which is what this exists for. A toast carrying one never withdraws
 * on a timer: a reader reaching for Undo must not lose it mid-reach, and there
 * is no timeout long enough to be safe that is also short enough to be a toast.
 *
 * Pressing it runs the inverse write and puts the message down — a message still
 * offering an action it has already taken is a second press waiting to happen.
 */
export const CarryingAnUndo: Story = {
  render: () => (
    <Bench
      label="Remove access"
      message="Access removed for Jana Brandt."
      options={{
        action: { label: "Undo", onAct: () => {} },
      }}
    />
  ),
};

/**
 * A message the reader has not answered is never taken away by one that is only
 * reporting. Press both: the archive keeps the region until it is dismissed, and
 * the save waits its turn behind it.
 */
export const AVerbOutranksAReport: Story = {
  render: () => (
    <Bench
      label="Archive"
      message="Contract archived."
      options={{ action: { label: "Undo", onAct: () => {} } }}
      extra={<QueuedBehind />}
    />
  ),
};

function QueuedBehind() {
  const toast = useToast();
  return (
    <Button variant="ghost" onClick={() => toast.show("Settings saved.")}>
      Save a setting
    </Button>
  );
}

/**
 * The long-content case, at the width a record name can reach. The sentence
 * wraps and the box stops at its own ceiling; the mark and the verbs beside it
 * keep their width, so the control never gets squeezed out by the words.
 */
export const LongContent: Story = {
  render: () => (
    <Bench
      label="Archive a long one"
      message="Archived “Nordwest Maschinenbau Vertriebsgesellschaft mbH & Co. KG — Rahmenvertrag 2026”."
      options={{ action: { label: "Undo", onAct: () => {} } }}
    />
  ),
};
