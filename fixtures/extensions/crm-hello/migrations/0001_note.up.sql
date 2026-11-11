-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- A note about a company and one of its contacts. The unit declares both ids in
-- its Reach, so the core's merge moves them and its erasure clears them.
CREATE TABLE ext.ext_crm_hello_note (
    id          uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    company_id  uuid,
    contact_id  uuid,
    body        text NOT NULL
);

-- Conditional because a throwaway database applying this under its owning role
-- has no margince_app role.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_crm_hello_note TO margince_app;
  END IF;
END $$;
