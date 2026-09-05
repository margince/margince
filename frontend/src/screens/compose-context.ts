/**
 * What kind of communication the composer is about to send, worked out from
 * what it already knows.
 *
 * Its own file beside compose.tsx because it is the one piece of this screen
 * that is a RULE rather than a rendering: the same question the server asks,
 * answered here only to decide what the reader is shown. The server remains the
 * authority and re-asks at staging.
 *
 * The rule it mirrors: a reply to a message the subject sent us is a reply,
 * whether or not anybody says so. The engine derives that from the anchor, so
 * the honest thing for a reply is to CLAIM NOTHING and let it — a claim the
 * evidence does not carry is recorded as a claim the evidence does not carry,
 * which is worse than silence. What is left to ask about is a message with no
 * anchor to derive from, which is the only case a human has to answer.
 */

import type { components } from "../api/schema";

export type CommunicationContext =
  components["schemas"]["CommunicationContext"];

/** The anchor a composed message is answering, as far as the composer knows. */
export interface Anchor {
  kind?: string;
  direction?: string | null;
}

/**
 * Whether the message being composed continues something the subject started.
 *
 * INBOUND is the whole test. An anchor we sent ourselves is not the subject
 * writing to us, so replying to our own outbound message on a thread they never
 * answered is an unprompted follow-up wearing a reply's clothes — and it is the
 * case a rep reaches by re-opening their own sent mail.
 */
export function repliesToTheSubject(anchor: Anchor | undefined): boolean {
  if (!anchor) {
    return false;
  }
  const correspondence = anchor.kind === "email" || anchor.kind === "message";
  return correspondence && anchor.direction === "inbound";
}

/**
 * What the composer should say this message is, or nothing.
 *
 * Three answers, and the difference between the first two matters more than it
 * looks:
 *
 *  - `undefined` — say nothing, because the anchor already says it. The engine
 *    resolves `reply_to_inbound` from the thread, and a claim added on top
 *    could only agree with it or contradict it.
 *  - a category — the reader answered the question, because there was no anchor
 *    to answer it for them.
 *  - `undefined` with `asks` true — nobody has answered yet.
 */
export function contextFor(args: {
  anchor: Anchor | undefined;
  chosen: CommunicationContext | "";
}): CommunicationContext | undefined {
  if (repliesToTheSubject(args.anchor)) {
    return undefined;
  }
  return args.chosen === "" ? undefined : args.chosen;
}

/**
 * Whether the composer has to ask the reader why they are writing.
 *
 * Only where nothing else answers it. A reply derives, so asking would be
 * asking a reader to restate what the thread in front of them already says —
 * and a question with an obvious answer trains people to answer without
 * reading, which is how the purpose dropdown this replaces came to be set to
 * whatever was first in the list.
 */
export function asksWhy(anchor: Anchor | undefined): boolean {
  return !repliesToTheSubject(anchor);
}
