// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import { useMe } from "./session";
import { readStored, STORAGE_KEYS, watchStored, writeStored } from "./storage";

// Whether the signed-in account read what Margince reads and writes. Only this
// browser knows: the server has no such fact, so a new browser asks once more.

function seenKey(account: string) {
  return { family: STORAGE_KEYS.setupPermissionsSeen, member: account };
}

export function usePermissionsSeen(): Readonly<{
  seen: boolean;
  markSeen: () => void;
}> {
  const account = useMe().data?.user.id ?? null;
  const seen = useSyncExternalStore(
    (changed) =>
      account === null
        ? () => undefined
        : watchStored(seenKey(account), changed),
    () => account !== null && readStored(seenKey(account)) === "1",
  );
  return {
    seen,
    markSeen: () => {
      if (account !== null) writeStored(seenKey(account), "1");
    },
  };
}
