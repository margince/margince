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
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// A seat's OWN other addresses: a send-as alias, a private domain the same
// person reads, an address they forward from.
//
// Mail among a person's own addresses is not correspondence with anybody, so
// declaring one keeps those messages out of the CRM and stops the address being
// minted as a contact. Until this card there was no way to say so — the endpoint
// shipped and nothing reached it.
//
// The caller's own only, and that is the point rather than a limitation: a
// colleague's alias says which of THEIR mail is private, which is the thing the
// list exists to protect.

type OwnerIdentity = components["schemas"]["CaptureOwnerIdentity"];
type Kind = components["schemas"]["CaptureExclusionKind"];

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
      const { data, error } = await api.POST("/capture/owner-identities", {
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
      toast.show(t("ownerIdentities.added"));
    },
  });
}

function useRemoveIdentity() {
  const queryClient = useQueryClient();
  return useMutation({
    // The id is a VARIABLE, so the press belongs to the render the reader saw
    // (frontend/AGENTS.md, mutation-variable-coverage).
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/capture/owner-identities/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return id;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-owner-identities"] });
    },
  });
}

export function OwnerIdentitiesCard() {
  const t = useT();
  const query = useOwnerIdentities();
  const remove = useRemoveIdentity();
  const [declaring, setDeclaring] = useState(false);
  return (
    <Panel title={t("ownerIdentities.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("ownerIdentities.sub")}</p>
        <SettingList>
          <SettingRow
            label={t("ownerIdentities.addLabel")}
            description={t("ownerIdentities.addDescription")}
            control={
              <Button small onClick={() => setDeclaring(true)}>
                {t("ownerIdentities.add")}
              </Button>
            }
          />
          {/* Not retroactive, and the row says so where the reader is deciding.
              Mail already captured under the old reading stays, and a contact
              already minted from an alias stays until it is merged. */}
          <SettingRow
            label={t("ownerIdentities.current")}
            description={t("ownerIdentities.notRetroactive")}
            layout="stack"
            control={
              <QueryGate query={query}>
                {(list) => (
                  <IdentityRows
                    list={list.data}
                    pending={remove.isPending}
                    onRemove={(id) => remove.mutate(id)}
                  />
                )}
              </QueryGate>
            }
          />
        </SettingList>
        {remove.isError && (
          <Callout tone="danger" live="alert">
            {problemMessageOf(remove.error, t)}
          </Callout>
        )}
        {declaring && <DeclareDialog onClose={() => setDeclaring(false)} />}
      </PanelBody>
    </Panel>
  );
}

const kindLabel: Record<Kind, MessageKey> = {
  address: "ownerIdentities.kind.address",
  domain: "ownerIdentities.kind.domain",
};

/**
 * One declared address per row: what it names on the left, whether it is an
 * address or a whole domain as the row's answer, and the verb that withdraws it
 * at the right.
 *
 * The same row language the exclusions card uses, because a reader auditing
 * what binds their mailbox travels one column across both.
 */
function IdentityRows({
  list,
  pending,
  onRemove,
}: Readonly<{
  list: OwnerIdentity[];
  pending: boolean;
  onRemove: (id: string) => void;
}>) {
  const t = useT();
  if (list.length === 0) {
    // A reading, not a failed read: "I have declared no other addresses" is
    // what a reader opens this card to confirm.
    return (
      <EmptyState>
        <p className="t-small" data-testid="owner-identities-empty">
          {t("ownerIdentities.empty")}
        </p>
      </EmptyState>
    );
  }
  return (
    <SettingList testId="owner-identities-list">
      {list.map((identity) => (
        <SettingRow
          key={identity.id}
          label={identity.value}
          value={t(kindLabel[identity.kind])}
          control={
            <Button
              small
              variant="ghost"
              disabled={pending}
              aria-label={t("ownerIdentities.remove")}
              onClick={() => onRemove(identity.id)}
            >
              <Trash2 aria-hidden size={16} />
            </Button>
          }
        />
      ))}
    </SettingList>
  );
}

// Mounted only while it is open, so a half-typed address is gone the next time
// rather than waiting there under a kind nobody re-chose.
function DeclareDialog({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const add = useAddIdentity();
  const headingId = useId();
  const [kind, setKind] = useState<Kind>("address");
  const [draft, setDraft] = useState("");
  const value = draft.trim();
  return (
    <Modal open onClose={onClose} labelledBy={headingId}>
      <h2 id={headingId} className="t-h2 modal-title">
        {t("ownerIdentities.addLabel")}
      </h2>
      <form
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
          <Callout tone="danger" live="alert">
            {problemMessageOf(add.error, t)}
          </Callout>
        )}
        <div className="modal-actions">
          <Button variant="ghost" onClick={onClose} type="button">
            {t("create.cancel")}
          </Button>
          <Button type="submit" disabled={value === "" || add.isPending}>
            {t("ownerIdentities.confirm")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
