// The App-side half of the MCP Apps postMessage transport (SEP-1865,
// 2026-01-26): a view is an MCP client that reaches its host through
// window.parent rather than through a socket.
//
// It is imported by EVERY view and folded into each built document, so the
// handshake exists once. A per-view copy would be two implementations of one
// protocol, and the failure they would eventually differ on is the one where a
// view renders nothing because it never announced itself.
//
// WHAT THIS FILE REFUSES TO DO, and why each refusal is load-bearing:
//
//   It never builds DOM from a string. Everything a view displays arrives in
//   `structuredContent`, which is customer data — a contact's name, a note
//   someone pasted, the subject line of an ingested email. That is untrusted
//   text by this system's own reckoning, and the view runs inside a sandbox
//   whose whole job is to contain it. Assigning that text as MARKUP would hand
//   it the one privilege the sandbox cannot take back: execution inside the
//   view's own origin. So text reaches the page only through textContent, and
//   structure only through createElement.
//
//   The markup-assignment properties are not named here, deliberately: the
//   admission check reads the built document, and prose that spelled one would
//   trip a check on the very thing it was explaining.
//
//   It holds no credential. A view is given a tool's ANSWER, never the means to
//   ask again. There is no token here to leak, which is a stronger property
//   than a token handled carefully.
//
//   It calls no tool of its own accord. A view that must act on the user's
//   click imports actions.ts, which may ask the HOST to run a tool the view
//   declared in actions.json and nothing else; a view that does not import it
//   carries no code that sends a request. The host routes the call through the
//   same passport and approval checks as any other.

import { minorUnitDigits, toMajorUnits } from "../format/minorunits";
import { followContentSize } from "./size";
import { applyTheme, followHostChange } from "./theme";
import {
  asFiniteNumber,
  asRecord,
  asText,
  asWarnings,
  type Warning,
} from "./types";

const PROTOCOL_VERSION = "2026-01-26";

/** What a view shows for a value it does not have. Never "NaN", never "0". */
export const ABSENT = "—";

/** The handler a view registers, called once per tool result the host pushes. */
type ResultHandler = (data: unknown, warnings: Warning[]) => void;

let nextID = 1;

/** nextRequestID mints the id of an outbound request, shared with the handshake's
 *  so no two requests of one view carry the same one. */
export function nextRequestID(): number {
  return nextID++;
}
let resultHandler: ResultHandler | null = null;
// The id our own ui/initialize was sent under, cleared as it is consumed.
let initializeID: number | null = null;
// Whether the handshake has completed. A view is only supposed to be given a
// result after it has announced itself, and these two states are what let
// handle() refuse anything out of order.
let initialized = false;
let sizing = false;
// The host's origin, LEARNED rather than configured.
//
// A view is loaded into an opaque sandbox origin and cannot know its host's
// ahead of time — reading window.parent.origin throws cross-origin, and the
// specification therefore prescribes '*' for the opening message. But the
// response to it arrives with the host's origin attached, so from that point on
// there IS something to pin: every later message is sent to that origin and
// accepted only from it.
//
// 'null' is kept as a sentinel rather than pinned. An opaque origin reports
// itself as the STRING "null", which is not a usable postMessage target, so a
// host in one keeps the wildcard — the sender check below is what holds there.
let hostOrigin: string | null = null;

/** What a view that acts on its host registers: the host's response to a
 *  request the view sent, and the capabilities the host announced. */
type ResponseHandler = (message: Record<string, unknown>) => void;
type CapabilityHandler = (hostCapabilities: unknown) => void;
let responseHandler: ResponseHandler | null = null;
let capabilityHandler: CapabilityHandler | null = null;

/** onResponse registers the one handler for responses to this view's own
 *  requests. Only actions.ts calls it, so a view that declares no action
 *  never carries the code that sends one. */
export function onResponse(
  fn: ResponseHandler,
  capabilities: CapabilityHandler,
): void {
  responseHandler = fn;
  capabilityHandler = capabilities;
}

/**
 * fromHost checks every inbound message on TWO things: it came from the frame
 * that embedded us, and — once the host's origin is known — from that origin.
 *
 * The sender check is the stronger of the two and it holds from the first
 * message: a sandboxed view can still be messaged by anything holding a handle
 * to its window, and a view that rendered whatever arrived would let a second
 * sender choose what the human is shown. The origin check adds what the sender
 * check cannot see, which is a host that navigated the parent frame somewhere
 * else between the handshake and the result.
 */
