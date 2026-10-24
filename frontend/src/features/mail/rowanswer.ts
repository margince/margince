import type { components } from "../../api/schema";

type Kind = components["schemas"]["Activity"]["kind"];

// The row's verb and the dialog's framing both read this, so a call row cannot open
// a dialog that calls the email a reply. An unnamed kind is a mail anchor not yet read.
export function rowAnswer(kind: Kind | undefined): "reply" | "about" | null {
  switch (kind) {
    case undefined:
    case "email":
    case "message":
      return "reply";
    case "call":
    case "note":
    case "meeting":
      return "about";
    case "task":
      return null;
  }
}
