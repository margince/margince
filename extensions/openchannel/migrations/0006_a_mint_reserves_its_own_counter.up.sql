-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Minting a signing secret reserves a number under the endpoint's row lock,
-- seals outside every transaction of this unit's — Secrets().PutUser opens a
-- pool transaction of its own, and nesting one inside another takes a second
-- connection while holding the first — and then records only if the number it
-- reserved is still the endpoint's own.
--
-- THAT NUMBER CANNOT BE `version`. version is the row's optimistic-concurrency
-- counter and every write bumps it: registering an outward address, pausing the
-- endpoint, anything. A mint that reserved a version and then found it moved
-- would report "another signing secret was minted" for a member who merely
-- changed their URL in the same moment — and the screen discards the secret on
-- that error, so a credential that IS live is thrown away and the member is told
-- to mint again for no reason.
--
-- So the mint reserves a counter only minting touches. A number that moved
-- under a mint then means what the refusal says it means.

ALTER TABLE ext.ext_openchannel_endpoint
    ADD COLUMN secret_version bigint NOT NULL DEFAULT 0;

COMMENT ON COLUMN ext.ext_openchannel_endpoint.secret_version IS
    'Bumped only by minting a signing secret, so a mint can tell another mint from any other write to this row.';
