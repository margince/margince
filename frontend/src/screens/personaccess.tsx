import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { throwProblem, useViewerId } from "./common";

type Person = components["schemas"]["Person"];

/**
 * PersonAccess says who this contact is for, and gives its owner the one verb
 * that changes the answer.
 *
 * The question it answers is not "may I edit this" — `writable` already says
 * that, and the edit affordances draw themselves from it. It is "why can I see
 * this at all", which until the server sent `visibility` no surface could
 * answer: a contact private to the reader's own mailbox and one shared with
 * the whole organization looked identical, and the owner of the private one
 * had no way to tell, let alone to change it.
 *
 * Absent `visibility` renders nothing rather than guessing. A panel that
 * assumed `workspace` would tell a reader their private contact is public.
 */
export function PersonAccess({ person }: Readonly<{ person: Person }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  // OWNERSHIP, not writability. `writable` is true for a write grant and for
  // an unbounded seat as well, and neither of those is the mailbox this
  // contact came from — the endpoint matches on owner_id exactly and answers
  // 404 to everybody else. Offering the button on `writable` would show it to
  // a colleague holding a grant and then fail their click.
  const viewerId = useViewerId();
  const isOwner = Boolean(viewerId) && person.owner_id === viewerId;

  const publish = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/people/{id}/publish", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      toast.show(t("personAccess.published"));
      await queryClient.invalidateQueries({
        queryKey: ["person360", person.id],
      });
    },
  });

  if (!person.visibility) {
    return null;
  }
  const isPrivate = person.visibility === "owner";
  return (
    <Panel title={t("personAccess.title")}>
      <PanelBody>
        <p className="t-small">
          {isPrivate
            ? t("personAccess.privateToYou")
            : t("personAccess.organization")}
        </p>
        {isPrivate && isOwner && (
          <Button
            small
            disabled={publish.isPending}
            onClick={() => publish.mutate(person.id)}
          >
            {t("personAccess.share")}
          </Button>
        )}
      </PanelBody>
    </Panel>
  );
}
