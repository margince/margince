import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { Button, Modal, SectionHeader } from "./atoms";
import { Heading } from "./heading";

// fe-uat (frontend/scripts/fe-uat.mjs) maps modal.tsx → modal.stories.tsx and
// fails a change to modal.tsx whose stories here do not render clean.
//
// Every one of these opens ON MOUNT, because a dialog rendered closed
// screenshots as an empty canvas. The trigger stays so the reader can reopen it
// after dismissing, which is also the only way to see the surface arrive.
const meta: Meta = {
  title: "Design System/Modal",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The centred box everyone pictures when they read "Modal".
function ModalDemo() {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Open the dialog
      </Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy={titleId}>
        <Heading
          size="large"
          id={titleId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          Merge these companies?
        </Heading>
        <p className="t-caption">
          Globex GmbH keeps its record; the duplicate's activities, deals and
          contacts move onto it. This cannot be undone.
        </p>
        <div className="actions">
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button variant="danger" onClick={() => setOpen(false)}>
            Merge
          </Button>
        </div>
      </Modal>
    </>
  );
}

export const Dialog: Story = {
  render: () => <ModalDemo />,
};

// placement="right" is the drawer form of the SAME Modal: anchored to the
// trailing edge, the record behind it still legible. One component, one prop
// between it and the centred dialog everyone pictures when they read "Modal".
//
// It FLOATS — a --space-4 gap on three sides, the pane radius and the pop
// shadow — so the scrim runs all the way round it. On a phone the gap goes and
// the same drawer is a full-screen sheet; the Storybook viewport control is
// where to see that, because it is the width that decides, not a prop.
function DrawerDemo() {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      {/* Something behind the drawer, because "the record stays legible" is
          the whole claim the placement makes and an empty canvas cannot show
          it being kept. */}
      <SectionHeader title="Globex GmbH" />
      <p className="t-body">
        Anna Brandt replied on Tuesday and is waiting on pricing. Nobody has
        written since.
      </p>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Open the drawer
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        placement="right"
      >
        <Heading
          size="large"
          id={titleId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          Write to Anna Brandt
        </Heading>
        <p className="t-caption">
          The draft sits beside the record it is about, so a rep can read the
          history while writing rather than remembering it.
        </p>
        <div className="actions">
          <Button onClick={() => setOpen(false)}>Discard</Button>
          <Button variant="primary" onClick={() => setOpen(false)}>
            Send
          </Button>
        </div>
      </Modal>
    </>
  );
}

export const Drawer: Story = {
  render: () => <DrawerDemo />,
};

// The WIDE drawer, which is the one that holds bands: a sticky head, a
// scrolling body and a sticky foot, each paying its own padding to the drawer's
// own edge. That is what the clipped corners are for — without them the two
// bands square off the radius they sit in — and it is also where the close
// control has to stay legible, drawn over the head rather than under it.
function WideDrawerDemo() {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      <SectionHeader title="Globex GmbH" />
      <Button variant="primary" onClick={() => setOpen(true)}>
        Open the brief
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        placement="right"
        size="wide"
      >
        <div className="drawer-head">
          <Heading size="large" id={titleId} className="t-h2">
            Before the room with Anna Brandt
          </Heading>
          <p className="t-caption">Tuesday 14:00 · 40 minutes · Munich</p>
        </div>
        <div className="drawer-body">
          {[
            "The objective: leave with a signed pilot scope.",
            "The risk: procurement has not seen the security pack.",
            "The response: send it Monday, named to Anna's counsel.",
            "The close plan: pilot in March, rollout in June, review in September.",
          ].map((line) => (
            <p key={line} className="t-body">
              {line}
            </p>
          ))}
        </div>
        <div className="drawer-foot">
          <Button onClick={() => setOpen(false)}>Discard</Button>
          <Button variant="primary" onClick={() => setOpen(false)}>
            Send the brief
          </Button>
        </div>
      </Modal>
    </>
  );
}

export const DrawerWide: Story = {
  render: () => <WideDrawerDemo />,
};
