-- A seat's own other addresses: a send-as alias, a private domain the same
-- person reads, an address they forward from.
--
-- Mail among a person's own addresses is not correspondence with anybody, and
-- an alias is not a contact. Without this the capture ladder reads a message
-- FROM an owner's alias as inbound mail from a stranger and mints that stranger
-- as a person — which is how a founder's own private domain became a contact
-- record every seat could read.
--
-- PER USER, never per workspace. One seat's alias says nothing about another
-- seat's mail, and a workspace-wide list would let anyone silence a colleague's
-- counterparty by claiming their address. The workspace's own mail domains are
-- a different thing and already exist (workspace_email_domain): those say "we
-- are all colleagues here", this says "that is also me".
CREATE TABLE capture_owner_identity (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    kind text NOT NULL,
    value text NOT NULL,
    -- Where the claim came from. 'user' is the seat typing it in; 'provider'
    -- is reserved for an address a provider attests (Gmail's sendAs list),
    -- which nothing writes yet — the scope that would authorise reading it is
    -- not requested at grant, and asking for a wider scope is a decision about
    -- what the product asks of a user, not a detail of this table.
    source text NOT NULL DEFAULT 'user',
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_owner_identity_kind_check CHECK (kind IN ('address', 'domain')),
    CONSTRAINT capture_owner_identity_source_check CHECK (source IN ('user', 'provider')),
    -- Folded, like every other address this module stores, so a lookup never
    -- has to guess which case the writer used.
    CONSTRAINT capture_owner_identity_value_check CHECK ((value = lower(value)) AND (value <> '')),
    -- One claim per seat per value. A second insert of the same alias is the
    -- same fact, not a second one.
    CONSTRAINT capture_owner_identity_user_value_key UNIQUE (user_id, kind, value)
);

-- No standalone index on user_id: the UNIQUE (user_id, kind, value) constraint
-- carries a btree leading with user_id, which serves the only read this table
-- has — "every identity of the acting seat" — and the CASCADE above.
