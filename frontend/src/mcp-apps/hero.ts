// The lead of a card: the one figure it is about, the sentence that reads it,
// and a picture of its parts. A reader in a chat takes a card in at a glance,
// so the answer comes first and the rows after it.
//
// The pictures are CSS on plain elements. A view cannot build SVG (the
// namespace is an absolute URL, which the admission check refuses).

import { el } from "./bridge";
import { eyebrow } from "./parts";

/** One part of a whole, drawn as a slice or a segment in a status tone. */
export type Part = Readonly<{
  label: string;
  value: number;
  tone: "accent" | "success" | "warning" | "danger" | "neutral";
  shown?: string;
}>;

/** hero builds the lead: a kicker, the figure, the sentence and a picture. */
export function hero(
  opts: Readonly<{
    kicker: string;
    figure: string;
    caption: string;
    sentence: string;
    picture?: HTMLElement;
    beside?: HTMLElement;
  }>,
): HTMLElement {
  const lead = el("section", "view-hero");
  const words = el("div", "view-hero-words");
  const figure = el("p", "view-hero-figure");
  figure.append(
    el("span", "view-hero-number", opts.figure),
    el("span", "view-hero-caption", opts.caption),
  );
  words.append(
    eyebrow(opts.kicker),
    figure,
    el("p", "view-hero-sentence", opts.sentence),
  );
  lead.appendChild(words);
  if (opts.beside !== undefined) {
    lead.classList.add("view-hero-split");
    lead.appendChild(opts.beside);
  }
  if (opts.picture !== undefined) lead.appendChild(opts.picture);
  return lead;
}

/**
 * segments draws parts of one total on one track, each as wide as its share,
 * with a legend that names every part. The track is decoration for a reader
 * who also has the legend, so it is hidden from assistive technology.
 */
export function segments(parts: readonly Part[]): HTMLElement {
  const total = parts.reduce((sum, p) => sum + Math.max(0, p.value), 0);
  const wrap = el("div", "view-segments");
  const track = el("div", "view-segments-track");
  track.setAttribute("aria-hidden", "true");
  for (const part of parts) {
    if (total <= 0 || part.value <= 0) continue;
    const slice = el("span", "view-segment");
    slice.dataset.tone = part.tone;
    slice.style.flexGrow = String(part.value / total);
    track.appendChild(slice);
  }
  wrap.append(track, legend(parts));
  return wrap;
}

/**
 * ring draws parts of one whole as a ring around its count, the count in the
 * middle and a legend beside it. A conic gradient paints it, one stop per part.
 */
export function ring(
  parts: readonly Part[],
  centre: string,
  centreWord: string,
): HTMLElement {
  const total = parts.reduce((sum, p) => sum + Math.max(0, p.value), 0);
  const wrap = el("div", "view-ring-wrap");
  const disc = el("div", "view-ring");
  disc.setAttribute("aria-hidden", "true");
  let from = 0;
  const stops: string[] = [];
  for (const part of parts) {
    if (total <= 0 || part.value <= 0) continue;
    const to = from + (part.value / total) * 100;
    stops.push(`var(--ring-${part.tone}) ${from}% ${to}%`);
    from = to;
  }
  disc.style.setProperty(
    "--ring-fill",
    stops.length === 0
      ? "var(--bgChip)"
      : `conic-gradient(${stops.join(", ")})`,
  );
  const middle = el("div", "view-ring-middle");
  middle.append(
    el("span", "view-ring-count", centre),
    el("span", "view-ring-word", centreWord),
  );
  disc.appendChild(middle);
  wrap.append(disc, legend(parts));
  return wrap;
}

function legend(parts: readonly Part[]): HTMLElement {
  const list = el("ul", "view-legend");
  for (const part of parts) {
    const item = el("li", "view-legend-item");
    const swatch = el("span", "view-swatch");
    swatch.dataset.tone = part.tone;
    item.append(
      swatch,
      el("span", "view-legend-label", part.label),
      el("span", "view-legend-value", part.shown ?? String(part.value)),
    );
    list.appendChild(item);
  }
  return list;
}

/**
 * share draws one item's part of the whole as a thin bar: the same bar the
 * app's `Meter dense` draws, in the item's tone.
 */
export function share(fraction: number, tone: "warning" | "danger" | "flat") {
  const bar = el("span", `meterbar meterbar-dense meterbar-${tone} view-share`);
  bar.setAttribute("aria-hidden", "true");
  const fill = el("span");
  fill.style.width = `${Math.round(Math.min(1, Math.max(0, fraction)) * 100)}%`;
  bar.appendChild(fill);
  return bar;
}
