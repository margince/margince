// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useRef, useState } from "react";
import "./tokenspecimen.css";

// Shared by the Foundations stories that print a token beside its live value.
// A hook and some style objects, no markup, so it is not a primitive and the
// catalog has nothing to list.

/**
 * A token's value as the element it is measured on sees it, read at runtime so
 * a retune in tokens.css moves the page with it. Re-read when the toolbar flips
 * `data-theme` on the root, because the story is not remounted when it does.
 */
export function useTokenValue<T extends HTMLElement>(
  token: string,
): { ref: RefObject<T | null>; value: string; width: number } {
  const ref = useRef<T>(null);
  const [value, setValue] = useState("");
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const read = () => {
      if (ref.current) {
        setValue(getComputedStyle(ref.current).getPropertyValue(token).trim());
        setWidth(ref.current.getBoundingClientRect().width);
      }
    };
    read();
    const watch = new MutationObserver(read);
    watch.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    return () => watch.disconnect();
  }, [token]);
  return { ref, value, width };
}

const SAMPLE_HUES: Readonly<Record<string, string>> = {
  "--avatar-hue-base": "150",
  "--avatar-hue-a": "200",
  "--avatar-hue-b": "100",
};

export type ThemedValue = Readonly<{ light: string; dark: string }>;

function readAll(tokens: readonly string[]): Map<string, string> {
  const probe = document.createElement("div");
  probe.className = "avatar-mesh";
  for (const [name, hue] of Object.entries(SAMPLE_HUES)) {
    probe.style.setProperty(name, hue);
  }
  document.body.append(probe);
  const style = getComputedStyle(probe);
  const read = new Map(
    tokens.map((token) => [token, style.getPropertyValue(token).trim()]),
  );
  probe.remove();
  return read;
}

/**
 * Every token's resolved value in both themes at once. The root is flipped to
 * light, read, flipped to dark, read, and put back inside one task, so nothing
 * paints in between. Reading on the root is what makes derived tokens right: a
 * `color-mix()` over a themed base only resolves against the dark base when it
 * is declared on the element that carries the dark block. A probe element with
 * the avatar class stands in for a monogram, so the mesh colours resolve too.
 */
export function useThemeValues(
  tokens: readonly string[],
): ReadonlyMap<string, ThemedValue> {
  const [values, setValues] = useState<ReadonlyMap<string, ThemedValue>>(
    new Map(),
  );
  const key = tokens.join(" ");
  useEffect(() => {
    const root = document.documentElement;
    const original = root.getAttribute("data-theme");
    const wanted = key.split(" ");
    root.setAttribute("data-theme", "light");
    const light = readAll(wanted);
    root.setAttribute("data-theme", "dark");
    const dark = readAll(wanted);
    if (original === null) {
      root.removeAttribute("data-theme");
    } else {
      root.setAttribute("data-theme", original);
    }
    setValues(
      new Map(
        wanted.map((token) => [
          token,
          { light: light.get(token) ?? "", dark: dark.get(token) ?? "" },
        ]),
      ),
    );
  }, [key]);
  return values;
}

export const specimenColumn = "specimen-column";
export const specimenNote = "specimen-note";
export const specimenToken = "specimen-token";
export const specimenIntro = "specimen-intro";

/** A length as both units, from a resolved value or a measured width. */
export function describeLength(value: string, width: number): string {
  const parsed = /^(-?[\d.]+)(px|rem)$/.exec(value);
  const px = parsed
    ? Number(parsed[1]) * (parsed[2] === "rem" ? 16 : 1)
    : width;
  return `${Number((px / 16).toFixed(3))}rem · ${Number(px.toFixed(2))}px`;
}
