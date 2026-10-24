// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import { searchHitRoute } from "../../lib/searchkinds";
import "./search.css";

type SearchNearMatch = components["schemas"]["SearchNearMatch"];

// Names spelt close to a search that found nothing. Each is a plain link, so
// opening one in a new tab works; a lead shows its company after the name.
export function NearMatches({
  matches,
}: Readonly<{ matches?: readonly SearchNearMatch[] }>) {
  const t = useT();
  if (!matches || matches.length === 0) {
    return null;
  }
  return (
    <div className="search-near">
      <p>{t("search.nearMatches")}</p>
      <ul className="search-hits">
        {matches.map((match) => {
          const route = searchHitRoute(match.type, match.id);
          return (
            <li key={`${match.type}:${match.id}`} className="search-hit">
              {route ? (
                <a className="entity-link" href={routeHash(route)}>
                  {match.title}
                </a>
              ) : (
                <span>{match.title}</span>
              )}
              {match.snippet && <span> · {match.snippet}</span>}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
