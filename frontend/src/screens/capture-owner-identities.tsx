import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  EmptyState,
  Modal,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { captureValueMessage } from "./capturevalue";
import { QueryGate, throwProblem, unwrap } from "./common";
import { type MutationOutcome, useUndoableRemoval } from "./undoableremoval";
import "./capture-owner-identities.css";

// A seat's OWN other addresses: a send-as alias, a private domain the same
// contact reads, an address they forward from.
//
// Mail among a contact's own addresses is not correspondence with anybody, so
// declaring one keeps those messages out of the CRM and stops the address being
// minted as a contact. Until this card there was no way to say so — the endpoint
// shipped and nothing reached it.
//
// The caller's own only, and that is the point rather than a limitation: a
// colleague's alias says which of THEIR mail is private, which is the thing the
// list exists to protect.

type OwnerIdentity = components["schemas"]["CaptureOwnerIdentity"];
type Kind = components["schemas"]["CaptureOwnerIdentityKind"];

const KINDS: readonly Kind[] = ["address", "domain"];

function useOwnerIdentities() {
  return useQuery({
    queryKey: ["capture-owner-identities"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/capture/owner-identities",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useAddIdentity() {
  const toast = useToast();
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: { kind: Kind; value: string }) => {
      return unwrap(
        await api.POST("/capture/owner-identities", {
          body,
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      toast.show(t("ownerIdentities.added"));
    },
  });
}

type Declared = Pick<OwnerIdentity, "kind" | "value">;

// On the hooks, not on `mutate`: an Undo runs after its row has unmounted.
function useRemoveIdentity(outcome: MutationOutcome<Declared | null>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (identity: OwnerIdentity): Promise<Declared | null> => {
      unwrap(
        await api.DELETE("/capture/owner-identities/{id}", {
          params: { path: { id: identity.id } },
        }),
      );
      // Declaring it again restores a typed address. A learned one would come
      // back as typed, so it offers no undo.
      return identity.source === "user"
        ? { kind: identity.kind, value: identity.value }
        : null;
    },
    onError: outcome.onError,
    onSuccess: (declared) => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      outcome.onSuccess(declared);
    },
  });
}

function useRestoreIdentity(outcome: MutationOutcome<Declared>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (declared: Declared) => {
      unwrap(
        await api.POST("/capture/owner-identities", {
          body: { kind: declared.kind, value: declared.value },
        }),
      );
    },
    onError: outcome.onError,
    onSuccess: (_, declared) => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      outcome.onSuccess(declared);
    },
  });
}

function useIdentityRemoval() {
  const t = useT();
  const toasts = useUndoableRemoval<Declared>({
    removed: t("ownerIdentities.removedToast"),
    restored: t("ownerIdentities.restoredToast"),
  });
  const restore = useRestoreIdentity(toasts.restored);
  return useRemoveIdentity(
    toasts.removed((declared) => restore.mutate(declared)),
  );
}

