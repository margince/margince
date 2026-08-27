// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { webUrl } from "../format/weburl";

// A destination OFF our origin, drawn as a link when the address is one we may
// follow and as plain text when it is not.
//
// Three things were being spelled per call site, and a site that forgot any one
// of them was a defect nobody saw in review:
//
//   1. `rel`. `target="_blank"` without `noopener` hands the opened page a live
//      `window.opener` handle back into this tab. Several sites in this tree
//      carried `noreferrer` alone, which most engines imply opener-blocking for
//      but none promise; both are spelled here so neither depends on a default.
//   2. The scheme check. An href is untrusted record data — a crawler wrote it,
//      a connector wrote it, or a person pasted it — so it goes through
//      `webUrl`, the one place deciding whether a string may become a link.
//      `Chip` already delegates the same way; this is that rule for a value
//      that is not a chip.
//   3. What a refused address draws. It stays as TEXT: the reader keeps the
//      fact and loses only the link, which is what `Chip` settled on and the
//      opposite of hiding a value because its address was unusable.
//
// It is not `Button`: a link goes somewhere and a button does something, and
// the two carry different affordances and different keyboard contracts. It is
// not `Chip` either — a Chip is one attribute of a record with an icon naming
// its kind, and this is a bare address in a field's value column.
export function OffsiteLink({
  href,
  children,
  className = "link-button",
}: Readonly<{
  href: string;
  // What the reader sees. Callers pass a shortened or humanized form of the
  // address; the full one stays in `href`, so the row reads as a value rather
  // than as a URL.
  children: ReactNode;
  // The affordance this link wears. Defaults to the product's secondary text
  // affordance; a caller whose surface already styles its own anchors passes
  // its class instead.
  className?: string;
}>) {
  const destination = webUrl(href);
  if (!destination) {
    return <span className={className}>{children}</span>;
  }
  return (
    <a
      className={className}
      href={destination.toString()}
      target="_blank"
      rel="noopener noreferrer"
    >
      {children}
    </a>
  );
}
