// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { mailbox, telUri } from "../format/contacturi";
import { useWriteTo, type WriteToRecord } from "../screens/writeto";

// An address or a number a reader can ACT on — an address opens the product's
// composer to the record it belongs to, a number is the `tel:` link its scheme
// gives it — and plain text when the value is not one.
//
// It exists because every record page printed these as text. A reader who sees
// an address expects to click it, and a page that shows one and does nothing
// on the click teaches them the record is a read-only printout.
//
// An address is a BUTTON into the composer, because writing on the product's
// behalf is the composer's: its consent gate, its filing of the message under
// the record, its thread. A `mailto:` handed the address to the reader's own
// client and the product never saw the message — the same address, pressed on
// a header and on a card, then led to two different places. Which record the
// message is filed under is the caller's to say, because the surface showing
// the address is the one that knows. The `mailto:` remains for the one case
// the composer cannot serve: a reader with no connected mailbox to send from,
// whose own client is then the honest answer rather than a refusal.
//
// The decision about whether a string may become an action is `contacturi`'s,
// for the reason `webUrl` owns the same decision for web addresses: a value
// handed on from record data is an injection surface, and it is decided once.
// A refused value keeps the fact and loses only the action, exactly as
// `OffsiteLink` does.
type Contact = Readonly<
  {
    value: string;
    // What the reader sees. Defaults to the value itself; a caller that leads
    // with an icon, or shortens a long address, passes its own.
    children?: ReactNode;
    // The affordance this control wears. The product's secondary text
    // affordance by default; a surface that styles its own passes its class.
    className?: string;
    // What a REFUSED value wears instead. Never the control's class: a pointer
    // and an underline on text that opens nothing promise an action the reader
    // cannot take. A caller whose layout the text must keep (a mono face, a
    // wrapping width) names that here; by default the text is plain.
    textClassName?: string;
  } & (
    | { kind: "phone" }
    | {
        kind: "email";
        // The record a message to this address is filed under.
        record: WriteToRecord;
        // The record takes no writes — archived, closed — so the address is a
        // fact to keep and not a door to open. The verb beside it carries the
        // reason; a second control saying the same thing would say it twice.
        readOnly?: boolean;
      }
  )
>;

export function ContactLink(props: Contact) {
  const writeTo = useWriteTo();
  const { value, children, className = "link-button", textClassName } = props;
  const body = children ?? value;
  if (props.kind === "phone") {
    const href = telUri(value);
    if (!href) {
      return <span className={textClassName}>{body}</span>;
    }
    // No `target="_blank"`: a `tel:` is handled by the operating system, not
    // by a new tab, and a blank target on one leaves an empty tab behind in
    // some browsers.
    return (
      <a className={className} href={href}>
        {body}
      </a>
    );
  }
  const address = mailbox(value);
  // A record that takes no writes is the same case as a refused value: text,
  // because a control that opens nothing is a promise the reader cannot
  // collect on.
  if (!address || props.readOnly) {
    return <span className={textClassName}>{body}</span>;
  }
  if (!writeTo) {
    // Nothing writes from the product here, so the reader's own client does.
    // The address was admitted above, so this carries nothing but it. No
    // `target="_blank"`, for the reason the `tel:` link gives.
    return (
      <a className={className} href={`mailto:${address}`}>
        {body}
      </a>
    );
  }
  return (
    <button
      type="button"
      className={className}
      onClick={() => writeTo({ ...props.record, address })}
    >
      {body}
    </button>
  );
}
