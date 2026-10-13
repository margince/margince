import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
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
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// A seat's OWN other addresses: a send-as alias, a private domain the same
// contact reads, an address they forward from.
//
// Mail among a contact's own addresses is not correspondence with anybody, so
// declaring one keeps those messages out of the CRM and stops the address being
// minted as a contact.
//
// The caller's own only, and that is the point rather than a limitation: a
// colleague's alias says which of THEIR mail is private, which is the thing the
// list exists to protect.

type OwnerIdentity = components["schemas"]["CaptureOwnerIdentity"];
type Kind = components["schemas"]["CaptureOwnerIdentityKind"];
type Source = components["schemas"]["CaptureOwnerIdentitySource"];

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
  const add = (
    <Button variant="ghost" onClick={() => setDeclaring(true)}>
      <Plus aria-hidden />
      {t("ownerIdentities.add")}
    </Button>
  );
  const hasRows = (query.data?.data.length ?? 0) > 0;
  return (
    // With rows the verb moves to the head; with none it is the empty state's own.
    <Panel
      title={t("ownerIdentities.title")}
      titleAction={hasRows ? add : null}
    >
      <PanelBody>
        <PanelIntro>{t("ownerIdentities.sub")}</PanelIntro>
        <QueryGate query={query} pendingLabel={t("ownerIdentities.title")}>
          {(list) =>
            list.data.length === 0 ? (
              <EmptyState title={t("ownerIdentities.emptyTitle")} action={add}>
                <span data-testid="owner-identities-empty">
                  {t("ownerIdentities.emptyBody")}
                </span>
              </EmptyState>
            ) : (
              <IdentityTable
                list={list.data}
                pending={remove.isPending}
                onRemove={(id) => remove.mutate(id)}
              />
            )
          }
        </QueryGate>
        {remove.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("ownerIdentities.removeFailed")}
          >
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
 * Where a claim came from. A seat scanning this list would otherwise find an
 * address they never entered and could not tell whether they forgot adding it
 * or Margince learned it, and they are the one who decides whether it stays.
 */
const sourceLabel: Record<Source, MessageKey> = {
  user: "ownerIdentities.source.user",
  delivered_to: "ownerIdentities.source.deliveredTo",
  provider: "ownerIdentities.source.provider",
};

function IdentityTable({
  list,
  pending,
  onRemove,
}: Readonly<{
  list: OwnerIdentity[];
  pending: boolean;
  onRemove: (id: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <DataTable<OwnerIdentity>
        label={t("ownerIdentities.title")}
        rows={list}
        rowKey={(identity) => identity.id}
        columns={[
          {
            key: "address",
            header: t("ownerIdentities.colAddress"),
            render: (identity) => identity.value,
          },
          {
            key: "kind",
            header: t("ownerIdentities.colType"),
            render: (identity) => t(kindLabel[identity.kind]),
          },
          {
            key: "source",
            header: t("ownerIdentities.colSource"),
            render: (identity) => t(sourceLabel[identity.source]),
          },
          {
            key: "remove",
            header: t("ownerIdentities.colRemove"),
            align: "end",
            render: (identity) => (
              <IconAction
                inline
                label={t("ownerIdentities.remove", {
                  value: identity.value,
                })}
                icon={<Trash2 aria-hidden />}
                disabled={pending}
                onClick={() => onRemove(identity.id)}
              />
            ),
          },
        ]}
      />
      {/* Not retroactive, said under the list the reader is changing. */}
      <p className="t-sub">{t("ownerIdentities.notRetroactive")}</p>
    </>
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
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("ownerIdentities.addLabel")}
      </Heading>
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
          <Callout
            tone="danger"
            kind="outcome"
            title={t("ownerIdentities.addFailed")}
          >
            {problemMessageOf(add.error, t)}
          </Callout>
        )}
        <div className="actions">
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
