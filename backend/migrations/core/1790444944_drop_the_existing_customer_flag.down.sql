SET LOCAL lock_timeout = '3s';

CREATE TABLE consent_existing_customer_flag (
    contact_id uuid NOT NULL,
    sale_reference text NOT NULL,
    collected_at timestamptz NOT NULL,
    similar_goods_note text NOT NULL,
    optout_notice_given boolean NOT NULL,
    set_by_user_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    revoked_at timestamptz,
    revoked_reason text,
    CONSTRAINT consent_existing_customer_notice CHECK (optout_notice_given),
    CONSTRAINT consent_existing_customer_flag_pkey PRIMARY KEY (contact_id),
    CONSTRAINT consent_existing_customer_contact_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE CASCADE,
    CONSTRAINT consent_existing_customer_setter_fkey FOREIGN KEY (set_by_user_id) REFERENCES app_user(id) ON DELETE SET NULL
);

COMMENT ON TABLE consent_existing_customer_flag IS 'UWG §7(3) existing-customer flag with its four cumulative conditions as columns (ADR-0098 D4).';

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE consent_existing_customer_flag TO margince_app;
