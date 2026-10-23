import { useQueryClient } from "@tanstack/react-query";
import { CheckSquare } from "lucide-react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useClaimSettle, useTaskUpdate } from "../../components/taskactions";
import { useCompleteTask } from "../../components/taskcomplete";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useSetAside } from "../records/record360";

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
  const completeTask = useCompleteTask(task);
  const claim = useClaimSettle(refresh);
  const notYet = useMomentSetAside(contactId);
  const done = moment.recommended_action;
  const blocked = done.state === "blocked";
  const pending =
    task.isPending || claim.isPending || notYet.deferOf(moment).pending;
  const failure = task.error ?? claim.error ?? notYet.failure;
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
    completeTask(question.promise_id, version);
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
          onClick={notYet.deferOf(moment).onDefer}
        >
          {t("contact.mayBeDone.notYet")}
        </Button>
      </span>
      {failure && <ErrorLine error={failure} inline />}
    </>
  );
}

// Putting one moment away for this reader while its evidence stands, and the
// Undo that shows it again. The contact page's "Not now" and this card's "Not
// yet" are one dismissal, so both go through here.
export function useMomentSetAside(contactId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  return useSetAside<ContactMoment>({
    setAside: async (moment) => {
      unwrap(
        await api.POST("/contacts/{id}/moment/dismiss", {
          params: { path: { id: contactId } },
          body: {
            claim_key: moment.claim_key,
            evidence_fingerprint: moment.evidence_fingerprint,
          },
        }),
        t,
      );
    },
    restore: async (moment) => {
      unwrap(
        await api.DELETE("/contacts/{id}/moment/dismiss", {
          params: {
            path: { id: contactId },
            query: {
              claim_key: moment.claim_key,
              evidence_fingerprint: moment.evidence_fingerprint,
            },
          },
        }),
        t,
      );
    },
    refresh: () =>
      queryClient.invalidateQueries({ queryKey: ["contact360", contactId] }),
    idOf: (moment) => moment.claim_key,
  });
}
