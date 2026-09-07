import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { VisibilityLine } from "../design-system/visibility";
import { useT } from "../i18n";
import { throwProblem } from "./common";

type Person = components["schemas"]["Person"];

/**
 * PersonAccess says who this contact is for, and gives anybody who may edit it
 * the verb that changes the answer.
 *
 * The question it answers is not "may I edit this" — `writable` already says
 * that, and the edit affordances draw themselves from it. It is "why can I see
 * this at all", which until the server sent `visibility` no surface could
 * answer: a contact private to the reader's own mailbox and one shared with
 * the whole organization looked identical, and the owner of the private one
 * had no way to tell, let alone to change it.
 *
 * BOTH DIRECTIONS, and gated on `writable` rather than on ownership. The panel
 * used to offer publishing alone, to the owner alone, because visibility moved
 * one way only. It moves both ways now: the sender classifier publishes a
 * contact it judges a real counterparty with nobody approving it, so the
 * common case was a machine making a decision no human could undo — the row's
 * own owner included.
 *
 * Absent `visibility` renders nothing rather than guessing. A panel that
 * assumed `workspace` would tell a reader their private contact is public.
 */
export function PersonAccess({ person }: Readonly<{ person: Person }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();

  const setVisibility = useMutation({
    mutationFn: async (visibility: "workspace" | "owner") => {
      const { error } = await api.PATCH("/people/{id}", {
        params: {
          path: { id: person.id },
          // The row this panel drew, pinned. Unpinned is last-write-wins, and
          // the column this write moves is the one that decides who may read
          // the record — a save built on a stale render would silently undo
          // somebody else's answer to that question.
          ...ifMatch(requireVersion(person.version)),
        },
        body: { visibility },
      });
      if (error) {
        throwProblem(error);
      }
      return visibility;
    },
    onSuccess: async (visibility) => {
      toast.show(
        visibility === "workspace"
          ? t("personAccess.published")
          : t("personAccess.madePrivate"),
      );
      await queryClient.invalidateQueries({
        queryKey: ["person360", person.id],
      });
    },
  });

  if (!person.visibility) {
    return null;
  }
  const isPrivate = person.visibility === "owner";
  // The one gate, and the same one the rest of the record draws its edit
  // affordances from. Offering the verb on anything narrower would hide it
  // from a colleague the server would have let through.
  const mayChange = Boolean(person.writable);
  return (
    <Panel title={t("personAccess.title")}>
      <PanelBody>
        {/* The same mark a mail row and the drawer draw, with the one verb
            beside it: a contact private to its owner and a message limited
            to its participants are the same fact about two things, and a
            reader who has learned the mark on one should read it on the
            other. */}
        <VisibilityLine
          state={isPrivate ? "private" : "team"}
          action={
            mayChange && (
              <Button
                variant="link"
                pending={setVisibility.isPending}
                onClick={() =>
                  setVisibility.mutate(isPrivate ? "workspace" : "owner")
                }
              >
                {isPrivate
                  ? t("personAccess.share")
                  : t("personAccess.makePrivate")}
              </Button>
            )
          }
        />
        <p className="t-caption">
          {isPrivate
            ? t("personAccess.privateToYou")
            : t("personAccess.organization")}
        </p>
      </PanelBody>
    </Panel>
  );
}
