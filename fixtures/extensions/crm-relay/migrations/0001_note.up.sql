-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- One note per relayed message, so the fixture's tool and job read a table of
-- their own.
CREATE TABLE ext.ext_crm_relay_note (
    id       uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    body     text        NOT NULL,
    noted_at timestamptz NOT NULL DEFAULT now()
);
