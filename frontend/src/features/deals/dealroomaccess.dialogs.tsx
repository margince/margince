import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { formatDateAbbrev } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { looksLikeEmail } from "../../lib/emailshape";
import {
  fieldCheckSentence,
  problemMessageOf,
  refusalSentences,
  unwrap,
} from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import type {
  Capability,
  DealRoom,
  Issued,
  Participant,
} from "./dealroomaccess";
import { IssuedNotice } from "./dealroomaccess.notices";
import { refreshSeats, watchInviteMail } from "./dealroomaccess.queries";
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

type Invitee = Readonly<{ full_name: string; email: string }>;

// What the form refuses before sending, by wire field, in the server's words.
function inviteeChecks(
  invitee: Invitee,
  labels: Invitee,
  locale: Locale,
): Partial<Record<keyof Invitee, string>> {
  const required = (label: string) =>
    fieldCheckSentence("required", locale, label);
  return {
    ...(invitee.full_name === ""
      ? { full_name: required(labels.full_name) }
      : null),
    ...(invitee.email === ""
      ? { email: required(labels.email) }
      : looksLikeEmail(invitee.email)
        ? null
        : {
            email: fieldCheckSentence("email_malformed", locale, labels.email),
          }),
  };
}

export function InviteDialog({
  room,
  open,
  onClose,
}: Readonly<{ room: DealRoom; open: boolean; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [capability, setCapability] = useState<Capability>("comment");
  const [issued, setIssued] = useState<Issued | null>(null);
  const labels: Invitee = {
    full_name: t("access.nameLabel"),
    email: t("access.emailLabel"),
  };
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
      if (data?.queued) {
        watchInviteMail(queryClient, room.id);
      }
      void refreshSeats(queryClient, room.id);
      if (data) {
        setIssued(data);
      }
    },
    onError: (error) => {
      faults.show(refusalSentences(error, t, labels).byField);
    },
  });
  const close = () => {
    setIssued(null);
    setName("");
    setEmail("");
    setCapability("comment");
    faults.show(new Map());
    invite.reset();
    onClose();
  };
  const send = () => {
    const invitee = { full_name: name.trim(), email: email.trim() };
    const found = inviteeChecks(invitee, labels, locale);
    if (faults.show(new Map(Object.entries(found)))) {
      return;
    }
    invite.mutate({
      name: invitee.full_name,
      email: invitee.email,
      capability,
    });
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
      pending={invite.isPending}
      error={
        invite.isError ? refusalSentences(invite.error, t, labels).rest : null
      }
      onConfirm={() => (issued ? close() : send())}
      intent="form"
    >
      {issued ? (
        <IssuedLink issued={issued} />
      ) : (
        <>
          <div ref={faultsRoot} className="form-stack">
            <Field
              label={labels.full_name}
              required
              error={faults.of("full_name")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    faults.edited("full_name");
                  }}
                />
              )}
            </Field>
            <Field label={labels.email} required error={faults.of("email")}>
              {(control) => (
                <TextInput
                  {...control}
                  type="email"
                  value={email}
                  onChange={(e) => {
                    setEmail(e.target.value);
                    faults.edited("email");
                  }}
                />
              )}
            </Field>
          </div>
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
      if (data?.queued) {
        watchInviteMail(queryClient, room.id);
      }
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
  const readerZone = useViewerZone();
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
  const close = () => {
    revoke.reset();
    onClose();
  };
  return (
    <ConfirmModal
      open={open}
      onClose={close}
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
          ? ` · ${t("access.lastSeen", { when: formatDateAbbrev(participant.last_seen_at, locale, readerZone) })}`
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
  const close = () => {
    change.reset();
    setCapability(participant.capability);
    onClose();
  };
  return (
    <ConfirmModal
      open={open}
      onClose={close}
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