function fromHost(event: MessageEvent): boolean {
  if (event.source !== window.parent) return false;
  if (hostOrigin === null || hostOrigin === "null") return true;
  return event.origin === hostOrigin;
}

/**
 * send posts one message to the host, pinned to its origin once that is known
 * and '*' only for the opening message, which is sent before there is anything
 * to learn it from.
 *
 * Nothing sensitive travels outward on either path: a handshake, and for a
 * view with actions, a named tool call with its arguments. No credential
 * ever leaves here, because a view holds none.
 */
export function sendToHost(message: Record<string, unknown>): void {
  const target =
    hostOrigin === null || hostOrigin === "null" ? "*" : hostOrigin;
  window.parent.postMessage({ jsonrpc: "2.0", ...message }, target);
}

/**
 * announce opens the handshake.
 *
 * THE MEMBER NAMES ARE THE EXTENSION'S, NOT THE CORE PROTOCOL'S. A view
 * announces itself with `appInfo` and `appCapabilities`; `clientInfo` and
 * `capabilities` are what an MCP CLIENT sends on the transport below, and they
 * are the obvious wrong guess because every other handshake in this system
 * spells them that way. A host validating the request against the extension's
 * schema refuses the wrong pair outright — the view then loads, sandboxes, and
 * sits blank forever, because a refused initialise produces no error anywhere
 * the document can show.
 *
 * `appCapabilities` is deliberately EMPTY. Every member of it — tools the host
 * may call, display modes, experimental features — is a capability these views
 * do not have and must not claim: a view here is a renderer, and the widest
 * part of this extension's surface is the part where it stops being one.
 */
function announce(): number {
  const id = nextRequestID();
  sendToHost({
    id,
    method: "ui/initialize",
    params: {
      protocolVersion: PROTOCOL_VERSION,
      appInfo: { name: "margince-view", version: "1" },
      appCapabilities: {},
    },
  });
  return id;
}

/**
 * completeHandshake answers the response to our own ui/initialize, which is the
 * readiness signal: the host is listening, so we confirm and then wait to be
 * given a result.
 *
 * The id is CLEARED as it is consumed, so a repeated response cannot make the
 * view announce itself twice. A host that received two
 * ui/notifications/initialized would be entitled to read the second as a fresh
 * view and re-send everything it had already sent.
 */
function completeHandshake(
  event: MessageEvent,
  message: Record<string, unknown>,
): void {
  initializeID = null;
  initialized = true;
  // Learned here and only here, from the one message whose sender is already
  // proven to be the embedding frame.
  hostOrigin = event.origin;
  applyTheme(asRecord(message.result).hostContext);
  capabilityHandler?.(asRecord(message.result).hostCapabilities);
  sendToHost({ method: "ui/notifications/initialized", params: {} });
}

/**
 * deliverResult hands one tool answer to the view's renderer.
 *
 * `params` IS the CallToolResult — `structuredContent` sits directly on it, not
 * under a nested member — and that structuredContent is the envelope every tool
 * on this surface seals its answer into: `data` is the tool's own result,
 * `warnings` are the conditions the answer came with.
 *
 * THE WARNINGS ARE PASSED ON, not dropped. A bounded read reports its bound as a
 * warning rather than in its data — `who_knows` stops at a cap and says so — so
 * a renderer given only `data` would present a truncated list as the whole
 * network. That is the one thing the tool's own contract forbids in those words.
 */
function deliverResult(message: Record<string, unknown>): void {
  if (resultHandler === null) return;
  const envelope = asRecord(asRecord(message.params).structuredContent);
  resultHandler(envelope.data ?? null, asWarnings(envelope.warnings));
  // Sized from the first answer drawn: before it the document is only padding,
  // and a host told that height shrinks the frame just to grow it again.
  if (!sizing) {
    sizing = true;
    followContentSize(sendToHost);
  }
}

