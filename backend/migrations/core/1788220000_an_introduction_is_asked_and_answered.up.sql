-- The person graph names who can open a door; nothing recorded the asking.
-- A rep left the CRM to write the request, and whether the colleague agreed,
-- introduced, or declined lived in somebody's inbox — so the next reader saw a
-- warm route and no sign it had already been asked and refused.
--
-- One row per ask, carrying the request, the colleague's answer, and what
-- actually happened. The statuses are kept apart on purpose: permission to
-- mention a name is NOT an introduction, and a screen that reported
-- `introduced` because a colleague allowed a name-drop would claim a handshake
-- nobody made.
SET LOCAL lock_timeout = '3s';
CREATE TABLE intro_request (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  person_id uuid NOT NULL REFERENCES person(id) ON DELETE CASCADE,
  requester_user_id uuid NOT NULL REFERENCES app_user(id),
  introducer_user_id uuid NOT NULL REFERENCES app_user(id),
  route_type text NOT NULL,
  through_person_id uuid REFERENCES person(id),

  -- Three pieces of copy, because they have three audiences. The reason is the
  -- requester talking to their colleague; the value is what the CONTACT gets;
  -- the forwardable note is prose the colleague can paste to the contact. One
  -- field for all three would send the internal ask to the prospect.
  internal_reason text NOT NULL,
  value_for_target text NOT NULL DEFAULT '',
  forwardable_note text NOT NULL DEFAULT '',

  -- Who wrote the note, kept beside the note itself. Marking it machine-authored
  -- only while the drawer is open loses the fact everywhere it matters after:
  -- the colleague's screen, and the request's own history.
  note_generated_by text NOT NULL DEFAULT 'human',
  note_ai_generated boolean NOT NULL DEFAULT false,

  -- The requester's answer to "what if this colleague cannot". Stored, not held
  -- in a browser: the colleague reads it when deciding, and an audit has to be
  -- able to say what fallback was chosen.
  fallback_policy text NOT NULL DEFAULT 'none',
  name_drop_allowed boolean NOT NULL DEFAULT false,

  status text NOT NULL DEFAULT 'requested',
  decision_reason text,
  suggested_user_id uuid REFERENCES app_user(id),

  due_at timestamptz NOT NULL,
  requested_at timestamptz NOT NULL DEFAULT now(),
  decided_at timestamptz,
  introduced_at timestamptz,
  name_dropped_at timestamptz,
  replied_at timestamptz,
  closed_at timestamptz,
  -- The activity that closed it, when one did. A reply advanced by the consumer
  -- names its evidence; a human marking it introduced may name one too.
  source_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,

  version integer NOT NULL DEFAULT 1,
  captured_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,

  CONSTRAINT intro_request_route_type CHECK (route_type IN ('direct', 'through_contact')),
  -- A route through somebody names them, and a direct one does not. Either half
  -- alone describes a route nobody can act on.
  CONSTRAINT intro_request_route_shape CHECK
    ((route_type = 'through_contact') = (through_person_id IS NOT NULL)),
  CONSTRAINT intro_request_status CHECK (status IN (
    'requested', 'accepted', 'name_drop_approved', 'suggest_other', 'declined',
    'introduced', 'name_dropped', 'replied', 'expired', 'cancelled')),
  -- Suggesting somebody else without naming them is not an answer.
  CONSTRAINT intro_request_suggestion_named CHECK
    (status <> 'suggest_other' OR suggested_user_id IS NOT NULL),
  CONSTRAINT intro_request_fallback CHECK
    (fallback_policy IN ('name_drop', 'next_route', 'none')),
  CONSTRAINT intro_request_note_origin CHECK
    (note_generated_by IN ('human', 'model', 'deterministic')),
  CONSTRAINT intro_request_reason_present CHECK (internal_reason <> ''),
  -- A colleague cannot introduce somebody to themselves, and asking them to is
  -- a routing bug rather than a favour.
  CONSTRAINT intro_request_distinct_seats CHECK (requester_user_id <> introducer_user_id)
);

-- One open ask per route per contact. The duplicate guard is the index rather
-- than a read-then-write check: two tabs pressing send at once both pass the
-- check and only one may pass this.
CREATE UNIQUE INDEX intro_request_open_route ON intro_request (
  person_id,
  introducer_user_id,
  COALESCE(through_person_id, '00000000-0000-0000-0000-000000000000'::uuid)
) WHERE status IN ('requested', 'accepted', 'name_drop_approved') AND archived_at IS NULL;

-- The colleague's own queue: what is waiting on them, soonest due first.
CREATE INDEX intro_request_awaiting_decision ON intro_request (introducer_user_id, due_at)
  WHERE status = 'requested' AND archived_at IS NULL;

-- The sweep's read: what has run out of time. Every status that can expire, so
-- an accepted ask cannot sit accepted for ever.
CREATE INDEX intro_request_due ON intro_request (due_at)
  WHERE status IN ('requested', 'accepted', 'name_drop_approved') AND archived_at IS NULL;

-- The tab's read: every ask about this contact, newest first.
CREATE INDEX intro_request_for_person ON intro_request (person_id, requested_at DESC);
