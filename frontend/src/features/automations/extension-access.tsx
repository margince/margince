// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, Info } from "lucide-react";
import { type ReactNode, useEffect } from "react";
import type { components } from "../../api/schema";
import { Badge, Disclosure, EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FactList } from "../../design-system/factlist";
import { KeyedName } from "../../design-system/keyedname";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  QueryGate,
  type QueryLike,
  QueryStates,
} from "../../design-system/querystates";
import { INTL_LOCALE } from "../../format/format";
import { objectLabel } from "../../format/rbacobjects";
import { useLocale, useT } from "../../i18n";
import { useCan, useCanMutate } from "../../lib/capability";
import {
  composedScreens,
  EXTENSION_SCREEN,
  extensionTitle,
  findExtension,
} from "../../lib/extensions";
import {
  isVersionSkew,
  ProblemError,
  problemMessageOf,
} from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { useMe } from "../../lib/session";
import "./extension-access.css";
import {
  EXTENSIONS_KEY,
  type ExtensionUnit,
  useExtensions,
} from "../../lib/extensions.queries";
import {
  ROLES_KEY,
  type Role,
  useRoles,
  useSetRoleGrant,
} from "../../lib/roles.queries";
import {
  GrantMatrix,
  type GrantMatrixRow,
  type GrantMatrixSection,
  grantOf,
} from "../settings/grantmatrix";
import { UnitGovernance } from "./extension-governance";

// Extension access (settings → Extensions): what the composed extension tier
// brought into this installation, and who may use it.
//
// The problem it exists to solve: a unit registers its own RBAC objects at
// composition time (ext_notes_note, ext_notes_signing_key, …), but nothing in
// the product could grant them — the only lever was hand-written SQL against
// role.permissions. So a shipped, enabled extension renders "you do not hold
// access" for every seat, and the screen that would explain why did not exist.
// This is that screen: the unit inventory, and one role × CRUD matrix per
// object the unit registered.
//
// It is deliberately NOT a general role editor. It shows the objects the
// extension tier contributed, because those are the ones no seeded role can
// possibly hold a grant on — every core object already comes granted by the
// bootstrap matrix.

// The wire shapes, read straight off the generated contract. `Role.version` is
// an int64 RowVersion, NOT a string: it rides back out as `If-Match`,
// stringified at the call.
export type ExtensionRole = Role;

// One entry per OPERATION, not per path: the inventory is deduplicated by
// (path, method) server-side precisely so a DELETE cannot hide behind a GET on
// the same path. The method is therefore rendered, never collapsed away — an
// operator reading this screen has to be able to see that a unit's route can
// delete.
export type ExtensionRoute = components["schemas"]["ComposedExtensionRoute"];

// What the two reads answer together: the state pane renders its pending and
// error surface, the unit cards render the units in it.
type ComposedAccess = Readonly<{
  extensions: readonly ExtensionUnit[];
  roles: readonly ExtensionRole[];
}>;

// One pending/error surface for the two reads the screen needs together: a
// matrix with roles but no unit inventory is not a partial screen, it is a
// different screen. QueryStates takes any QueryLike, so the merge needs no
// react-query lie.
function useExtensionAccess(enabled: boolean): QueryLike<ComposedAccess> {
  const extensions = useExtensions(enabled);
  const roles = useRoles(enabled);
  const queryClient = useQueryClient();
  // `enabled: false` stops the next READ; it does not forget the last one. A seat
  // whose admin role is taken away mid-session keeps rendering from the cache
  // otherwise — the whole inventory and every role's grant on every object, to a
  // reader the server would now refuse. Losing the role evicts what it bought.
  useEffect(() => {
    if (enabled) {
      return;
    }
    queryClient.removeQueries({ queryKey: EXTENSIONS_KEY });
    queryClient.removeQueries({ queryKey: ROLES_KEY });
  }, [enabled, queryClient]);
  return {
    isPending: extensions.isPending || roles.isPending,
    isError: extensions.isError || roles.isError,
    error: extensions.error ?? roles.error,
    data:
      extensions.data && roles.data
        ? { extensions: extensions.data, roles: roles.data }
        : undefined,
    refetch: () => {
      void extensions.refetch();
      void roles.refetch();
    },
  };
}

