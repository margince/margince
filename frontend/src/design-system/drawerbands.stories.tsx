// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { Badge, Button, Modal } from "./atoms";
import { DrawerBody, DrawerFoot, DrawerHead } from "./drawerbands";
import { Heading } from "./heading";

const meta: Meta<typeof DrawerHead> = {
  title: "Components/Overlays and layering/Drawer bands",
  component: DrawerHead,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DrawerHead>;

const CLAIMS = Array.from(
  { length: 24 },
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
          {CLAIMS.map((claim) => (
            <p key={claim} className="t-body">
              {claim}
            </p>
          ))}
        </DrawerBody>
        {wide && (
          <DrawerFoot>
            <span className="t-caption">
              Nothing is saved until you map it.
            </span>
            <Button variant="primary" onClick={() => setOpen(false)}>
              Save 3 claims
            </Button>
          </DrawerFoot>
        )}
      </Modal>
    </>
  );
}

/** The wide drawer's three bands: the head and foot stay put while the body
 *  scrolls between them, each paying its own inset to the drawer's edge. */
export const Banded: Story = {
  render: () => <BandedDrawer wide />,
};

/** The standard drawer is one scrolling column; the head only spaces the
 *  title from what follows. */
export const Standard: Story = {
  render: () => <BandedDrawer wide={false} />,
};
