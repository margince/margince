import type { components } from "../api/schema";

// Answers for the two preview doors, in the shape the server sends.
//
// Every surface that stages a send now asks the engine before anybody presses
// Send, so a harness that answers unknown routes with `{}` hands the component
// a preview with no recipients and the surface under test crashes for a reason
// that is not the test's. Each harness routes the preview doors here instead.

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = components["schemas"]["SendAuthorizationPreviewRecipient"];

/** Whether a request went to one of the two preview doors. */
export function isPreviewDoor(pathname: string): boolean {
  return pathname.endsWith(":preview");
}

/** The addressees a preview request named, read off its body. */
export function previewedAddresses(body: unknown): string[] {
  if (typeof body !== "object" || body === null) {
    return [];
  }
  const to: unknown = Reflect.get(body, "to");
  return Array.isArray(to)
    ? to.filter((entry): entry is string => typeof entry === "string")
    : [];
}

/** The engine allowing every addressee: the answer most sends get. */
export function allowedPreview(addresses: readonly string[]): Preview {
  return {
    allowed: true,
    recipients: addresses.map((address) => ({
      address,
      verdict: "allow",
      reason_code: "allowed",
      decided_by: "machine",
      would_refuse: false,
      can_be_overruled: false,
    })),
  };
}

/** One refused addressee, with the rest of the answer spelled by the caller. */
export function refusedPreview(
  address: string,
  over: Partial<Recipient> & Pick<Recipient, "reason_code" | "decided_by">,
): Preview {
  return {
    allowed: false,
    recipients: [
      {
        address,
        verdict: "deny",
        would_refuse: true,
        can_be_overruled: false,
        ...over,
      },
    ],
  };
}
