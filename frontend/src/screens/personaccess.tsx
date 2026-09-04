import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { throwProblem } from "./common";

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
  const mayWrite = useCanWriteRecord("person", person);

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
        {/* The verb belongs to the OWNER, and `writable` is how the server
            already tells us who that is on a capture-private row: nobody else
            can write one. Offering it to a reader who cannot would produce a
            404, which is the right answer to the wrong question being asked. */}
        {isPrivate && mayWrite && (
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
