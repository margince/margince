// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { useOpenEmail } from "../../lib/openemail";
import { useRecordZone } from "../../lib/recordzone";
import { SEARCH_ACTIVITY_SEGMENT } from "../../lib/searchkinds";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { OpenActivityDrawer } from "./activitydrawer";

/** Opens a hit in the panel for what it is: a message, or any other activity. */
export type OpenHit = (activityId: string, panel: "email" | "activity") => void;

/**
 * Which hit the results page has open, and the drawers it opens in.
 *
 * `openAs` is the address's third segment: the activity panel's marker, or
 * absent for a message, so a link to an email keeps its two segments.
 *
 * The drawers sit at page level rather than inside a group. One per group
 * would put an `aria-modal` element in every group, and the results stay
 * legible behind the one that is open.
 */
export function useSearchPanels(
  openActivityId: string | undefined,
  openAs: string | undefined,
): Readonly<{ onOpen: OpenHit; drawers: ReactNode }> {
  const zone = useRecordZone();
  const routedToActivity = openAs === SEARCH_ACTIVITY_SEGMENT;
  const [openEmail, setOpenEmail] = useOpenEmail(
    routedToActivity ? undefined : openActivityId,
  );
  // An initial value, as useOpenEmail's is: once closed it stays closed.
  const [openActivity, setOpenActivity] = useState<string | null>(
    routedToActivity ? (openActivityId ?? null) : null,
  );
  const onOpen: OpenHit = (id, panel) =>
    panel === "email" ? setOpenEmail(id) : setOpenActivity(id);
  const drawers = (
    <>
      <OpenEmailDrawer
        activityId={openEmail}
        zone={zone}
        onClose={() => setOpenEmail(null)}
      />
      <OpenActivityDrawer
        activityId={openActivity}
        zone={zone}
        onClose={() => setOpenActivity(null)}
      />
    </>
  );
  return { onOpen, drawers };
}
