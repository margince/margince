// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { mailtoUri, telUri } from "../format/contacturi";

// An address or a number a reader can ACT on, drawn as the link its scheme
// gives it — `mailto:` for a mailbox, `tel:` for a number — and as plain text
// when the value is not one.
//
// It exists because every record page printed these as text. A reader who sees
// an address expects to click it, and a page that shows one and does nothing
// on the click teaches them the record is a read-only printout. The link hands
// the value to the reader's own client; the product's composer, with its
// consent gate, is still the way to write on the product's behalf, which is
// why the header's own verb stays a `Button` and this stays a link.
//
// The decision about whether a string may become a link is `contacturi`'s, for
// the reason `webUrl` owns the same decision for web addresses: an href built
// from record data is an injection surface, and it is decided once. A refused
// value keeps the fact and loses only the link, exactly as `OffsiteLink` does.
export function ContactLink({
  kind,
  value,
  children,
  className = "link-button",
  textClassName,
}: Readonly<{
  kind: "email" | "phone";
  value: string;
  // What the reader sees. Defaults to the value itself; a caller that leads
  // with an icon, or shortens a long address, passes its own.
  children?: ReactNode;
  // The affordance this link wears. The product's secondary text affordance by
  // default; a surface that styles its own anchors passes its class instead.
  className?: string;
  // What a REFUSED value wears instead. Never the link's class: a pointer and
  // an underline on text that opens nothing promise an action the reader
  // cannot take. A caller whose layout the text must keep (a mono face, a
  // wrapping width) names that here; by default the text is plain.
  textClassName?: string;
}>) {
  const href = kind === "email" ? mailtoUri(value) : telUri(value);
  const body = children ?? value;
  if (!href) {
    return <span className={textClassName}>{body}</span>;
  }
  // No `target="_blank"`: a `mailto:` or `tel:` is handled by the operating
  // system, not by a new tab, and a blank target on one leaves an empty tab
  // behind in some browsers.
  return (
    <a className={className} href={href}>
      {body}
    </a>
  );
}