export function ExtensionAccessCard() {
  const t = useT();
  const me = useMe();
  // `extension_access:read`, which is what `GET /v1/extensions` asks for
  // (compose/handlers_extensions.go).
  //
  // This WAS the literal admin role, and the comment here argued there was no
  // useCan question to ask — that a `role` object would encode a constant an
  // admin could revoke and never restore. That reasoning was about the object
  // it declined, not about this one: `extension_access` names what an installed
  // unit may reach, ops holds it, and the server answers ops 200. Keeping the
  // role check meant refusing a reader the authority admits.
  //
  // The seat ceiling still ANDs on top, because a read seat may read this page
  // and may not write anything on it.
  //
  // BOTH grants, because the card is two reads behind one flag: the inventory
  // from `GET /v1/extensions` (extension_access:read) and every role's grant on
  // every object from `GET /v1/roles`, which asks for role_admin:read
  // (identity/roles.go ListRoles). On the inventory grant alone the whole card
  // 403s on its second query — and the matrix it draws is the one surface that
  // grants an extension's objects at all, so half of it is not a card.
  // Each predicate on its own line: `a() && b()` short-circuits, and a hook
  // that stops being called between renders is a hook-order violation, not a
  // narrower gate.
  const readsInventory = useCan("extension_access", "read");
  const readsRoles = useCan("role_admin", "read");
  const canSee = readsInventory && readsRoles;
  // The toggles write through PATCH /roles/{key}/objects/{object}, which asks
  // for role_admin:UPDATE (identity/roles.go SetRoleObjectGrant) — a strictly
  // wider grant than the read above. Ops holds the read and not the update, so
  // without this every tick it is shown would 403.
  const seatMayWrite = useCanMutate();
  const writesRoles = useCan("role_admin", "update");
  const canMutate = seatMayWrite && writesRoles;
  const query = useExtensionAccess(canSee);
  const composed = query.data;

  return (
    <div className="ext-stack">
      <QueryGate query={me} pendingLabel={t("extAccess.title")}>
        {() =>
          canSee ? (
            <InventoryState query={query} />
          ) : (
            <Panel>
              <PanelBody>
                <EmptyState>{t("extAccess.adminOnly")}</EmptyState>
              </PanelBody>
            </Panel>
          )
        }
      </QueryGate>
      {/* Gated on the grant as well as on the data: `enabled: false` is about
          the next request, and a card drawn from a cache the reader may no
          longer read is the disclosure the notice above refuses. */}
      {canSee &&
        composed?.extensions.map((unit) => (
          <UnitCard
            key={unit.name}
            unit={unit}
            roles={composed.roles}
            canManage={canMutate}
          />
        ))}
    </div>
  );
}

