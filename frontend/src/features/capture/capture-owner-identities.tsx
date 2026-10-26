import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Field,
  Modal,
  SegmentedControl,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../../lib/problem";
import {
  type MutationOutcome,
  useRowHandoff,
  useUndoableRemoval,
} from "../../lib/undoableremoval";
import { captureValueMessage } from "./capturevalue";
import "./capture-owner-identities.css";

// A seat's own other addresses. Examples are a send-as alias, a private domain
// the same contact reads, and an address they forward from.
//
// Mail among a contact's own addresses is not correspondence with anybody.
// Declaring one keeps those messages out of the CRM and stops the address being
// minted as a contact.
//
// The caller's own only, by design: a colleague's alias says which of their
// mail is private, which is what the list exists to protect.

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

// The way back from one removal is the id it took away.
type Withdrawal = Readonly<{ id: string }>;

// The server words these for an API reader; the reader gets the catalog's.
const RESTORE_REFUSALS = new Map<string, MessageKey>([
  ["not_found", "ownerIdentities.restoreGone"],
  ["removal_moved_on", "ownerIdentities.restoreMovedOn"],
  ["identity_declared_again", "ownerIdentities.restoreDeclaredAgain"],
]);

// On the hooks, not on `mutate`: an Undo runs after its row has unmounted.
function useRemoveIdentity(outcome: MutationOutcome<Withdrawal | null>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (identity: OwnerIdentity): Promise<Withdrawal> => {
      unwrap(
        await api.DELETE("/capture/owner-identities/{id}", {
          params: { path: { id: identity.id } },
        }),
      );
      return { id: identity.id };
    },
    onError: outcome.onError,
    onSuccess: (withdrawal) => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      outcome.onSuccess(withdrawal);
    },
  });
}

function useRestoreIdentity(outcome: MutationOutcome<Withdrawal>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (withdrawal: Withdrawal) => {
      unwrap(
        await api.POST("/capture/owner-identities/{id}/restore", {
          params: { path: { id: withdrawal.id } },
        }),
      );
    },
    // A refusal can mean the address is already back, so the list reads again.
    onError: (error) => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      outcome.onError(error);
    },
    onSuccess: (_, withdrawal) => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      outcome.onSuccess(withdrawal);
    },
  });
}

function useIdentityRemoval(removed: string) {
  const t = useT();
  const toasts = useUndoableRemoval<Withdrawal>({
    removed,
    restored: t("ownerIdentities.restoredToast"),
    restoreRefused: (error) => {
      const key = RESTORE_REFUSALS.get(problemCodeOf(error) ?? "");
      return key ? t(key) : problemMessageOf(error, t);
    },
  });
  const restore = useRestoreIdentity(toasts.restored);
  return useRemoveIdentity(
    toasts.removed((withdrawal) => restore.mutate(withdrawal)),
  );
}

export function OwnerIdentitiesCard() {
  const t = useT();
  const query = useOwnerIdentities();
  const removeTyped = useIdentityRemoval(t("ownerIdentities.removedToast"));
  // A withdrawn delivered-to alias is refused from then on, and the notice says so.
  const removeLearned = useIdentityRemoval(
    t("ownerIdentities.removedLearnedToast"),
  );
  const removing = [removeTyped, removeLearned].find(
    (removal) => removal.isPending,
  )?.variables?.id;
  const [declaring, setDeclaring] = useState(false);
  const addVerb = useRef<HTMLButtonElement>(null);
  return (
    <Panel title={t("ownerIdentities.title")}>
      <PanelBody>
        <PanelIntro>{t("ownerIdentities.sub")}</PanelIntro>
      </PanelBody>
      {/* One list, so a hairline parts the add row from the first address. The
          add row stands while the addresses load or fail. */}
      <SettingList bleed="settings" testId="owner-identities-list">
        <SettingRow
          label={t("ownerIdentities.addLabel")}
          description={
            query.data?.data.length === 0 ? (
              <span data-testid="owner-identities-empty">
                {t("ownerIdentities.addDescriptionEmpty")}
              </span>
            ) : (
              t("ownerIdentities.addDescription")
            )
          }
          control={
            <Button ref={addVerb} onClick={() => setDeclaring(true)}>
              {t("ownerIdentities.add")}
            </Button>
          }
        />
        <QueryGate query={query} pendingLabel={t("ownerIdentities.title")}>
          {(list) =>
            list.data.map((identity) => (
              <IdentityRow
                key={identity.id}
                identity={identity}
                pending={removing === identity.id}
                landing={() => addVerb.current}
                onRemove={(chosen) =>
                  (chosen.source === "delivered_to"
                    ? removeLearned
                    : removeTyped
                  ).mutate(chosen)
                }
              />
            ))
          }
        </QueryGate>
      </SettingList>
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
 * An address somebody typed needs no explanation. One the product learned does:
 * a seat scanning this card would otherwise find an address they never entered
 * and could not tell who added it. They decide whether it stays.
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

function IdentityRow({
  identity,
  pending,
  landing,
  onRemove,
}: Readonly<{
  identity: OwnerIdentity;
  pending: boolean;
  landing: () => HTMLElement | null;
  onRemove: (identity: OwnerIdentity) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const verbs = useRowHandoff(landing);
  const learned = learnedNote(identity.source);
  return (
    <SettingRow
      label={identity.value}
      description={
        <span className="owner-identity-caption">
          <span>{t(kindLabel[identity.kind])}</span>
          <span>{formatDate(identity.created_at, locale, viewerZone())}</span>
          {learned !== null && <span>{t(learned)}</span>}
        </span>
      }
      control={
        <span className="cell-actions" ref={verbs}>
          <IconAction
            variant="ghost"
            pending={pending}
            label={t("ownerIdentities.remove", { value: identity.value })}
            icon={<Trash2 aria-hidden />}
            onClick={() => onRemove(identity)}
          />
        </span>
      }
    />
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
        <Field
          label={t("ownerIdentities.valueLabel")}
          labelHidden
          hint={t("ownerIdentities.notRetroactive")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder={
                kind === "address"
                  ? t("ownerIdentities.addressPlaceholder")
                  : t("ownerIdentities.domainPlaceholder")
              }
            />
          )}
        </Field>
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
