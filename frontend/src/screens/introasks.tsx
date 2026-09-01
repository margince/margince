// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The asks in flight about this contact, and who owns the next move.
//
// Both parties read this card and it says different things to each: the
// requester learns they are waiting, the colleague learns they are being
// waited ON. That is one payload and one component, because a second one
// would eventually disagree with this one about what the ask's state means.

import { useState } from "react";
import { Badge, Button, Card } from "../design-system/atoms";
import { useT } from "../i18n";
import { useViewerId } from "./common";
import { IntroDecisionDrawer } from "./introdecision";
import { type IntroRequest, useIntroRequests } from "./introrequests";

/**
 * IntroAsksCard lists the open and settled asks the viewer is party to.
 *
 * An empty list renders nothing at all rather than an empty card: a contact
 * nobody has asked about is the ordinary case, and a card saying so on every
 * such page is noise on most of them.
 */
export function IntroAsksCard({
  personId,
  personName,
}: Readonly<{ personId: string; personName: string }>) {
  const t = useT();
  // Which side of the ask the reader is on. Undefined while /me is in flight,
  // and the card then shows state without offering an answer — never the
  // reverse, which would put the decision in front of the wrong person.
  const viewerUserId = useViewerId();
  const asks = useIntroRequests(personId);
  const [deciding, setDeciding] = useState<IntroRequest | undefined>();

  const rows = asks.data ?? [];
  if (rows.length === 0) {
    return null;
  }

  return (
    <Card title={t("person.intro.asksTitle")} sub={t("person.intro.asksSub")}>
      <ul className="pn-asks">
        {rows.map((ask) => {
          // Only the colleague being asked may answer, and only while the ask
          // is still open. The requester sees the same row without the button:
          // a control that answers 403 is a control that exists to fail.
          const mine =
            viewerUserId !== undefined &&
            ask.introducer_user_id === viewerUserId;
          return (
            <li className="pn-ask" key={ask.id}>
              <Badge quiet>{t(STATUS_LABEL[ask.status])}</Badge>{" "}
              {ask.internal_reason}
              {mine && ask.status === "requested" ? (
                <Button onClick={() => setDeciding(ask)}>
                  {t("person.intro.answerAction")}
                </Button>
              ) : null}
            </li>
          );
        })}
      </ul>

      {deciding ? (
        <IntroDecisionDrawer
          personId={personId}
          personName={personName}
          request={deciding}
          open
          onClose={() => setDeciding(undefined)}
        />
      ) : null}
    </Card>
  );
}

// Every status the contract admits has words here, so a state the server can
// send cannot reach a reader as a raw enum. A name-drop and an introduction
// read differently on purpose: they are different events.
const STATUS_LABEL: Record<
  IntroRequest["status"],
  Parameters<ReturnType<typeof useT>>[0]
> = {
  requested: "person.intro.stateRequested",
  accepted: "person.intro.stateAccepted",
  name_drop_approved: "person.intro.stateNameDropApproved",
  suggest_other: "person.intro.stateSuggestOther",
  declined: "person.intro.stateDeclined",
  introduced: "person.intro.stateIntroduced",
  name_dropped: "person.intro.stateNameDropped",
  replied: "person.intro.stateReplied",
  expired: "person.intro.stateExpired",
  cancelled: "person.intro.stateCancelled",
};
