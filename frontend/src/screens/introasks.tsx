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
import {
  type IntroRequest,
  useCancelIntroRequest,
  useCompleteIntroRequest,
  useIntroRequests,
} from "./introrequests";

type CompleteMutation = ReturnType<typeof useCompleteIntroRequest>;
type CancelMutation = ReturnType<typeof useCancelIntroRequest>;

// The statuses an ask can still be withdrawn from — the same set the server's
// lifecycle table allows a `cancelled` move from. Kept here rather than
// derived, because the two sides agreeing without sharing code is exactly the
// kind of drift this file exists to avoid noticing too late.
const WITHDRAWABLE = new Set<IntroRequest["status"]>([
  "requested",
  "accepted",
  "name_drop_approved",
]);

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
  // Which row's outcome/withdraw button is in flight, so a failure badge
  // lands beside the row that failed rather than every row in the list —
  // the two mutations below are shared across the whole card, not per-row.
  const [actingId, setActingId] = useState<string | undefined>();
  const complete = useCompleteIntroRequest(personId);
  const cancel = useCancelIntroRequest(personId);

  const rows = asks.data ?? [];
  if (rows.length === 0) {
    return null;
  }

  return (
    <Card title={t("person.intro.asksTitle")} sub={t("person.intro.asksSub")}>
      <ul className="pn-asks">
        {rows.map((ask) => (
          <AskRow
            key={ask.id}
            ask={ask}
            viewerUserId={viewerUserId}
            acting={actingId === ask.id}
            complete={complete}
            cancel={cancel}
            onAnswer={() => setDeciding(ask)}
            onActing={() => setActingId(ask.id)}
          />
        ))}
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

// One ask: the state everybody sees, and the verbs only one side of it may
// press. Split out of the list above because the four verbs — answer,
// complete two different ways, withdraw — read as one flat block once they
// are all inline in the same map.
function AskRow({
  ask,
  viewerUserId,
  acting,
  complete,
  cancel,
  onAnswer,
  onActing,
}: Readonly<{
  ask: IntroRequest;
  viewerUserId: string | undefined;
  acting: boolean;
  complete: CompleteMutation;
  cancel: CancelMutation;
  onAnswer: () => void;
  onActing: () => void;
}>) {
  const t = useT();
  // Only the colleague being asked may answer, and only while the ask is
  // still open. The requester sees the same row without the button: a
  // control that answers 403 is a control that exists to fail.
  const mine =
    viewerUserId !== undefined && ask.introducer_user_id === viewerUserId;
  const isRequester =
    viewerUserId !== undefined && ask.requester_user_id === viewerUserId;
  const outcome = outcomeFor(ask.status, mine, isRequester);
  return (
    <li className="pn-ask">
      <Badge quiet>{t(STATUS_LABEL[ask.status])}</Badge> {ask.internal_reason}
      {mine && ask.status === "requested" ? (
        <Button onClick={onAnswer}>{t("person.intro.answerAction")}</Button>
      ) : null}
      {outcome ? (
        <Button
          onClick={() => {
            onActing();
            complete.mutate({ id: ask.id, version: ask.version });
          }}
          disabled={acting && complete.isPending}
        >
          {t(outcome)}
        </Button>
      ) : null}
      {isRequester && WITHDRAWABLE.has(ask.status) ? (
        <Button
          variant="ghost"
          onClick={() => {
            onActing();
            cancel.mutate({ id: ask.id, version: ask.version });
          }}
          disabled={acting && cancel.isPending}
        >
          {t("person.intro.withdrawAction")}
        </Button>
      ) : null}
      {acting ? <OutcomeErrors complete={complete} cancel={cancel} /> : null}
    </li>
  );
}

// Which outcome the row offers, if any — the handshake, which either party
// may report because either may be the one who sees it happen, or the
// name-drop, which is the requester's own act and so only they may report.
// A plain lookup rather than two inline conditions: the two cases share every
// line except the status they gate on, the who-may-press rule and the label.
function outcomeFor(
  status: IntroRequest["status"],
  mine: boolean,
  isRequester: boolean,
):
  | "person.intro.completeIntroducedAction"
  | "person.intro.completeNameDroppedAction"
  | null {
  if (status === "accepted" && (mine || isRequester)) {
    return "person.intro.completeIntroducedAction";
  }
  if (status === "name_drop_approved" && isRequester) {
    return "person.intro.completeNameDroppedAction";
  }
  return null;
}

function OutcomeErrors({
  complete,
  cancel,
}: Readonly<{ complete: CompleteMutation; cancel: CancelMutation }>) {
  const t = useT();
  return (
    <>
      {complete.isError ? (
        <p role="alert">
          <Badge tone="danger">{t("person.intro.completeFailed")}</Badge>
        </p>
      ) : null}
      {cancel.isError ? (
        <p role="alert">
          <Badge tone="danger">{t("person.intro.withdrawFailed")}</Badge>
        </p>
      ) : null}
    </>
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
