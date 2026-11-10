// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { TokenReading } from "../../design-system/tokeninput";

// Loose mirror of values.ParseEmail (backend/internal/shared/kernel/values/email.go),
// a bare addr-spec: one @, a dotted domain, nothing that ends or quotes an address.
const RECIPIENT = /^[^\s,;<>"@]+@[^\s,;<>"@]+\.[^\s,;<>"@.]+$/;

// A quoted name, an address in angle brackets, a hard break, or a word. A quote
// never spans `<`, `;` or a line; only `"…"` holds a comma (`"Doe, Jane"`).
const PIECE = /"[^"<>;\n]*"|'[^'<>,;\n]*'|<[^<>]*>|[,;\n]|[^\s,;<"]+|[<"]/g;
const HARD_BREAK = new Set([";", "\n"]);

function recipient(text: string): string | null {
  const trimmed = text.trim();
  return RECIPIENT.test(trimmed) ? trimmed : null;
}

const isQuoted = (piece: string) =>
  piece.length > 1 && (piece[0] === '"' || piece[0] === "'");

/**
 * readAddressList reads a line as a mail client copies one out: addresses split
 * by commas, semicolons or white space, each bare or as `Name <address>`. The
 * name is dropped, because the send takes the address alone.
 */
export function readAddressList(raw: string): TokenReading {
  const values: string[] = [];
  const refused: string[] = [];
  // Words since the last address or hard break, cut at each comma. They are a
  // display name if an `<address>` follows, and refused text otherwise.
  let segment: string[] = [];
  let name: string[][] = [segment];
  const startName = () => {
    segment = [];
    name = [segment];
  };
  const refuseName = () => {
    refused.push(...segmentsOf(name));
    startName();
  };
  for (const [piece] of raw.matchAll(PIECE)) {
    const bare = !isQuoted(piece);
    if (HARD_BREAK.has(piece)) {
      refuseName();
    } else if (piece === ",") {
      segment = [];
      name.push(segment);
    } else if (piece.length > 1 && piece.startsWith("<")) {
      takeBracketed(piece, name, values, refused);
      startName();
    } else if (bare && piece.includes("@")) {
      // An address, or one typed wrong: never part of a name.
      refuseName();
      (recipient(piece) ? values : refused).push(piece);
    } else {
      segment.push(piece);
    }
  }
  refuseName();
  return { values, refused };
}

function segmentsOf(name: readonly (readonly string[])[]): string[] {
  return name
    .filter((segment) => segment.length > 0)
    .map((segment) => segment.join(" "));
}

// A name is the part before `<address>`, and the part one comma before that when
// both hold words and no quoted name (`Doe, Jane`). Every other part is refused.
function takeBracketed(
  piece: string,
  name: readonly (readonly string[])[],
  values: string[],
  refused: string[],
) {
  const address = recipient(piece.slice(1, -1));
  if (!address) {
    refused.push(
      [segmentsOf(name).join(", "), piece].filter(Boolean).join(" "),
    );
    return;
  }
  const first = name.at(-1) ?? [];
  const before = name.at(-2) ?? [];
  const lastFirst =
    first.length > 0 && before.length > 0 && !before.some(isQuoted);
  const reach = lastFirst ? 2 : 1;
  refused.push(...segmentsOf(name.slice(0, -reach)));
  values.push(address);
}