export function OwnerIdentitiesCard() {
  const t = useT();
  const query = useOwnerIdentities();
  const remove = useIdentityRemoval();
  const [declaring, setDeclaring] = useState(false);
  const [confirming, setConfirming] = useState<OwnerIdentity | null>(null);
  return (
    <Panel title={t("ownerIdentities.title")}>
      <PanelBody>
        <PanelIntro>{t("ownerIdentities.sub")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={t("ownerIdentities.addLabel")}
          description={t("ownerIdentities.addDescription")}
          control={
            <Button onClick={() => setDeclaring(true)}>
              {t("ownerIdentities.add")}
            </Button>
          }
        />
      </SettingList>
      <PanelGroupHead title={t("ownerIdentities.current")} level="h3" />
      {/* Not retroactive, and the note says so where the reader is deciding. */}
      <PanelBody>
        <PanelIntro>{t("ownerIdentities.notRetroactive")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("ownerIdentities.title")}>
        {(list) => (
          <IdentityRows
            list={list.data}
            pending={remove.isPending}
            onRemove={(identity) =>
              identity.source === "user"
                ? remove.mutate(identity)
                : setConfirming(identity)
            }
          />
        )}
      </QueryGate>
      <ConfirmModal
        open={confirming !== null}
        onClose={() => setConfirming(null)}
        title={t("ownerIdentities.removeLearnedTitle", {
          value: confirming?.value ?? "",
        })}
        confirmLabel={t("ownerIdentities.removeConfirm")}
        confirmVariant="danger"
        pending={remove.isPending}
        onConfirm={() => {
          if (confirming) {
            remove.mutate(confirming, { onSettled: () => setConfirming(null) });
          }
        }}
      >
        <p>{t("ownerIdentities.removeLearnedBody")}</p>
      </ConfirmModal>
      {declaring && <DeclareDialog onClose={() => setDeclaring(false)} />}
    </Panel>
  );
}

const kindLabel: Record<Kind, MessageKey> = {
  address: "ownerIdentities.kind.address",
  domain: "ownerIdentities.kind.domain",
};

/**
 * A row says where its claim came from, and only when the seat did not make it.
 *
 * An address somebody typed needs no explanation. One the product LEARNED does:
 * a seat scanning this card would otherwise find an address they never entered
 * and have no way to tell whether they forgot adding it or something else did —
 * and they are the colleague who decides whether it stays.
 */
function learnedNote(source: OwnerIdentity["source"]): MessageKey | null {
  switch (source) {
    case "delivered_to":
      return "ownerIdentities.learned.deliveredTo";
    case "provider":
      return "ownerIdentities.learned.provider";
    default:
      return null;
  }
}

/**
 * One declared address per row, with its kind and date under it. The verb at
 * the end withdraws it.
 */
function IdentityRows({
  list,
  pending,
  onRemove,
}: Readonly<{
  list: OwnerIdentity[];
  pending: boolean;
  onRemove: (identity: OwnerIdentity) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (list.length === 0) {
    // A reading, not a failed read: "I have declared no other addresses" is
    // what a reader opens this card to confirm.
    return (
      <EmptyState>
        <p data-testid="owner-identities-empty">{t("ownerIdentities.empty")}</p>
      </EmptyState>
    );
  }
  return (
    <SettingList bleed="records" testId="owner-identities-list">
      {list.map((identity) => {
        const learned = learnedNote(identity.source);
        return (
          <SettingRow
            key={identity.id}
            label={identity.value}
            description={
              <span className="owner-identity-caption">
                <span>{t(kindLabel[identity.kind])}</span>
                <span>
                  {formatDate(identity.created_at, locale, viewerZone())}
                </span>
                {learned !== null && <span>{t(learned)}</span>}
              </span>
            }
            control={
              <IconAction
                variant="ghost"
                disabled={pending}
                label={t("ownerIdentities.remove", { value: identity.value })}
                icon={<Trash2 aria-hidden />}
                onClick={() => onRemove(identity)}
              />
            }
          />
        );
      })}
    </SettingList>
  );
}

// Mounted only while it is open, so a half-typed address is gone the next time
// rather than waiting there under a kind nobody re-chose.
function DeclareDialog({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const add = useAddIdentity();
  const headingId = useId();
  const formId = useId();
  const [kind, setKind] = useState<Kind>("address");
  const [draft, setDraft] = useState("");
  const value = draft.trim();
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("ownerIdentities.addLabel")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (value === "") {
            return;
          }
          add.mutate({ kind, value }, { onSuccess: onClose });
        }}
      >
        <SegmentedControl
          options={KINDS}
          value={kind}
          onChange={setKind}
          labels={{
            address: t("ownerIdentities.kind.address"),
            domain: t("ownerIdentities.kind.domain"),
          }}
          label={t("ownerIdentities.kindLabel")}
        />
        <TextInput
          value={draft}
          aria-label={t("ownerIdentities.valueLabel")}
          onChange={(event) => setDraft(event.target.value)}
          placeholder={
            kind === "address"
              ? t("ownerIdentities.addressPlaceholder")
              : t("ownerIdentities.domainPlaceholder")
          }
        />
        {add.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("ownerIdentities.addFailed")}
          >
            {captureValueMessage(add.error, add.variables?.kind, t)}
          </Callout>
        )}
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose} type="button">
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={value === "" || add.isPending}
        >
          {t("ownerIdentities.confirm")}
        </Button>
      </div>
    </Modal>
  );
}
