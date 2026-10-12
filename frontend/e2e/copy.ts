// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A catalog string as a whole-name pattern, so a test names a control by the
// copy it wears rather than re-spelling it: each {placeholder} given in `known`
// must read exactly that, and any other matches whatever text it was filled with.
export function copy(
  value: string,
  known: Readonly<Record<string, string>> = {},
): RegExp {
  const literal = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const pattern = value
    .split(/(\{[a-z]+\})/i)
    .map((part) => {
      const name = /^\{([a-z]+)\}$/i.exec(part)?.[1];
      if (name === undefined) return literal(part);
      return name in known ? literal(known[name]) : ".+";
    })
    .join("");
  return new RegExp(`^${pattern}$`);
}
