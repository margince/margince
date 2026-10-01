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
import type { CalloutTone } from "../design-system/callout";
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

/** panelRow mirrors `PanelRow`: one row, ruled from the row above it. */
export function panelRow(className?: string): HTMLElement {
  return el(
    "div",
    className === undefined ? "panel-row" : `panel-row ${className}`,
  );
}

/** panelBody mirrors `PanelBody`. */
export function panelBody(): HTMLElement {
  return el("div", "panel-body");
}

/** panelFoot is the panel's footer band, under a rule. */
export function panelFoot(): HTMLElement {
  return el("footer", "panel-foot");
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
 * meter mirrors `Meter` dense and flat: a proportion as a thin bar, named for
 * assistive technology. A share that is not known draws an empty trough that
 * is hidden from assistive technology rather than announced as a meter with no
 * value: an absent factor is not a factor of zero, and the em dash beside it
 * already says so in words.
 */
export function meter(share: number | null, label: string): HTMLElement {
  const bar = el("div", "meterbar meterbar-dense meterbar-flat");
  const fill = el("span");
  bar.appendChild(fill);
  if (share === null) {
    bar.setAttribute("aria-hidden", "true");
    return bar;
  }
  const percent = Math.round(Math.min(1, Math.max(0, share)) * 100);
  bar.setAttribute("role", "meter");
  bar.setAttribute("aria-label", label);
  bar.setAttribute("aria-valuemin", "0");
  bar.setAttribute("aria-valuemax", "100");
  bar.setAttribute("aria-valuenow", String(percent));
  fill.style.width = `${percent}%`;
  return bar;
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

/**
 * callout mirrors `Callout` as a standing notice: a bordered note whose title
 * says the news and whose text says the rest. No live role, because Callout
 * gives a notice with no `kind` none — a verdict drawn with the panel is read
 * with it, not announced over it.
 */
export function callout(
  tone: CalloutTone,
  title: string,
  text: string,
): HTMLElement {
  const node = el("div", `callout callout-${tone}`);
  const body = el("div", "callout-body");
  const copy = el("div", "callout-copy");
  copy.append(el("p", "callout-title", title), el("div", "callout-text", text));
  body.appendChild(copy);
  node.appendChild(body);
  return node;
}