// The read's pending, error and empty surface, in a pane of its own; with
// units in hand the unit cards speak instead.
function InventoryState({
  query,
}: Readonly<{ query: QueryLike<ComposedAccess> }>) {
  const t = useT();
  if ((query.data?.extensions.length ?? 0) > 0) {
    return null;
  }
  return (
    <Panel>
      <PanelBody>
        <QueryStates query={query} pendingLabel={t("extAccess.title")}>
          <EmptyState>{t("extAccess.empty")}</EmptyState>
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

function UnitCard({
  unit,
  roles,
  canManage,
}: Readonly<{
  unit: ExtensionUnit;
  roles: readonly ExtensionRole[];
  canManage: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Two sources say which units exist and they can disagree. The inventory
  // above is what /extensions answered — what the RUNNING BINARY composed —
  // while `#/ext/<name>` can only resolve what the SPA's generated registry
  // was built with. A bundle older than the binary lists a unit here whose
  // page App.tsx's ExtensionRoute would answer with its (deliberate, honest)
  // not-found card.
  //
  // So the link is rendered from the registry, never from the API's list. And
  // when the registry misses, the miss is SAID rather than silently omitted:
  // "this unit has no page" and "the page exists but this bundle predates it"
  // are different facts, and an operator hunting a missing screen has to be
  // able to tell them apart — an absent link would leave them debugging the
  // grants below instead of the deploy.
  // How much this unit contributes at all. A jurisdiction pack contributes
  // none of the three: it supplies policy the core consults and registers no
  // object, route or job, which is a complete answer rather than an empty card.
  const brings =
    unit.rbac_objects.length + unit.routes.length + unit.jobs.length;
  const descriptor = findExtension(unit.name);
  // A descriptor is not a page. `de` has one and publishes NO operations, so
  // the generic fallback App.tsx renders for a unit without its own screen
  // draws a heading over an empty list — a link that promises a page and opens
  // nothing. The link is offered when the unit has something to show: its own
  // screen, or at least one published operation for the fallback to list.
  // Object.hasOwn, not a bare index: the generated registry is an object
  // literal, so `composedScreens["constructor"]` answers from the prototype
  // chain with Object itself — truthy, and the unit-name grammar admits
  // `constructor`. App.tsx guards the same lookup the same way.
  const page =
    descriptor &&
    (Object.hasOwn(composedScreens, descriptor.name) ||
      descriptor.verbs.length > 0)
      ? descriptor
      : null;
  const unread = unit.rbac_objects.filter((object) =>
    roles.every((role) => !grantOf(role, object).read),
  );
  const title = extensionTitle(unit.name, locale);
  return (
    <Panel
      className="ext-unit"
      title={title}
      titleAction={
        <div className="ext-unit-actions">
          <Badge>{t("extAccess.version", { version: unit.version })}</Badge>
          {page ? (
            // The unit's name is IN the link text: several units share the
            // page, and "Open" five times names nothing out of context. The
            // hash comes from routeHash and the exported screen token, so
            // link and router cannot drift apart.
            <a
              className="ext-unit-link"
              href={routeHash({ screen: EXTENSION_SCREEN, id: page.name })}
            >
              <ArrowUpRight aria-hidden size={15} />
              <span className="ext-unit-link-text">
                {t("extAccess.openUnit", { name: title })}
              </span>
            </a>
          ) : null}
        </div>
      }
    >
      <PanelBody className="ext-unit-body">
        <p>{unit.description}</p>
        {/* Said only where the two registries disagree: a unit the running
            binary composed whose descriptor this bundle does not carry is a
            version skew, not a unit that simply has no page. */}
        {descriptor ? null : (
          <p className="ext-unit-nopage">
            <Info aria-hidden size={15} />
            {t("extAccess.noPage", { name: title })}
          </p>
        )}
        {unit.rbac_objects.length === 0 ? (
          <p className="t-caption">{t("extAccess.noObjects")}</p>
        ) : null}
        {/* Every switch below also carries this as its `reason`, which is what
            puts it in the accessibility tree beside the control. */}
        {!canManage && unit.rbac_objects.length > 0 ? (
          <Callout kind="standing" title={t("extAccess.readOnlyTitle")}>
            {t("extAccess.readOnly")}
          </Callout>
        ) : null}
        {/* Spoken although standing: it appears and disappears as the last
            read grant is toggled, and the change is the news. */}
        {unread.length > 0 ? (
          <Callout
            tone="warning"
            kind="standing"
            live="status"
            title={t("extAccess.nobodyReadsTitle")}
          >
            {t("extAccess.nobodyReads", {
              object: new Intl.ListFormat(INTL_LOCALE[locale], {
                style: "long",
                type: "conjunction",
              }).format(unread.map((object) => objectLabel(unit.name, object))),
            })}
          </Callout>
        ) : null}
      </PanelBody>
      {unit.rbac_objects.length > 0 ? (
        <UnitMatrix
          unit={unit}
          title={title}
          roles={roles}
          canManage={canManage}
        />
      ) : null}
      <UnitGovernance unit={unit.name} canManage={canManage} />
      {/* Reference, not a decision: an operator opens it to check which
          object gates the route they care about. */}
      {brings > 0 ? (
        <PanelBody>
          <Disclosure summary={t("extAccess.brings.heading")}>
            <UnitBrings unit={unit} />
          </Disclosure>
        </PanelBody>
      ) : null}
    </Panel>
  );
}

// Built-in and custom roles are told apart only where both kinds stand in the
// grid. The mark goes on the fewer of the two, so it says what differs.
function roleBadges(
  roles: readonly ExtensionRole[],
  t: ReturnType<typeof useT>,
): ReadonlyMap<string, string> {
  const custom = roles.filter((role) => !role.is_system);
  if (custom.length === 0 || custom.length === roles.length) {
    return new Map();
  }
  const markCustom = custom.length * 2 <= roles.length;
  const marked = markCustom ? custom : roles.filter((role) => role.is_system);
  const word = t(markCustom ? "extAccess.customRole" : "extAccess.systemRole");
  return new Map(marked.map((role) => [role.key, word]));
}

function UnitBrings({ unit }: Readonly<{ unit: ExtensionUnit }>) {
  const t = useT();
  const list = (
    items: readonly string[],
    show: (item: string) => ReactNode = (item) => item,
  ) =>
    items.length === 0 ? (
      t("extAccess.brings.none")
    ) : (
      <ul className="ext-brings-list">
        {items.map((item) => (
          <li key={item}>{show(item)}</li>
        ))}
      </ul>
    );
  return (
    <FactList
      facts={[
        {
          key: "objects",
          term: t("extAccess.brings.objects"),
          value: list(unit.rbac_objects, (object) => (
            <KeyedName name={objectLabel(unit.name, object)} code={object} />
          )),
        },
        {
          key: "routes",
          term: t("extAccess.brings.routes"),
          // One entry per method and path: a DELETE must not hide behind the
          // GET on the same path.
          value: list(
            unit.routes.map((route) => `${route.method} ${route.path}`),
          ),
        },
        {
          key: "jobs",
          term: t("extAccess.brings.jobs"),
          value: list(unit.jobs),
        },
      ]}
    />
  );
}

// The unit's role × CRUD grid, one group of role rows per object it registers.
function UnitMatrix({
  unit,
  title,
  roles,
  canManage,
}: Readonly<{
  unit: ExtensionUnit;
  title: string;
  roles: readonly ExtensionRole[];
  canManage: boolean;
}>) {
  const t = useT();
  const setGrant = useSetRoleGrant();
  const badges = roleBadges(roles, t);
  // Only a ProblemError carries a server code, so a rejected fetch can never be
  // mistaken for a concurrent edit.
  const skew =
    setGrant.error instanceof ProblemError &&
    isVersionSkew(setGrant.error.problem);
  const sections = unit.rbac_objects.map((object): GrantMatrixSection => {
    const label = objectLabel(unit.name, object);
    return {
      key: object,
      title: label,
      rows: roles.map((role): GrantMatrixRow => {
        const grant = grantOf(role, object);
        return {
          key: role.key,
          name: role.name,
          badge: badges.get(role.key),
          grant,
          cellLabel: (action) =>
            t("extAccess.cell", {
              role: role.name,
              action: t(`extAccess.action.${action}`),
              object: label,
            }),
          // The write carries this role's whole grant on this object.
          pending:
            setGrant.isPending &&
            setGrant.variables?.roleKey === role.key &&
            setGrant.variables.object === object,
          onChange: (action, next) =>
            setGrant.mutate({
              roleKey: role.key,
              object,
              grant: { ...grant, [action]: next },
              // The version this cell was read from: the server compares against
              // what the operator saw.
              version: role.version,
            }),
        };
      }),
    };
  });

  return (
    <>
      {/* One write at a time: two on one role race its version. */}
      <GrantMatrix
        rowHeader={t("extAccess.roleColumn")}
        sections={sections}
        canManage={canManage}
        readOnlyReason={t("extAccess.readOnly")}
        busy={setGrant.isPending}
        name={t("extAccess.matrixName", { unit: title })}
        bleed
      />
      {setGrant.isError ? (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("extAccess.grantFailed")}
          >
            {/* A skew means someone else's flip applied and the grid was just
                repainted with it, which a generic refusal would hide. */}
            {skew
              ? t("extAccess.versionSkew")
              : problemMessageOf(setGrant.error, t)}
          </Callout>
        </PanelBody>
      ) : null}
    </>
  );
}
