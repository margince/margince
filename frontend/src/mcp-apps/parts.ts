// The app's components, as the standalone views build them.
//
// A view carries no React, so it cannot render <Panel>, <Badge> or <Avatar>.
// What it can do is emit the same markup those components emit, wearing the
// same classes, and import the same sheets through view.css. That is what
// this module is: one builder per component, each a mirror of the component
// named beside it, so a panel in a host's frame is the app's panel rather
// than a look of its own. The badge's builder is badge.ts, alone, because it
// is the one the badge-spelling gate has to exempt by file.
//
// Every builder composes el() and heading() from bridge.ts. Text arrives as
// text and never as markup, so the containment property stated there holds
// for every node built here too.
//
// NO GLYPHS. The app's callout icon and the Sparkles on an AI badge are SVG,
// and an SVG element is only built through its namespace URI — a string the
// admission check refuses in a view, as it refuses every absolute URL. So a
// view's callout and AI badge carry their meaning in their words alone, which
// the components already guarantee they do.

import { meshOf, meshStyle, monogramOf } from "../design-system/avatarmesh";
import type { PanelTone } from "../design-system/panel";
import type { StrengthBand } from "../design-system/strengthmeter";
import { el, heading } from "./bridge";

let nextTitleID = 1;

/**
 * panel mirrors `Panel`: a section named by the title in its head band, with
 * an optional badge or count at the band's far end. The caller appends rows,
 * a body or a foot after it.
 */
export function panel(
  title: string,
  opts?: Readonly<{
    tone?: PanelTone;
    level?: "h1" | "h2";
    action?: HTMLElement;
  }>,
): HTMLElement {
  const section = el(
    "section",
    opts?.tone === undefined ? "panel" : `panel panel-${opts.tone}`,
  );
  const id = `panel-title-${nextTitleID++}`;
  section.setAttribute("aria-labelledby", id);
  const head = el("header", "panel-head");
  const name = heading("medium", title, {
    as: opts?.level ?? "h2",
    className: "panel-title",
  });
  name.id = id;
  head.appendChild(name);
  if (opts?.action !== undefined) head.appendChild(opts.action);
  section.appendChild(head);
  return section;
}

/** panelBody mirrors `PanelBody`. */
export function panelBody(): HTMLElement {
  return el("div", "panel-body");
}

/**
 * avatar mirrors `Avatar`: the monogram on the mesh keyed by the record's own
 * id, so a colleague is one chip in the app and in a host's panel — `sm` in a
 * list row, `md` in a record's head. Hidden from assistive technology, because
 * the name beside it already says who it is.
 */
export function avatar(
  name: string,
  identity: string,
  size: "sm" | "md" = "sm",
): HTMLElement {
  const node = el(
    "span",
    `avatar avatar-mesh avatar-${size}`,
    monogramOf(name),
  );
  node.setAttribute("aria-hidden", "true");
  for (const [property, value] of Object.entries(
    meshStyle(meshOf(identity || name)),
  )) {
    node.style.setProperty(property, String(value));
  }
  return node;
}

/**
 * button mirrors `Button`: the same `btn` classes the app's sheet draws, as a
 * plain button because a view has no form to submit. `onPress` is the whole
 * behaviour, and `busy` mirrors `pending` — aria-disabled rather than disabled,
 * so focus stays where the reader put it while the write is out.
 */
export function button(
  label: string,
  variant: "primary" | "ghost",
  onPress: () => void,
  busy = false,
): HTMLElement {
  const node = el("button", `btn btn-${variant}`, label);
  node.setAttribute("type", "button");
  if (busy) {
    node.setAttribute("aria-busy", "true");
    node.setAttribute("aria-disabled", "true");
    return node;
  }
  node.addEventListener("click", onPress);
  return node;
}

/** strengthMeter mirrors `StrengthMeter`: three rising bars beside the word. */
export function strengthMeter(band: StrengthBand, word: string): HTMLElement {
  const node = el("span", "strength-meter");
  node.dataset.band = band;
  const bars = el("span", "strength-meter-bars");
  bars.setAttribute("aria-hidden", "true");
  for (let i = 0; i < 3; i++) bars.appendChild(el("i"));
  node.append(bars, word);
  return node;
}
