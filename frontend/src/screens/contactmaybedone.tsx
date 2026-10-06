import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CheckSquare } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { useClaimSettle, useTaskUpdate } from "./taskactions";

// The verbs of a promise card that asks whether our last email kept it. The
// server never closes the promise itself: Done is the reader's word for it,
// and Not yet puts the question away until we write to them again.

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type MayBeDone = components["schemas"]["ContactMomentMayBeDone"];

export function MayBeDoneVerbs({
  moment,
  question,
  view,
}: Readonly<{
  moment: ContactMoment;
  question: MayBeDone;
  view: Contact360;
}>) {
  const t = useT();
  const contactId = view.contact.id;
  const refresh = [["contact360", contactId]] as const;
  const task = useTaskUpdate(refresh);
  const claim = useClaimSettle(refresh);
  const notYet = useMomentDismiss();
  const done = moment.recommended_action;
  const blocked = done.state === "blocked";
  const pending = task.isPending || claim.isPending || notYet.isPending;
  const failure = task.error ?? claim.error ?? notYet.error;
  const complete = () => {
    if (question.promise_type === "claim") {
      claim.mutate({ id: question.promise_id, outcome: "done" });
      return;
    }
    // The version the card was drawn against, so a task a colleague changed
    // meanwhile is refused rather than completed over their edit.
    const version = view.next_steps?.data.find(
      (row) => row.id === question.promise_id,
    )?.version;
    task.mutate({ id: question.promise_id, version, body: { is_done: true } });
  };
  return (
    <>
      <span className="today-verb">
        <Button
          variant="ai"
          onClick={complete}
          disabled={pending}
          reason={
            blocked
              ? (done.blocked_reason ?? t("contact.rail.blocked"))
              : undefined
          }
        >
          <CheckSquare aria-hidden="true" />
          {done.label}
        </Button>
      </span>
      <span className="today-verb">
        <Button
          variant="ghost"
          disabled={pending}
          onClick={() =>
            notYet.mutate({
              contactId,
              body: {
                claim_key: moment.claim_key,
                evidence_fingerprint: moment.evidence_fingerprint,
              },
            })
          }
        >
          {t("contact.mayBeDone.notYet")}
        </Button>
      </span>
      {failure && <ErrorLine error={failure} inline />}
    </>
  );
}

// Putting one moment away for this reader while its evidence stands. The
// contact rides with the press, so a page that moved on between the press and
// the answer still refreshes the contact the dismissal was for.
function useMomentDismiss() {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      contactId,
      body,
    }: {
      contactId: string;
      body: components["schemas"]["DismissContactMomentRequest"];
    }) => {
      const { error } = await api.POST("/contacts/{id}/moment/dismiss", {
        params: { path: { id: contactId } },
        body,
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: (_data, { contactId }) =>
      queryClient.invalidateQueries({ queryKey: ["contact360", contactId] }),
  });
}
