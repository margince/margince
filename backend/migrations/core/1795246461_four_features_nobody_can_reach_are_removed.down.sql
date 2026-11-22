-- The tables come back empty. What they held is not restored.
SET LOCAL lock_timeout = '3s';

CREATE TABLE public.capture_digest (
    id uuid DEFAULT public.uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    digest_date date NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE TABLE public.company_domain_question_reader (
    domain text NOT NULL,
    reader_id uuid NOT NULL,
    evidence_at timestamp with time zone
);
CREATE TABLE public.data_task_run_history (
    run_id uuid NOT NULL,
    task_id text NOT NULL,
    step text NOT NULL,
    progress jsonb NOT NULL,
    preserved_at timestamp with time zone NOT NULL
);
CREATE TABLE public.stage_progression_outcome (
    id uuid DEFAULT public.uuidv7() NOT NULL,
    approval_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    pipeline_id uuid NOT NULL,
    from_stage_id uuid NOT NULL,
    to_stage_id uuid NOT NULL,
    evidence_kinds text[] DEFAULT '{}'::text[] NOT NULL,
    outcome text NOT NULL,
    rejection_reason text,
    evidence_corrected boolean DEFAULT false NOT NULL,
    decided_by_system boolean DEFAULT false NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL,
    reversed_at timestamp with time zone,
    reversed_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    undo_window_hours integer,
    accepted_at timestamp with time zone,
    CONSTRAINT stage_progression_outcome_kind CHECK ((outcome = ANY (ARRAY['proposed'::text, 'approved_clean'::text, 'approved_edited'::text, 'rejected'::text, 'auto_applied'::text, 'reversed'::text, 'expired'::text, 'superseded'::text]))),
    CONSTRAINT stage_progression_outcome_reason_is_a_rejections CHECK (((rejection_reason IS NULL) OR (outcome = 'rejected'::text))),
    CONSTRAINT stage_progression_outcome_reversal_is_timed CHECK (((reversed_by IS NULL) OR (reversed_at IS NOT NULL))),
    CONSTRAINT stage_progression_outcome_version_js_safe CHECK (((version >= '-9007199254740991'::bigint) AND (version <= '9007199254740991'::bigint)))
);
COMMENT ON COLUMN public.stage_progression_outcome.undo_window_hours IS 'The undo window in force when this move was applied, frozen so a later policy edit cannot extend or close it. NULL on rows written before this column existed, which read the rule live as they always did.';
CREATE TABLE public.stage_progression_policy (
    id uuid DEFAULT public.uuidv7() NOT NULL,
    pipeline_id uuid NOT NULL,
    from_stage_id uuid NOT NULL,
    to_stage_id uuid NOT NULL,
    mode text DEFAULT 'propose'::text NOT NULL,
    clean_acceptance_threshold numeric(4,3) DEFAULT 0.950 NOT NULL,
    correction_reversal_threshold numeric(4,3) DEFAULT 0.010 NOT NULL,
    min_reviewed integer DEFAULT 200 NOT NULL,
    min_observation_days integer DEFAULT 28 NOT NULL,
    window_days integer DEFAULT 30 NOT NULL,
    undo_window_hours integer DEFAULT 72 NOT NULL,
    enabled_by uuid,
    enabled_at timestamp with time zone,
    suspended_at timestamp with time zone,
    suspended_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT stage_progression_policy_enabling_is_timed CHECK (((enabled_by IS NULL) OR (enabled_at IS NOT NULL))),
    CONSTRAINT stage_progression_policy_mode CHECK ((mode = ANY (ARRAY['propose'::text, 'auto'::text]))),
    CONSTRAINT stage_progression_policy_suspension_whole CHECK (((suspended_at IS NULL) = (suspended_reason IS NULL))),
    CONSTRAINT stage_progression_policy_thresholds_are_shares CHECK (((clean_acceptance_threshold >= (0)::numeric) AND (clean_acceptance_threshold <= (1)::numeric) AND (correction_reversal_threshold >= (0)::numeric) AND (correction_reversal_threshold <= (1)::numeric))),
    CONSTRAINT stage_progression_policy_version_js_safe CHECK (((version >= '-9007199254740991'::bigint) AND (version <= '9007199254740991'::bigint))),
    CONSTRAINT stage_progression_policy_volume_is_a_bar CHECK (((min_reviewed > 0) AND (min_observation_days > 0) AND (window_days > 0) AND (undo_window_hours > 0)))
);
COMMENT ON TABLE public.stage_progression_policy IS 'Per-transition governance for automatic stage moves. mode is what an admin ASKED for; the thresholds are what the apply path re-checks every time.';
COMMENT ON COLUMN public.stage_progression_policy.suspended_at IS 'Set by the product when the measured record goes bad, never by an admin. Kept apart from mode so an admin''s setting survives a suspension.';
ALTER TABLE ONLY public.capture_digest
    ADD CONSTRAINT capture_digest_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.capture_digest
    ADD CONSTRAINT capture_digest_user_id_digest_date_key UNIQUE (user_id, digest_date);
ALTER TABLE ONLY public.company_domain_question_reader
    ADD CONSTRAINT company_domain_question_reader_pkey PRIMARY KEY (domain, reader_id);
ALTER TABLE ONLY public.data_task_run_history
    ADD CONSTRAINT data_task_run_history_pkey PRIMARY KEY (run_id);
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.stage_progression_policy
    ADD CONSTRAINT stage_progression_policy_pkey PRIMARY KEY (id);
CREATE INDEX company_domain_question_reader_reader_idx ON public.company_domain_question_reader USING btree (reader_id, domain);
CREATE INDEX idx_stage_progression_outcome_deal_target ON public.stage_progression_outcome USING btree (deal_id, to_stage_id, decided_at DESC);
CREATE INDEX idx_stage_progression_outcome_from_stage ON public.stage_progression_outcome USING btree (from_stage_id);
CREATE INDEX idx_stage_progression_outcome_to_stage ON public.stage_progression_outcome USING btree (to_stage_id);
CREATE INDEX idx_stage_progression_outcome_transition ON public.stage_progression_outcome USING btree (pipeline_id, from_stage_id, to_stage_id, decided_at DESC);
CREATE INDEX idx_stage_progression_policy_from_stage ON public.stage_progression_policy USING btree (from_stage_id);
CREATE INDEX idx_stage_progression_policy_to_stage ON public.stage_progression_policy USING btree (to_stage_id);
CREATE UNIQUE INDEX stage_progression_outcome_approval_ux ON public.stage_progression_outcome USING btree (approval_id);
CREATE UNIQUE INDEX stage_progression_policy_transition_key ON public.stage_progression_policy USING btree (pipeline_id, from_stage_id, to_stage_id);
CREATE TRIGGER text_bound_insert AFTER INSERT ON public.company_domain_question_reader FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"domain": 20000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000}');
CREATE TRIGGER text_bound_insert AFTER INSERT ON public.data_task_run_history FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"step": 20000, "task_id": 20000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"step":20000,"task_id":20000}');
CREATE TRIGGER text_bound_insert AFTER INSERT ON public.stage_progression_outcome FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"outcome": 20000, "evidence_kinds": 20000, "rejection_reason": 2000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}');
CREATE TRIGGER text_bound_insert AFTER INSERT ON public.stage_progression_policy FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), NULL::jsonb, '{"mode": 20000, "suspended_reason": 2000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000,"suspended_reason":2000}');
CREATE TRIGGER text_bound_update AFTER UPDATE ON public.company_domain_question_reader FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), to_jsonb(old.*), '{"domain": 20000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000}');
CREATE TRIGGER text_bound_update AFTER UPDATE ON public.data_task_run_history FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), to_jsonb(old.*), '{"step": 20000, "task_id": 20000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"step":20000,"task_id":20000}');
CREATE TRIGGER text_bound_update AFTER UPDATE ON public.stage_progression_outcome FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), to_jsonb(old.*), '{"outcome": 20000, "evidence_kinds": 20000, "rejection_reason": 2000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}');
CREATE TRIGGER text_bound_update AFTER UPDATE ON public.stage_progression_policy FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(new.*), to_jsonb(old.*), '{"mode": 20000, "suspended_reason": 2000}'::jsonb)) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000,"suspended_reason":2000}');
CREATE TRIGGER trg_stage_progression_outcome_updated BEFORE UPDATE ON public.stage_progression_outcome FOR EACH ROW EXECUTE FUNCTION public.set_updated_at_bump_version();
ALTER TABLE ONLY public.capture_digest
    ADD CONSTRAINT capture_digest_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_user(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.company_domain_question_reader
    ADD CONSTRAINT company_domain_question_reader_domain_fkey FOREIGN KEY (domain) REFERENCES public.company_domain_disposition(domain) ON DELETE CASCADE;
ALTER TABLE ONLY public.company_domain_question_reader
    ADD CONSTRAINT company_domain_question_reader_reader_id_fkey FOREIGN KEY (reader_id) REFERENCES public.app_user(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_deal_fkey FOREIGN KEY (deal_id) REFERENCES public.deal(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_from_stage_fkey FOREIGN KEY (from_stage_id) REFERENCES public.stage(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_pipeline_fkey FOREIGN KEY (pipeline_id) REFERENCES public.pipeline(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_reversed_by_fkey FOREIGN KEY (reversed_by) REFERENCES public.app_user(id) ON DELETE SET NULL;
ALTER TABLE ONLY public.stage_progression_outcome
    ADD CONSTRAINT stage_progression_outcome_to_stage_fkey FOREIGN KEY (to_stage_id) REFERENCES public.stage(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_policy
    ADD CONSTRAINT stage_progression_policy_enabled_by_fkey FOREIGN KEY (enabled_by) REFERENCES public.app_user(id) ON DELETE SET NULL;
ALTER TABLE ONLY public.stage_progression_policy
    ADD CONSTRAINT stage_progression_policy_from_stage_fkey FOREIGN KEY (from_stage_id) REFERENCES public.stage(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_policy
    ADD CONSTRAINT stage_progression_policy_pipeline_fkey FOREIGN KEY (pipeline_id) REFERENCES public.pipeline(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.stage_progression_policy
    ADD CONSTRAINT stage_progression_policy_to_stage_fkey FOREIGN KEY (to_stage_id) REFERENCES public.stage(id) ON DELETE CASCADE;