function handle(event: MessageEvent): void {
  if (!fromHost(event)) return;
  const message = asRecord(event.data);
  if (message.jsonrpc !== "2.0") return;
  // `"result" in message` rather than a truthy check: `"result": null` is a
  // legal JSON-RPC response, and treating it as "not answered yet" left the view
  // permanently blank — no re-announce, no timeout, every later result dropped,
  // and nothing anywhere saying why.
  if (
    initializeID !== null &&
    message.id === initializeID &&
    "result" in message
  ) {
    completeHandshake(event, message);
    return;
  }
  if (
    responseHandler !== null &&
    initialized &&
    typeof message.id === "number" &&
    ("result" in message || "error" in message)
  ) {
    responseHandler(message);
    return;
  }
  // The host telling us something about the frame changed — a theme switch, a
  // resize. Followed rather than ignored: a view that read the theme once at
  // initialise sits in the old palette until it is closed and reopened, inside
  // a host that has already repainted around it.
  //
  // `params` is handed over WHOLE, because it IS the context here rather than
  // carrying one under `hostContext` the way the initialize result does. That
  // difference, and the partial-update rule, live in followHostChange.
  if (
    message.method === "ui/notifications/host-context-changed" &&
    initialized
  ) {
    followHostChange(message.params);
    return;
  }
  // A result BEFORE the handshake is dropped rather than rendered. The view has
  // not been told the theme or the display mode yet, so rendering then shows the
  // human a panel drawn against defaults the host already corrected — and
  // accepting data outside the sequence is how a view ends up rendering whatever
  // arrives whenever it arrives.
  if (message.method === "ui/notifications/tool-result" && initialized) {
    deliverResult(message);
  }
}

/**
 * onResult registers the view's renderer. `warnings` is always an array, so a
 * renderer never has to decide whether an absent one means "none" or "not
 * computed".
 */
export function onResult(fn: ResultHandler): void {
  resultHandler = fn;
}

/**
 * warned reports whether one condition was raised. The codes belong to the
 * envelope, so a view asks by code rather than reading prose it would then have
 * to keep in step with the server's wording.
 */
export function warned(warnings: Warning[], code: string): boolean {
  return warnings.some((w) => w.code === code);
}

/**
 * el and heading below are the ONLY two ways anything reaches the page, and
 * both take text rather than markup. parts.ts and badge.ts compose them into
 * the app's components and touch the document no other way, which is what keeps the
 * containment property at the top of this file true of every character a
 * reader sees.
 *
 * el REFUSES a heading tag at compile time. A heading is not just an element
 * here — it carries a size token and the class the stylesheet keys off — so a
 * view that reached for `el("h2", …)` would render a heading with no type on
 * it, in a document where nothing else says what a heading looks like. The type
 * sends that call to heading() instead of leaving it to review.
 */
export function el(
  tag: Exclude<
    keyof HTMLElementTagNameMap,
    "h1" | "h2" | "h3" | "h4" | "h5" | "h6"
  >,
  className?: string,
  text?: string | number,
): HTMLElement {
  const node = document.createElement(tag);
  if (className !== undefined) node.className = className;
  if (text !== undefined) node.textContent = String(text);
  return node;
}

/** The sizes a heading can be, the app's seven --fontHeading* steps. */
export type HeadingSize =
  | "xxlarge"
  | "xlarge"
  | "large"
  | "medium"
  | "small"
  | "xsmall"
  | "xxsmall";

/** What a heading may be rendered as when the size's own element is wrong. */
export type HeadingElement =
  | "h1"
  | "h2"
  | "h3"
  | "h4"
  | "h5"
  | "h6"
  | "div"
  | "span";

// The element a size means when nobody says otherwise. THIS TABLE IS THE TWIN OF
// the one in src/design-system/heading.tsx and has to say the same thing: the
// same heading rendered in the app and in a view is the same heading, and a view
// whose h2 was the app's h3 would read as a different document outline for the
// same content. src/design-system/heading-spelling.test.ts parses both literals
// and fails in either direction on a key one side carries and the other does not.
//
// It is a copy rather than an import because these documents have NO REACT. A
// view is built standalone and inlined whole into a third-party host, so
// importing the component would pull a renderer into a page that has no root to
// render into — and this module's whole shape (createElement, textContent, no
// framework) is what makes that page auditable. The types are duplicated with
// the table so the two halves of one rule sit together rather than one of them
// reaching across the tier boundary.
const HEADING_ELEMENT: Readonly<Record<HeadingSize, HeadingElement>> = {
  xxlarge: "h1",
  xlarge: "h1",
  large: "h2",
  medium: "h3",
  small: "h4",
  xsmall: "h5",
  xxsmall: "h6",
};

/**
 * heading puts a titled line on the page: the element follows the size, `as`
 * overrides it, and the class and `data-size` are what heading.css — imported
 * by view.css — draws the type from. Text, never markup, exactly as el().
 */
export function heading(
  size: HeadingSize,
  text: string | number,
  opts?: { as?: HeadingElement; className?: string },
): HTMLElement {
  const node = document.createElement(opts?.as ?? HEADING_ELEMENT[size]);
  node.className =
    opts?.className === undefined ? "heading" : `heading ${opts.className}`;
  node.dataset.size = size;
  node.textContent = String(text);
  return node;
}

