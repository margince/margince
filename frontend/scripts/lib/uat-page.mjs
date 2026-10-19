// How fe-uat.mjs drives one story page and reads what it drew: the viewport a
// story's tags ask for, and what the page reports once it rendered.

import { FAILURE_EVENTS } from "./story-outcome.mjs";

// The tags fe-uat honours.
//
// EXPECTED_ERROR_TAG: see the console listener in fe-uat.mjs.
//
// PHONE_TAG exists because of a mechanism that made every narrow story a lie.
// `globals: { viewport: … }` is applied by the Storybook MANAGER, which styles
// the preview iframe from outside it — and this harness loads `iframe.html`
// DIRECTLY, so the manager never runs and the story renders at whatever width
// the browser was given. Every story named `…Phone` was captured at 1024px,
// including the shell's, which had been claiming to show the phone layout since
// it was written. A story about a 390px layout has to be driven at 390px, so the
// tag moves the browser instead of asking the manager to.
export const EXPECTED_ERROR_TAG = "uat-expected-console-error";
const PHONE_TAG = "uat-phone";
// The same phone on its side, for a layout keyed on height running out.
const PHONE_ON_ITS_SIDE_TAG = "uat-phone-on-its-side";
// A wide desktop, for a layout that only exists above the top bar's 1100px.
const WIDE_TAG = "uat-wide";
const DESKTOP = { width: 1024, height: 720 };
const PHONE = { width: 390, height: 844 };
const PHONE_ON_ITS_SIDE = { width: 844, height: 390 };
const WIDE = { width: 1440, height: 900 };

export function viewportFor(tags) {
  if (tags.includes(PHONE_TAG)) return PHONE;
  if (tags.includes(PHONE_ON_ITS_SIDE_TAG)) return PHONE_ON_ITS_SIDE;
  if (tags.includes(WIDE_TAG)) return WIDE;
  return DESKTOP;
}

// What counts as painted. The canvas is the usual answer, but `Modal`
// (src/design-system/modal.tsx) returns `createPortal(…, document.body)` — a
// dialog opened from inside a collapsed container would otherwise be hidden
// along with it — so a drawer story renders FULLY while leaving
// `#storybook-root` empty, and reading the canvas alone called four such
// stories blank.
//
// The dialog ROLES and nothing wider: an overflow menu and a toast portal to
// the body too, and either one would let a story that never drew its subject
// pass here as rendered.
export const PAINTED =
  '#storybook-root > *, [role="dialog"], [role="alertdialog"]';
// Enough of a node to recognise it by, in a manifest read without the browser.
const DUMP_CHARS = 600;

// What the deadline actually saw. A verdict that now spans two places has to
// report both, or the next reader cannot tell an empty canvas from a dialog
// that opened and never became visible.
export async function paintCensus(page) {
  return await page.evaluate((limit) => {
    const clip = (el) => el.outerHTML.slice(0, limit);
    const root = document.querySelector("#storybook-root");
    const dialogs = [
      ...document.querySelectorAll('[role="dialog"], [role="alertdialog"]'),
    ];
    return [
      `#storybook-root: ${root ? clip(root) : "absent"}`,
      `dialogs (${dialogs.length}): ${dialogs.map(clip).join(" | ") || "none"}`,
    ].join("\n");
  }, DUMP_CHARS);
}

export async function storyOutcome(page) {
  return await page.evaluate(
    (events) =>
      Object.fromEntries(
        events.map((event) => [
          event,
          globalThis.__STORYBOOK_ADDONS_CHANNEL__?.last(event) ?? null,
        ]),
      ),
    FAILURE_EVENTS,
  );
}

export async function renderPhase(page) {
  return await page.evaluate(
    () => globalThis.__STORYBOOK_PREVIEW__?.currentRender?.phase ?? "unknown",
  );
}
