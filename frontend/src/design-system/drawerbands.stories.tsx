// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { expect, screen, waitFor, within } from "storybook/test";
import { Badge, Button, Field, Modal, TextInput } from "./atoms";
import { DrawerBody, DrawerFoot, DrawerHead } from "./drawerbands";
import { Heading } from "./heading";

const meta: Meta<typeof DrawerHead> = {
  title: "Components/Overlays and layering/Drawer bands",
  component: DrawerHead,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DrawerHead>;

// A claim per 12px of window: each is a line or more, so the body overflows
// at any canvas height.
const claims = () =>
  Array.from(
    { length: Math.ceil(window.innerHeight / 12) },
    (_, n) =>
      `Claim ${n + 1}: Globex GmbH named a procurement lead on its careers page, quoted with the date it was read.`,
  );

function BandedDrawer({ wide }: Readonly<{ wide: boolean }>) {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Open the drawer
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent={wide ? "drawer-reading" : "drawer"}
      >
        <DrawerHead>
          <Heading size="large" id={titleId} className="t-h2 modal-title">
            Research on Anna Brandt
          </Heading>
          <Badge>Public sources only</Badge>
        </DrawerHead>
        <DrawerBody>
          {claims().map((claim) => (
            <p key={claim} className="t-body">
              {claim}
            </p>
          ))}
        </DrawerBody>
        <DrawerFoot>
          <span className="t-caption">Nothing is saved until you map it.</span>
          <Button variant="primary" onClick={() => setOpen(false)}>
            Save 3 claims
          </Button>
        </DrawerFoot>
      </Modal>
    </>
  );
}

// A form drawer: its body is the field stack and its foot the action row, the
// shape the dialog footer pin also reads.
function FormBandedDrawer({ wide }: Readonly<{ wide: boolean }>) {
  const titleId = useId();
  return (
    <Modal
      open
      onClose={() => undefined}
      labelledBy={titleId}
      intent={wide ? "drawer-reading" : "drawer"}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          Map a claim
        </Heading>
      </DrawerHead>
      <DrawerBody className="form-stack">
        <Field label="Field on the record">
          {(control) => <TextInput {...control} defaultValue="Buying role" />}
        </Field>
        <Field label="Value">
          {(control) => (
            <TextInput {...control} defaultValue="Procurement lead" />
          )}
        </Field>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button variant="ghost">Cancel</Button>
        <Button variant="primary">Map claim</Button>
      </DrawerFoot>
    </Modal>
  );
}

const bodyScrollsToItsEnd = async () => {
  const dialog = await screen.findByRole("dialog", {
    name: "Research on Anna Brandt",
  });
  const body = dialog.querySelector<HTMLElement>(".drawer-body");
  if (!body) throw new Error("The drawer drew no body band.");
  await Promise.all(document.getAnimations().map((motion) => motion.finished));
  await expect(body.scrollHeight).toBeGreaterThan(body.clientHeight);
  const save = within(dialog).getByRole("button", { name: "Save 3 claims" });
  const footTop = save.getBoundingClientRect().top;
  body.scrollTop = body.scrollHeight;
  const last = within(body)
    .getAllByText(/^Claim \d+:/)
    .at(-1);
  if (!last) throw new Error("The drawer body drew no claims.");
  await waitFor(() =>
    expect(last.getBoundingClientRect().bottom).toBeLessThanOrEqual(
      body.getBoundingClientRect().bottom + 1,
    ),
  );
  await expect(save.getBoundingClientRect().top).toBe(footTop);
  await expect(save.getBoundingClientRect().bottom).toBeLessThanOrEqual(
    dialog.getBoundingClientRect().bottom,
  );
};

/** The reading drawer's three bands, each paying its own inset to the
 *  drawer's edge. */
export const Banded: Story = {
  render: () => <BandedDrawer wide />,
  play: bodyScrollsToItsEnd,
};

/** The standard drawer: the drawer's padding frames the three bands, and the
 *  body scrolls between the head and the foot. */
export const Standard: Story = {
  render: () => <BandedDrawer wide={false} />,
  play: bodyScrollsToItsEnd,
};

export const BandedDark: Story = { ...Banded, globals: { theme: "dark" } };

const holdsItsWidth = async () => {
  const dialog = await screen.findByRole("dialog", { name: "Map a claim" });
  await Promise.all(document.getAnimations().map((motion) => motion.finished));
  const body = dialog.querySelector<HTMLElement>(".drawer-body");
  if (!body) throw new Error("The drawer drew no body band.");
  await expect(dialog.scrollWidth).toBeLessThanOrEqual(dialog.clientWidth);
  await expect(body.getBoundingClientRect().right).toBeLessThanOrEqual(
    dialog.getBoundingClientRect().right,
  );
};

/** A form in the reading drawer: the body band is the field stack and the
 *  foot band the action row, and the drawer never scrolls sideways. */
export const FormBands: Story = {
  render: () => <FormBandedDrawer wide />,
  play: holdsItsWidth,
};

/** The same form in the standard drawer. */
export const FormBandsStandard: Story = {
  render: () => <FormBandedDrawer wide={false} />,
  play: holdsItsWidth,
};
