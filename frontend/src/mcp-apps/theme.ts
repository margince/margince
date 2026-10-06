// How a view follows the way round its host says it is drawn. Split from the
// transport so the bridge stays under the file-length ceiling.

import { asRecord, asText } from "./types";

/**
 * applyTheme follows the way round the host says it is drawn. Following it is
 * the whole reason a view looks embedded rather than pasted in.
 *
 * A HOST THAT STATES NOTHING IS NOT A HOST THAT IS LIGHT — and it is not this
 * module's question either. `hostContext.theme` is optional (SEP-1865), and the
 * unstated case is answered in the canon: tokens.css carries a
 * `prefers-color-scheme: dark` arm for any document that has not been stamped
 * light, so an unstamped view inside a dark host renders dark, and follows a
 * reader who changes their system appearance mid-session for free.
 *
 * So this stamps ONLY what the host stated. Resolving the platform preference
 * here instead would put the answer to "what does dark look like when nobody
 * said" in a TypeScript module the next non-SPA surface cannot find, and would
 * freeze it at whatever the preference was when the panel opened.
 */
export function applyTheme(hostContext: unknown): void {
  const stated = asText(asRecord(hostContext).theme);
  if (stated !== "") {
    stateTheme(stated);
  }
}

/**
 * followHostChange applies a host-context change notification.
 *
 * ITS PARAMS ARE THE CONTEXT ITSELF, not a `hostContext` member — unlike the
 * initialize RESULT, which nests one. Reading it the same way as the result is
 * the mistake this function exists to not make: the theme then never resolves,
 * and every notification looks like a host that stated nothing.
 *
 * AND A PARTIAL UPDATE IS PARTIAL. The host sends one of these whenever
 * anything about the frame changes — a resize notification carrying only
 * `containerDimensions` arrives right after every open — so "no theme stated"
 * here means "not mentioned", NOT "delegated to the stylesheet". Treating the
 * two the same is worse than ignoring the notification altogether: it undoes
 * the theme the handshake correctly resolved, moments after it resolved it.
 */
export function followHostChange(context: unknown): void {
  const stated = asText(asRecord(context).theme);
  if (stated === "") return;
  stateTheme(stated);
}

/**
 * stateTheme applies a theme the host has DECIDED.
 *
 * The attribute is how a decision beats the platform in BOTH directions: the
 * canon's media arm excludes `[data-theme="light"]`, so a host that states light
 * on a dark platform is honoured, and a host that states dark on a light one has
 * no media query to wait for.
 */
function stateTheme(theme: string): void {
  document.documentElement.dataset.theme = theme;
}
