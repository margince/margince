// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./atoms";
import { Disclosure } from "./disclosure";

// Disclosure: a section the reader opens when they want it. The cases here are
// the ones the component distinguishes — closed by default, forced open, a
// verb on the summary's line but outside its control, and a named group whose
// members open one at a time.
const meta: Meta<typeof Disclosure> = {
  title: "Design System/Disclosure",
  component: Disclosure,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Disclosure>;

export const Closed: Story = {
  render: () => (
    <Disclosure summary="Matching rules">
      <p className="t-caption">
        Closed by default: the reader pays one line for a surface they rarely
        open.
      </p>
    </Disclosure>
  ),
};

export const Open: Story = {
  render: () => (
    <Disclosure summary="Import log" open>
      <p className="t-caption">
        Forced open for a state the reader must not miss — a run in progress, or
        a result that just arrived.
      </p>
    </Disclosure>
  ),
};

export const WithAction: Story = {
  render: () => (
    <Disclosure
      summary="Employments"
      action={
        <Button small variant="link">
          Add employment
        </Button>
      }
    >
      <p className="t-caption">
        The verb sits on the summary's line and outside the summary, so pressing
        it does not toggle the section under it.
      </p>
    </Disclosure>
  ),
};

export const Accordion: Story = {
  render: () => (
    <div>
      <Disclosure summary="First message" name="accordion">
        <p className="t-caption">
          Sharing a name with the next: opening one closes the other, so a
          column of folds never holds two bodies open.
        </p>
      </Disclosure>
      <Disclosure summary="Second message" name="accordion">
        <p className="t-caption">The other member of the same group.</p>
      </Disclosure>
    </div>
  ),
};
