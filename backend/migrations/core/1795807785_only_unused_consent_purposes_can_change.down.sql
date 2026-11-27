SET LOCAL lock_timeout = '3s';
DROP TRIGGER used_purpose_does_not_change ON public.consent_purpose;
DROP TRIGGER consent_purpose_keys ON public.scheduled_send;
DROP TRIGGER consent_purpose_keys ON public.automation;
DROP TRIGGER consent_purpose_keys ON public.approval;
DROP TRIGGER consent_purpose_keys ON public.communication_decision;
DROP TRIGGER consent_purpose_keys ON public.comms_outbound;
DROP TRIGGER consent_purpose_keys ON public.consent_text_version;
DO $$
DECLARE reference record;
BEGIN
    FOR reference IN SELECT ns.nspname AS schema_name, rel.relname AS table_name
        FROM pg_trigger tr JOIN pg_class rel ON rel.oid = tr.tgrelid JOIN pg_namespace ns ON ns.oid = rel.relnamespace
        WHERE tr.tgname = 'live_consent_purpose_reference'
    LOOP
        EXECUTE format('DROP TRIGGER live_consent_purpose_reference ON %I.%I', reference.schema_name, reference.table_name);
    END LOOP;
END;
$$;
DROP FUNCTION public.lock_consent_purpose_keys();
DROP FUNCTION public.require_a_live_consent_purpose_reference();
DROP FUNCTION public.refuse_a_used_purpose_change();
DROP FUNCTION public.consent_purpose_has_references(uuid, text);
DROP FUNCTION public.consent_purpose_keys_in(jsonb);
DROP FUNCTION public.normalized_consent_purpose_key(text);