/**
 * percent renders a number a view displays as a proportion. Anything that is not
 * a finite number renders as an em dash rather than as "NaN" or "undefined": a
 * view is looking at data it did not produce, and a missing field is a thing
 * that happens.
 */
export function percent(value: unknown): string {
  const n = asFiniteNumber(value);
  return n === null ? ABSENT : `${Math.round(n * 100)}%`;
}

/** count renders a whole number, or the em dash for a value it cannot read. */
export function count(value: unknown): string {
  const n = asFiniteNumber(value);
  return n === null ? ABSENT : String(n);
}

/**
 * money renders an amount the way the product renders one: integer MINOR units
 * scaled by the currency's own minor-unit count, never by a hard-coded 100.
 *
 * THE SCALE IS format/minorunits', which mirrors the server's ISO 4217 table.
 * JPY stores 1234 minor units and means ¥1,234; a view that divided by 100
 * everywhere would render ¥12.34 for it, and the same class of mistake in the
 * other direction is what made an account brief report every deal a hundred
 * times too large.
 *
 * This used to ask Intl for the digit count, on the stated grounds that it was
 * the rule format.ts applied. Both halves of that stopped being true: format.ts
 * takes the scale from minorunits now, and Intl was never the same table —
 * Intl follows CLDR, which records how a currency is USED, and the server
 * follows ISO, which records what the standard ASSIGNS. They disagree on ten
 * codes, so this view rendered a stored IQD 1234 as "IQD 1,234" where the
 * record means 1.234 dinars, and MGA and IRR a hundredfold out.
 *
 * Importing is safe where importing formatMoney was not: minorUnitDigits takes
 * NO locale — a currency's minor-unit count is a property of the currency —
 * so nothing of the translation machinery follows it into a document that is
 * inlined whole and served to a third-party host. That is the reason the scale
 * lives in its own module rather than inside the formatters.
 *
 * THE LOCALE IS THE HOST'S, DELIBERATELY, and this is the one rendering in the
 * product that does not take the reader's chosen one. What is missing is not
 * the MAPPING — importing `INTL_LOCALE` would cost almost nothing — it is the
 * locale to look it up with: this document is inlined whole into a page the
 * product does not own, with no LocaleProvider above it and no signed-in
 * reader's choice reaching inside it, so there is nothing to index the table
 * by. `undefined` is the honest remaining answer, and it is at least the locale
 * the reader's own browser reports. `format/one-locale.test.ts` carries this
 * file as a named exemption with this reason.
 *
 * An amount that is not a finite number, or a currency Intl does not know,
 * renders as the em dash. Intl throws on an unknown currency code, and a view
 * that threw mid-render would leave the reader a blank panel.
 *
 * SO DOES AN AMOUNT OUTSIDE THE SAFE INTEGER RANGE. The field is an int64 on
 * the wire and a double by the time this sees it, so a value past 2^53 has
 * already been rounded to a number that is not the one that was stored. There
 * is nothing to recover — the digits are gone before this function is called —
 * and the choice is between an em dash and a money figure that is quietly
 * wrong. A reader can act on the first.
 */
export function money(amountMinor: unknown, currency: unknown): string {
  const minor = asFiniteNumber(amountMinor);
  const code = asText(currency);
  if (minor === null || code === "" || !Number.isSafeInteger(minor))
    return ABSENT;
  try {
    const digits = minorUnitDigits(code);
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(toMajorUnits(minor, code));
  } catch {
    return ABSENT;
  }
}

/**
 * day renders an instant as the calendar day it falls on IN UTC, which is the
 * same day the server's own evidence snippets name.
 *
 * UTC and not the reader's zone, because the two would disagree: the answer
 * says a promise is overdue, and a date rendered in a zone the server did not
 * judge in can read as "due tomorrow" beside the word "overdue". One clock,
 * one day, and the state beside it is true of the date shown.
 */
export function day(value: unknown): string {
  const text = asText(value);
  if (text === "") return ABSENT;
  const at = new Date(text);
  if (Number.isNaN(at.getTime())) return ABSENT;
  return at.toISOString().slice(0, 10);
}

/**
 * initBridge attaches the transport and announces this view to its host.
 *
 * It runs on import, which is what makes a view a client the moment its document
 * is loaded: a host may push a result immediately after the frame is created,
 * and a bridge waiting to be started by hand would miss it.
 */
export function initBridge(): void {
  window.addEventListener("message", handle);
  initializeID = announce();
}

initBridge();
