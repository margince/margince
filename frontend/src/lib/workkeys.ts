import type { QueryClient, QueryKey } from "@tanstack/react-query";

export const worklistKey = ["worklist"] as const;
export const magicKey = ["magic"] as const;
export const briefKey = ["brief"] as const;
export const digestKey = ["digest"] as const;
// The reader's own set-aside or stop on a quiet contact. The Worklist writes
// it and the contact page reads it, so both features key on this one prefix.
export const nudgeDismissalKey = ["nudge-dismissal"] as const;
// Every cached contact duplicate case. A case is read off its pairs, so a pair
// decision anywhere makes all of them stale.
export const contactCasesKey = ["contact-case"] as const;

export async function invalidateHandledChange(
  client: QueryClient,
): Promise<void> {
  await Promise.all(
    [worklistKey, magicKey, briefKey, ["deals"]].map((queryKey) =>
      client.invalidateQueries({ queryKey }),
    ),
  );
}

export function isLiveWorkRead(key: QueryKey): boolean {
  return (
    (key[0] === worklistKey[0] &&
      (key[1] === "handled" ||
        (key[1] === "mine" && key[2] === "all" && key[3] === ""))) ||
    (key[0] === "approvals" && key[1] === "pending") ||
    (key.length === 1 && (key[0] === magicKey[0] || key[0] === briefKey[0]))
  );
}

// Home's day is read when it opens, again once its answer is this old, and at
// once after the reader acts on it (invalidateHandledChange). Each read
// assembles the reader's whole day on the server; Home is not a live board.
export const HOME_DAY_FRESH_MS = 5 * 60_000;

// Home's day: the worklist it opens on, the brief and the receipt. Each costs
// the server a whole assembly, so it is read on change, not on a cadence.
export function isHomeDayRead(key: QueryKey): boolean {
  return (
    (key[0] === worklistKey[0] &&
      key[1] === "mine" &&
      key[2] === "all" &&
      key[3] === "") ||
    (key.length === 1 && (key[0] === magicKey[0] || key[0] === briefKey[0]))
  );
}
