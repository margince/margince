-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Four indexes that exist for optional sort headers.
--
-- Every one of them serves an ORDER BY a reader may never ask for, and each is
-- maintained on every write. last_activity_at is the clearest case: a trigger
-- writes it whenever an activity moves, so its index is maintained on a hot
-- path and blocks HOT updates besides. Sorting ten thousand companies without
-- an index costs a few milliseconds.
--
-- The sorts themselves are UNCHANGED. companyListFields still offers every one
-- of them; what goes is the index, not the capability. Re-add with a
-- measurement if company counts pass ~100k; display_name ascending is the
-- first one to want back.
--
-- idx_company_geocoded stays, and is not one of these. It is not a sort index:
-- search/querygeo.go answers radius questions by emitting geocode_status = 'ok'
-- with a bounding box over (geocode_lat, geocode_lon), which is exactly this
-- index's shape. Its partial predicate is load-bearing rather than an
-- optimisation — only a resolved row is reachable through it, so a company whose
-- address has moved cannot answer a distance from where it used to be.
DROP INDEX IF EXISTS idx_company_name_keyset;
DROP INDEX IF EXISTS idx_company_name_keyset_desc;
DROP INDEX IF EXISTS idx_company_updated_keyset;
DROP INDEX IF EXISTS idx_company_last_activity_keyset;
