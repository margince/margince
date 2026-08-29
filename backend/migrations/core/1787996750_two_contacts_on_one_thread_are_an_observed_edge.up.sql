-- Two external people on the same captured activity are an observed
-- relationship, and until now that fact sat unused in activity_participant:
-- the projection tier held only colleague↔contact pairs, so every account
-- picture was hub-and-spoke through our own team.
--
-- graph_contact_edge is the contact↔contact sibling of graph_interaction_edge
-- and keeps its whole contract: derived from the base tables, recomputed never
-- incremented, carrying no audit trail because a rebuild restores the only
-- correct state, and holding NOTHING from a limited-audience activity — who
-- talked to whom on a limited thread is content, and a projection row readable
-- by everyone would disclose it.
--
-- The pair is stored once, canonically ordered (person_a < person_b), because
-- co-participation has no direction: neither external wrote "to" the other
-- through us, we only saw them on one thread. No inbound/outbound columns for
-- the same reason — a direction we did not observe must not be invented.
CREATE TABLE graph_contact_edge (
    person_a uuid NOT NULL,
    person_b uuid NOT NULL,
    last_at timestamptz NOT NULL,
    count_90d integer DEFAULT 0 NOT NULL,
    count_total integer DEFAULT 0 NOT NULL,
    computed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT graph_contact_edge_pkey PRIMARY KEY (person_a, person_b),
    CONSTRAINT graph_contact_edge_ordered CHECK (person_a < person_b)
);

-- The reverse lookup: every read asks "who does THIS person talk to", and the
-- primary key serves only the person_a side of that question.
CREATE INDEX idx_graph_contact_edge_b ON graph_contact_edge (person_b);
