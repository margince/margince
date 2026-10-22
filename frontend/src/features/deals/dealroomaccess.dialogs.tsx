import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import type {
  Capability,
  DealRoom,
  Issued,
  Participant,
} from "./dealroomaccess";
import { IssuedNotice } from "./dealroomaccess.notices";
import { refreshSeats } from "./dealroomaccess.queries";
import {
  CAPABILITIES,
  CAPABILITY_HINTS,
  CAPABILITY_LABELS,
} from "./dealroomcapability";

/** The link a buyer opens: the public screen with the credential in the fragment. */
export function buyerLink(credential: string): string {
  return `${window.location.origin}/#/room?c=${encodeURIComponent(credential)}`;
}

// The link, shown once. Copy is the verb; the sentence under it is the thing
// a rep most needs to know and will otherwise learn from a locked-out buyer.
function IssuedLink({ issued }: Readonly<{ issued: Issued }>) {
  const t = useT();
  const link = buyerLink(issued.credential);
  const copy = useClipboardCopy(link, {
    copy: t("access.issued.copy"),
    copied: t("access.issued.copied"),
    remedy: t("access.issued.copyFailed"),
  });
  return (
    <div className="form-stack">
      <IssuedNotice queued={issued.queued} email={issued.participant.email} />
      <Field label={t("access.issued.linkLabel")}>
        {(control) => <TextInput {...control} readOnly value={link} />}
      </Field>
      <div className="card-actions">
        <Button onClick={copy.copy}>
          <Copy aria-hidden />
          {copy.label}
        </Button>
      </div>
      {/* Below the row, not in it: beside its button the notice loses the
          line it needs to say what to do instead. */}
      {copy.notice}
      <p className="t-caption">{t("access.issued.oneTime")}</p>
    </div>
  );
}

export function InviteDialog({
  room,
  open,
  onClose,
}: Readonly<{ room: DealRoom; open: boolean; onClose: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [capability, setCapability] = useState<Capability>("comment");
  const [issued, setIssued] = useState<Issued | null>(null);
  const invite = useMutation({
    mutationFn: async (input: {
      name: string;
      email: string;
      capability: Capability;
    }) => {
      return unwrap(
        await api.POST("/deal-rooms/{id}/participants", {
          params: { path: { id: room.id } },
          body: {
            full_name: input.name,
            email: input.email,
            capability: input.capability,
            source: "manual",
          },
        }),
        t,
      );
    },
    onSuccess: (data) => {
      void refreshSeats(queryClient, room.id);
      if (data) {
        setIssued(data);
      }
    },
  });
  const close = () => {
    setIssued(null);
    setName("");
    setEmail("");
    setCapability("comment");
    invite.reset();
    onClose();
  };
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={
        issued
          ? t("access.issued.title", { name: issued.participant.full_name })
          : t("access.inviteTitle")
      }
      confirmLabel={issued ? t("access.done") : t("access.inviteConfirm")}
      confirmDisabled={!issued && (name.trim() === "" || email.trim() === "")}
      pending={invite.isPending}
      error={invite.isError ? problemMessageOf(invite.error, t) : null}
      onConfirm={() =>
        issued
          ? close()
          : invite.mutate({
              name: name.trim(),
              email: email.trim(),
              capability,
            })
      }
      intent="form"
    >
      {issued ? (
        <IssuedLink issued={issued} />
      ) : (
        <>
          <Field label={t("access.nameLabel")} required>
            {(control) => (
              <TextInput
                {...control}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            )}
          </Field>
          <Field label={t("access.emailLabel")} required>
            {(control) => (
              <TextInput
                {...control}
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            )}
          </Field>
          <ChoiceList
            legend={t("access.capabilityLegend")}
            value={capability}
            onChange={setCapability}
            choices={CAPABILITIES.map((c) => ({
              value: c,
              label: t(CAPABILITY_LABELS[c]),
              description: t(CAPABILITY_HINTS[c]),
            }))}
          />
          <p>{t("access.inviteNote")}</p>
        </>
      )}
    </ConfirmModal>
  );
}

export function ReissueDialog({
  room,
  participant,
  open,
  onClose,
}: Readonly<{
  room: DealRoom;
  participant: Participant;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [issued, setIssued] = useState<Issued | null>(null);
  const reissue = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/deal-rooms/{id}/participants/{participantId}/resend", {
          params: { path: { id: room.id, participantId: participant.id } },
        }),
        t,
      );
    },
    onSuccess: (data) => {
      void refreshSeats(queryClient, room.id);
      if (data) {
        setIssued(data);
      }
    },
  });
  const close = () => {
    setIssued(null);
    reissue.reset();
    onClose();
  };
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t("access.issueLinkTitle", { name: participant.full_name })}
      confirmLabel={issued ? t("access.done") : t("access.issueLink")}
      pending={reissue.isPending}
      error={reissue.isError ? problemMessageOf(reissue.error, t) : null}
      onConfirm={() => (issued ? close() : reissue.mutate())}
    >
      {issued ? (
        <IssuedLink issued={issued} />
      ) : (
        <p>{t("access.issueLinkBody")}</p>
      )}
    </ConfirmModal>
  );
}

export function RevokeDialog({
  room,
  participant,
  open,
  onClose,
}: Readonly<{
  room: DealRoom;
  participant: Participant;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.POST("/deal-rooms/{id}/participants/{participantId}/revoke", {
          params: { path: { id: room.id, participantId: participant.id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      void refreshSeats(queryClient, room.id);
      onClose();
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("access.revokeTitle", { name: participant.full_name })}
      confirmLabel={t("access.revoke")}
      confirmVariant="danger"
      pending={revoke.isPending}
      error={revoke.isError ? problemMessageOf(revoke.error, t) : null}
      onConfirm={() => revoke.mutate()}
    >
      <p>
        {participant.email}
        {participant.last_seen_at
          ? ` · ${t("access.lastSeen", { when: formatDateAbbrev(participant.last_seen_at, locale, recordZone) })}`
          : ` · ${t("access.neverSignedIn")}`}
      </p>
      <p>{t("access.revokeBody")}</p>
    </ConfirmModal>
  );
}

export function CapabilityDialog({
  room,
  participant,
  open,
  onClose,
}: Readonly<{
  room: DealRoom;
  participant: Participant;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [capability, setCapability] = useState<Capability>(
    participant.capability,
  );
  const change = useMutation({
    mutationFn: async (next: Capability) => {
      unwrap(
        await api.PATCH("/deal-rooms/{id}/participants/{participantId}", {
          params: { path: { id: room.id, participantId: participant.id } },
          body: { capability: next },
        }),
        t,
      );
    },
    onSuccess: () => {
      void refreshSeats(queryClient, room.id);
      onClose();
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("access.changeCapabilityTitle", { name: participant.full_name })}
      intent="form"
      confirmLabel={t("access.save")}
      confirmDisabled={capability === participant.capability}
      pending={change.isPending}
      error={change.isError ? problemMessageOf(change.error, t) : null}
      onConfirm={() => change.mutate(capability)}
    >
      <ChoiceList
        legend={t("access.capabilityLegend")}
        value={capability}
        onChange={setCapability}
        choices={CAPABILITIES.map((c) => ({
          value: c,
          label: t(CAPABILITY_LABELS[c]),
          description: t(CAPABILITY_HINTS[c]),
        }))}
      />
    </ConfirmModal>
  );
}
