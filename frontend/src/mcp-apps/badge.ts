// The views' Badge: the one builder outside atoms.tsx that may mint the badge
// class. It is a module of its own so badge-spelling.test.ts can exempt it by
// file without exempting any other builder beside it.

import type { BADGE_TONES } from "../design-system/atoms";
import { el } from "./bridge";

type BadgeTone = (typeof BADGE_TONES)[number];

/** badge mirrors `Badge` in its soft variant, the one a status wears. */
export function badge(text: string, tone: BadgeTone = "default"): HTMLElement {
  const node = el("span", tone === "default" ? "badge" : `badge badge-${tone}`);
  node.appendChild(el("span", "badge-label", text));
  return node;
}
