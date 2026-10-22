import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { undoAction, useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { HandoverModal, HandoverSummary } from "./users-handover";
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
  const [confirmFactor, setConfirmFactor] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [handoverOpen, setHandoverOpen] = useState(false);
  const passwordLink = usePasswordLink();
  const openLink = () => {
    setLinkOpen(true);
    void passwordLink.mint(member.id);
  };

  const reactivate = useMutation({
    mutationKey: memberMutationKey(member.id, "reactivate"),
    mutationFn: async (id: string) => {
      unwrap(
        await api.POST("/users/{id}/reactivate", {
          params: { path: { id } },
        }),
      );
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
      unwrap(
        await api.POST("/users/{id}/deactivate", {
          params: { path: { id } },
        }),
      );
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

  const removeFactor = useMutation({
    mutationKey: memberMutationKey(member.id, "remove_mfa"),
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/users/{id}/mfa", { params: { path: { id } } }));
    },
    onSuccess: async () => {
      await refresh();
      setConfirmFactor(false);
      toast.show(t("users.mfaRemoved", { name: member.display_name }));
    },
  });

  const canMintLink = offers(member, "issue_password_link");
  const canRemoveFactor = offers(member, "remove_mfa");
  const canDeactivate = offers(member, "deactivate");
  const canReactivate = offers(member, "reactivate");
  // Offered on a deactivated member too: a member who left keeps owning their
  // records until somebody takes them over.
  const canHandOver = offers(member, "hand_over");
  const focusMember = () => document.getElementById(memberAnchorId(member.id));
  if (
    !(
      canMintLink ||
      canRemoveFactor ||
      canDeactivate ||
      canReactivate ||
      canHandOver ||
      linkOpen ||
      confirmOff ||
      confirmFactor
    )
  ) {
    return null;
  }
  return (
    <>
      <OverflowMenu
        label={t("table.rowActions", { name: member.display_name })}
      >
        {canMintLink && (
          <Button disabled={pending} onClick={openLink}>
            {t("users.link.action")}
          </Button>
        )}
        {canRemoveFactor && (
          <Button disabled={pending} onClick={() => setConfirmFactor(true)}>
            {t("users.removeMfa")}
          </Button>
        )}
        {canHandOver && (
          <Button disabled={pending} onClick={() => setHandoverOpen(true)}>
            {t("users.handover.action")}
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
        returnFocusTo={focusMember}
        actionsLead={
          canHandOver ? (
            <Button
              onClick={() => {
                setConfirmOff(false);
                setHandoverOpen(true);
              }}
            >
              {t("users.handover.action")}
            </Button>
          ) : undefined
        }
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
        <HandoverSummary member={member} enabled={confirmOff && canHandOver} />
      </ConfirmModal>
      {handoverOpen && (
        <HandoverModal
          member={member}
          open={handoverOpen}
          onClose={() => setHandoverOpen(false)}
          returnFocusTo={focusMember}
        />
      )}
      <ConfirmModal
        open={confirmFactor}
        onClose={() => setConfirmFactor(false)}
        title={t("users.removeMfaConfirmTitle", { name: member.display_name })}
        confirmLabel={t("users.removeMfa")}
        confirmVariant="danger"
        pending={removeFactor.isPending}
        error={
          removeFactor.error ? problemMessageOf(removeFactor.error, t) : null
        }
        onConfirm={() => removeFactor.mutate(member.id)}
        returnFocusTo={() => document.getElementById(memberAnchorId(member.id))}
      >
        <p>{t("users.removeMfaConfirmBody")}</p>
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
