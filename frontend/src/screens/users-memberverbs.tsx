import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button, OverflowMenu } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import {
  memberAnchorId,
  memberMutationKey,
  offers,
  type User,
  useMemberBusy,
  useMemberRefresh,
} from "./users-members";
import { PasswordLinkModal, usePasswordLink } from "./users-password-link";

// A member's verbs behind one menu. Nothing to offer draws nothing: a trigger
// over an empty panel is a promise the row cannot keep.
export function MemberVerbs({ member }: Readonly<{ member: User }>) {
  const t = useT();
  const toast = useToast();
  const refresh = useMemberRefresh();
  const pending = useMemberBusy(member.id);
  const [confirmOff, setConfirmOff] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const passwordLink = usePasswordLink();
  const openLink = () => {
    setLinkOpen(true);
    void passwordLink.mint(member.id);
  };

  const reactivate = useMutation({
    mutationKey: memberMutationKey(member.id, "reactivate"),
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/users/{id}/reactivate", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t("users.reactivated", { name: member.email }));
    },
    // Pressed from the menu or from an Undo toast, so the toast is the one
    // place a refusal can land.
    onError: (error) =>
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true }),
  });

  const deactivate = useMutation({
    mutationKey: memberMutationKey(member.id, "deactivate"),
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/users/{id}/deactivate", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // The roster first: closing the dialog hands focus back to the row, which
      // must already read the new status.
      await refresh();
      setConfirmOff(false);
      toast.show(t("users.deactivated", { name: member.email }), {
        action: undoAction(t("common.undo"), () =>
          reactivate.mutate(member.id),
        ),
      });
    },
  });

  const canMintLink = offers(member, "issue_password_link");
  const canDeactivate = offers(member, "deactivate");
  const canReactivate = offers(member, "reactivate");
  if (
    !(canMintLink || canDeactivate || canReactivate || linkOpen || confirmOff)
  ) {
    return null;
  }
  return (
    <>
      <OverflowMenu
        label={t("users.rowActions", { name: member.display_name })}
      >
        {canMintLink && (
          <Button disabled={pending} onClick={openLink}>
            {t("users.link.action")}
          </Button>
        )}
        {canDeactivate && (
          <Button disabled={pending} onClick={() => setConfirmOff(true)}>
            {t("users.deactivate")}
          </Button>
        )}
        {canReactivate && (
          <Button
            disabled={pending}
            onClick={() => reactivate.mutate(member.id)}
          >
            {t("users.reactivate")}
          </Button>
        )}
      </OverflowMenu>
      <ConfirmModal
        open={confirmOff}
        onClose={() => setConfirmOff(false)}
        title={t("users.deactivateConfirmTitle", { name: member.display_name })}
        confirmLabel={t("users.deactivate")}
        confirmVariant="danger"
        pending={deactivate.isPending}
        error={deactivate.error ? problemMessageOf(deactivate.error, t) : null}
        onConfirm={() => deactivate.mutate(member.id)}
        // The Deactivate item is gone once it worked; the member's own cell stays.
        returnFocusTo={() => document.getElementById(memberAnchorId(member.id))}
      >
        {/* Scheduled extension jobs act as themselves, so the agent body says
            what does not stop. */}
        <p>
          {t(
            member.is_agent
              ? "users.deactivateAgentConfirmBody"
              : "users.deactivateConfirmBody",
          )}
        </p>
      </ConfirmModal>
      {linkOpen && (
        <PasswordLinkModal
          memberName={member.display_name}
          link={passwordLink.state.link}
          pending={passwordLink.state.pending}
          error={passwordLink.state.error}
          onRetry={openLink}
          onClose={() => {
            // Drop the credential with the dialog, never merely hide it.
            passwordLink.clear();
            setLinkOpen(false);
          }}
        />
      )}
    </>
  );
}
