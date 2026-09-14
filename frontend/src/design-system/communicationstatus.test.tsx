/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import {
  CommunicationStatus,
  CommunicationStatusLine,
} from "./communicationstatus";

afterEach(cleanup);

// What the mark owes a reader who cannot see it. An icon-only control with no
// accessible name is announced as "button", which is the entire disclosure a
// blind rep would get about whether their message can go.
it("names the subject and the state, not just 'button'", async () => {
  render(
    <CommunicationStatus
      state="restricted"
      scope="contact_preferences"
      label="They asked us to stop marketing"
      name="Communication status for Anna: marketing stopped. View details."
    />,
  );
  expect(
    screen.getByRole("button", {
      name: /Communication status for Anna: marketing stopped/,
    }),
  ).toBeTruthy();
});

// The label is not rendered visibly unless asked for, because forty labels down
// a table is noise — but it must still reach a screen reader, or the dense form
// says nothing at all.
it("keeps the name available when the label is hidden", () => {
  render(
    <CommunicationStatus
      state="context_only"
      scope="contact_preferences"
      size={16}
      label="Checked for each message"
      name="Communication status for Bruno. View details."
    />,
  );
  expect(screen.queryByText("Checked for each message")).toBeNull();
  expect(
    screen.getByRole("button", { name: /Communication status for Bruno/ }),
  ).toBeTruthy();
});

it("shows the label beside the mark when the surface asks for it", () => {
  render(
    <CommunicationStatus
      state="ready"
      scope="current_message"
      showLabel
      label="Ready to send"
      name="Communication status: ready to send. View details."
    />,
  );
  expect(screen.getByText("Ready to send")).toBeTruthy();
});

it("opens its aside on click and closes on Escape", async () => {
  const user = userEvent.setup();
  render(
    <CommunicationStatus
      state="attention"
      scope="current_message"
      label="Needs a look"
      name="Communication status: needs a look. View details."
      detail={<p>No lawful basis on file</p>}
    />,
  );
  const trigger = screen.getByRole("button", { name: /needs a look/i });
  await user.click(trigger);
  expect(screen.getByText("No lawful basis on file")).toBeTruthy();
  await user.keyboard("{Escape}");
  expect(screen.queryByText("No lawful basis on file")).toBeNull();
});

// THE EXCEPTION NEVER RECOLOURS THE MARK. A message sent under a recorded
// exception was refused and sent anyway; drawing it as resolved would falsify
// the record the exception path exists to keep. The text appears, the class
// stays restricted.
it("shows a recorded exception without turning the mark green", async () => {
  const user = userEvent.setup();
  render(
    <CommunicationStatus
      state="restricted"
      scope="historical_message"
      label="Marketing objection at send time"
      name="Communication status: marketing objection. View details."
      exception="Sent with a recorded exception"
    />,
  );
  const trigger = screen.getByRole("button", { name: /marketing objection/i });
  expect(trigger.className).toContain("commstatus-restricted");
  expect(trigger.className).not.toContain("commstatus-ready");
  await user.click(trigger);
  expect(screen.getByText("Sent with a recorded exception")).toBeTruthy();
});

// `checking` says the answer has not come back. It must not wear the check
// glyph, because a reader glancing at it would read "ready" from a state that
// means "we do not know yet" — which is the exact silence this replaces.
it("draws checking as busy and never as a resolved answer", () => {
  const { container } = render(
    <CommunicationStatus
      state="checking"
      scope="current_message"
      label="Checking…"
      name="Communication status: checking. View details."
    />,
  );
  expect(container.querySelector(".commstatus-busy")).toBeTruthy();
  // NO ENVELOPE GLYPH AT ALL while the answer is outstanding. Asserting only
  // the busy mark's presence let the check glyph sit beside it and still pass,
  // which is the exact misreading this guards: a reader glancing at a tick
  // reads "ready" from a state that means "we do not know yet".
  expect(container.querySelector("svg.lucide-mail-check")).toBeNull();
  expect(container.querySelector("svg.lucide-mail")).toBeNull();
});

// The mark sits BESIDE the verbs, never inside one. A button nested in a button
// is invalid and unoperable: the outer press swallows the inner, so a rep
// reaching for the panel opens the composer instead.
it("seats the mark beside the action rather than inside it", () => {
  const { container } = render(
    <CommunicationStatusLine
      status={
        <CommunicationStatus
          state="ready"
          scope="current_message"
          label="Ready to send"
          name="Communication status: ready. View details."
        />
      }
    >
      <button type="button">Send</button>
    </CommunicationStatusLine>,
  );
  expect(container.querySelector("button button")).toBeNull();
  expect(screen.getAllByRole("button")).toHaveLength(2);
});

// A CONTACT SURFACE CANNOT SAY "READY". `ready` under current_message says this
// message may go, which a surface holding a message can know. Under
// contact_preferences it would say every future message may go, which nothing
// can know — the answer depends on what the message turns out to be.
//
// It softens to context_only rather than throwing, because the caller is often
// an adapter mapping a server answer and a crash would be worse than drawing a
// quieter true thing.
it("softens an over-confident ready on a contact surface", () => {
  render(
    <CommunicationStatus
      state="ready"
      scope="contact_preferences"
      label="Checked for each message"
      name="Communication status for Anna. View details."
    />,
  );
  const trigger = screen.getByRole("button", {
    name: /Communication status for Anna/,
  });
  expect(trigger.className).toContain("commstatus-context_only");
  expect(trigger.className).not.toContain("commstatus-ready");
});

// The same reading on a surface that DOES hold a message keeps its answer.
// Without this the rule above could be "never draw ready anywhere", which would
// take the composer's quiet confirmation away with it.
it("keeps ready where a message is actually in hand", () => {
  render(
    <CommunicationStatus
      state="ready"
      scope="current_message"
      label="Ready to send"
      name="Communication status: ready to send. View details."
    />,
  );
  const trigger = screen.getByRole("button", { name: /ready to send/i });
  expect(trigger.className).toContain("commstatus-ready");
});

// A standing objection is true however you reach somebody, so a contact surface
// must still be able to say it. Softening every state would hide the one
// answer a contact surface most needs to give.
it("still says restricted on a contact surface", () => {
  render(
    <CommunicationStatus
      state="restricted"
      scope="contact_preferences"
      label="They asked us to stop marketing"
      name="Communication status for Bruno: marketing stopped. View details."
    />,
  );
  const trigger = screen.getByRole("button", { name: /Bruno/ });
  expect(trigger.className).toContain("commstatus-restricted");
});
