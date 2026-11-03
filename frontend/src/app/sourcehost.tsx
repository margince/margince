// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useCallback, useState } from "react";
import { OpenEmailDrawer } from "../features/mail/openemaildrawer";
import { OpenActivityDrawer } from "../features/voice-knowledge/activitydrawer";
import { viewerZone } from "../format/timezone";
import { useActivity } from "../lib/activityread";
import { OpenSourceProvider } from "../lib/opensource";

/** The shell's drawer for a source opened anywhere under it. */
export function SourceHost({ children }: Readonly<{ children: ReactNode }>) {
  const [opened, setOpened] = useState<string | null>(null);
  const openSource = useCallback((activityId: string) => {
    setOpened(activityId);
  }, []);
  return (
    <OpenSourceProvider openSource={openSource}>
      {children}
      {/* Mounted only while open, so a closed drawer keeps no read that
          refetches on every window focus. */}
      {opened !== null && (
        <SourceDrawer activityId={opened} onClose={() => setOpened(null)} />
      )}
    </OpenSourceProvider>
  );
}

// The kind comes from the activity read, which also decides what this reader
// may see. A failed read goes to the activity drawer, which says so. The
// worklist's row drawer splits the same way and keeps its own. It also draws
// what the row asks, and refreshes the queue when a reply is sent.
function SourceDrawer({
  activityId,
  onClose,
}: Readonly<{ activityId: string; onClose: () => void }>) {
  const read = useActivity(activityId);
  const answered = read.data !== undefined || read.isError;
  const email = read.data?.kind === "email";
  const zone = viewerZone();
  return (
    <>
      <OpenEmailDrawer
        activityId={answered && email ? activityId : null}
        zone={zone}
        onClose={onClose}
        activity={read.data}
      />
      <OpenActivityDrawer
        activityId={answered && !email ? activityId : null}
        zone={zone}
        onClose={onClose}
        activity={read.data}
      />
    </>
  );
}
