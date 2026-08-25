// The location check: does an MCP App view, inside THIS host, get to read the
// device's position?
//
// WHY IT IS A VIEW OF ITS OWN AND NOT A LINE ON A PRODUCT CARD. The other five
// views state plainly that they render and nothing else — "a button here would
// be this view inventing authority it was not given". A button that asks the
// browser for a permission is exactly such a thing. Rather than weaken that
// rule on a card a customer sees, the question gets its own surface where a
// control is the entire point.
//
// WHAT IT IS FOR. The deck's business-card scenario ends with the assistant
// noticing you are at a conference and offering the venue as a tag. Every route
// to that fact either asks the user or guesses from an IP address. The device
// knows, and whether a view may ask it is the host's decision — the extension
// says a host "MAY honor these permissions… but are not required to". Nobody
// can read that decision out of a spec. It has to be run, per host: web,
// Android, iOS, desktop.
//
// WHAT ANSWER IT PRODUCES. Not a yes or a no — the browser's own error string.
// "disabled in this document by permissions policy" means the iframe carries no
// allow attribute and nobody was ever prompted. "User denied Geolocation" means
// a person saw a prompt and said no. Those are the same numeric code and they
// mean opposite things: the first is the host refusing, the second is the
// feature working.
//
// THIS IS A PROBE, NOT A PRODUCT SURFACE. It answers one question, and once
// every host in the matrix has answered it, it should be deleted rather than
// left sitting in resources/list forever.

import { el, onResult } from "../bridge";
import { describeEnvironment, type GeoResult, readPosition } from "../geo";
import "../view.css";

/** What each refusal means for the person reading it — the finding in a
 *  sentence, since the raw string is evidence and not an explanation. */
const MEANING: Record<string, string> = {
  "host-blocked":
    "The host did not allow this frame to ask. Nobody was prompted. This is the host's decision, not yours and not a bug here.",
  "user-declined":
    "A prompt was shown and refused. The permission itself got through — try again and accept to confirm.",
  "refused-unclassified":
    "Refused, and this build does not recognise the wording. It could be the host blocking the frame or a person declining — read the message above, and add it to the patterns in geo.ts once you know which.",
  unavailable:
    "This document has no geolocation API at all, which usually means it is not a secure context.",
  timeout:
    "Allowed, and asked, but no fix arrived in time. Not a permission problem.",
  "position-unavailable":
    "Allowed, and asked, but the device could not produce a position. Not a permission problem.",
};

/** rowsOf renders a label/value table without building markup from a string. */
function rowsOf(pairs: ReadonlyArray<readonly [string, string]>): HTMLElement {
  const rows = el("div", "rows");
  for (const [label, value] of pairs) {
    const row = el("div", "row");
    row.appendChild(el("span", "name", label));
    row.appendChild(el("span", "state", value));
    rows.appendChild(row);
  }
  return rows;
}

/** report replaces the outcome panel with what the last read actually said. */
function report(into: HTMLElement, result: GeoResult): void {
  into.replaceChildren();
  if (result.ok) {
    into.appendChild(el("div", "section-title", "Granted"));
    into.appendChild(
      rowsOf([
        ["latitude", String(result.latitude)],
        ["longitude", String(result.longitude)],
        ["accuracy", `${Math.round(result.accuracyM)} m`],
      ]),
    );
    return;
  }
  into.appendChild(el("div", "section-title", `Refused — ${result.refusal}`));
  into.appendChild(
    rowsOf([
      ["code", String(result.code)],
      // Verbatim. This string is the finding.
      ["message", result.message],
    ]),
  );
  into.appendChild(
    el("p", "meta", MEANING[result.refusal] ?? "Unrecognised refusal."),
  );
}

function render(root: HTMLElement): void {
  root.replaceChildren();
  root.appendChild(el("h1", undefined, "Location check"));
  root.appendChild(
    el(
      "p",
      "meta",
      "Does this host let a view read the device's position? Press the button. The message text is the answer.",
    ),
  );

  const outcome = el("div", "section");
  outcome.appendChild(el("div", "empty", "Not asked yet."));

  const button = el("button", "probe-button", "Read my location");
  // A real button element, so it is focusable and works from a keyboard without
  // this view adding a single handler for it.
  button.addEventListener("click", () => {
    outcome.replaceChildren(el("div", "empty", "Asking…"));
    // The promise never rejects, by construction — see geo.ts. No catch here
    // would hide anything, because there is nothing to catch.
    void readPosition().then((result) => report(outcome, result));
  });

  root.appendChild(button);
  root.appendChild(outcome);

  const env = el("div", "section");
  env.appendChild(el("div", "section-title", "Environment"));
  env.appendChild(el("div", "empty", "Reading…"));
  root.appendChild(env);
  // Environment first and without asking: the first question about a refusal is
  // always whether the frame could ever have succeeded, and every field here is
  // available before any prompt.
  void describeEnvironment().then((found) => {
    env.replaceChildren(el("div", "section-title", "Environment"));
    env.appendChild(rowsOf(Object.entries(found)));
  });
}

// This view answers no tool, so it draws on load rather than waiting for a
// result. The handler is still registered: a host that pushes one anyway gets a
// redraw instead of a view that silently ignores it.
onResult(() => {
  const root = document.getElementById("root");
  if (root !== null) render(root);
});

const initial = document.getElementById("root");
if (initial !== null) render(initial);
