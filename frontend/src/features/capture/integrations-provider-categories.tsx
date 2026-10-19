// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import type { ProviderConnection } from "./integrations-provider";
import { BuyableRefused } from "./integrations-provider.notices";
import { categoryName } from "./provider-categories";

// What this installation is willing to buy, one switch per priced category.
//
// Switching one on spends nothing and schedules nothing. It decides which buy
// buttons a rep is offered on a contact. Every purchase is still a rep pressing
// a priced button on one named record, the split the free tier exists to keep.
// So the switch means "available to buy", never "will be bought", and the row's
// copy has to say so. An admin who reads it as the latter leaves the paid half
// of the product switched off.
//
// The free categories are not here. The automatic lookup takes them, and the
// switch above governs them as one decision. A second control that could turn
// one off would half-disable a feature with nothing on screen explaining why.
export function PricedCategoryRows({
  connection,
  canEdit,
}: Readonly<{ connection: ProviderConnection; canEdit: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const patch = usePatchCategories();
  const priced = (connection.catalog ?? []).filter((entry) => !entry.free);
  // A provider nobody has connected is listed anyway, with its catalog, so the
  // card can say what it sells before a key exists. It has no row, and the
  // server omits the version for one. The contract declares `version` required,
  // so the types do not object to reading it. At runtime the patch would carry
  // `If-Match: undefined` and be refused as malformed every time.
  //
  // Absent rather than disabled: there is no connection to decide about yet. A
  // dead switch beside a key field reads as a permission the reader lacks.
  // The switches appear with the connection, which is the order the decision
  // has: a key, then what it may be spent on.
  const version = connection.version;
  if (priced.length === 0 || version === undefined) {
    return null;
  }
  const selection = connection.configuration.categories ?? {};
  return (
    <>
      {priced.map((entry) => {
        // A category the provider issues only alongside another cannot be
        // bought while that one is off: the vendor refuses the request. The
        // switch would promise a purchase nobody can make, and the buy button
        // would never appear, with nothing on screen saying why.
        //
        // Disabled with the reason rather than hidden: an admin looking for
        // the mobile number needs to find it and learn what it depends on. A
        // row that vanished would read as "this provider does not sell it".
        // Truthy, not `!== undefined`: the contract types this as
        // `[string, 'null']`. An absent prerequisite arrives as null on the
        // wire and as undefined from a fixture that omits the key. Both mean
        // "stands alone", and a check that caught only one would block every
        // row the server sent.
        const prerequisite = entry.requires ? entry.requires : undefined;
        const on = selection[entry.category] ?? false;
        // Refused only where the flip would turn it on. Switching a
        // prerequisite off leaves its dependent saved as on, and blocking that
        // row outright would strand the admin. The only control able to undo
        // the combination would be the one they cannot reach. Turning the
        // prerequisite back on to release it spends nothing, but reads like
        // the card fighting them. Off is always allowed.
        const blocked =
          prerequisite !== undefined && !selection[prerequisite] && !on;
        const dependency =
          prerequisite === undefined
            ? undefined
            : t("provider.buyableNeeds", {
                prerequisite: categoryName(prerequisite, t),
              });
        return (
          <SettingRow
            key={entry.category}
            label={t("provider.buyable", {
              category: categoryName(entry.category, t),
            })}
            // The dependency sentence belongs to the Switch's `reason`, which
            // renders and announces it. Repeating it here would read it to a
            // screen reader twice and print it twice on the card.
            description={plural(
              "provider.buyableHint",
              creditsOf(entry),
              // Priced for the pair where there is one, so a dependent row
              // never quotes a figure smaller than the press spends.
              { credits: formatNumber(creditsOf(entry), locale) },
            )}
            control={(control) => (
              <Switch
                describedBy={control["aria-describedby"]}
                checked={selection[entry.category] ?? false}
                // The whole selection, not the one key. The contract's patch
                // replaces the map rather than merging into it. Sending one
                // pair would switch off every category not named, including the
                // free ones the automatic lookup runs on.
                onChange={(next) =>
                  patch.mutate({
                    provider: connection.provider,
                    version,
                    categories: { ...selection, [entry.category]: next },
                  })
                }
                // The admin denial comes first. A seat that may not edit
                // anything is not told about a dependency it could not act on
                // either way.
                reason={
                  canEdit
                    ? blocked
                      ? dependency
                      : undefined
                    : t("captureSettings.adminOnly")
                }
                disabled={!canEdit || blocked}
                pending={patch.isPending}
                label={t("provider.buyable", {
                  category: categoryName(entry.category, t),
                })}
                labelHidden
              />
            )}
          />
        );
      })}
      <BuyableRefused error={patch.error} />
    </>
  );
}

/** The credits one category costs, summed across pools. A category priced in
 *  two pools is one purchase, and two figures on one row would read as a
 *  choice between them. */
function creditsOf(
  entry: components["schemas"]["ProviderCategoryCost"],
): number {
  return Object.values(entry.cost).reduce((total, n) => total + n, 0);
}

type CategoryPatch = {
  provider: components["schemas"]["Provider"];
  version: number;
  categories: Record<string, boolean>;
};

// Saving the fetch scope, with the version the card was rendered from.
//
// If-Match rather than a blind write: two admins on this card both decide what
// the installation may spend on. A lost update there is a category switched on
// by somebody who never saw it happen.
function usePatchCategories() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ provider, version, categories }: CategoryPatch) => {
      return unwrap(
        await api.PATCH("/provider-connections/{provider}", {
          params: { path: { provider }, ...ifMatch(version) },
          body: { configuration: { categories } },
        }),
      );
    },
    // Returned, not fired and forgotten. This mutation writes under If-Match
    // with the version the card was rendered from. So it stays pending until
    // the refetch that replaces that version has landed. A switch that went
    // idle first would let the next press send the stale map under the stale
    // version. The server refuses that with a conflict the admin cannot explain.
    //
    // The other mutations on this card write blind, so the same shape costs
    // them only a briefly stale render. They are left as they are rather than
    // made to wait for a reason that is not theirs.
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["provider-connections"] }),
  });
}
