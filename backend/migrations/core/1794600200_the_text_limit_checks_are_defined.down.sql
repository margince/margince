SET LOCAL lock_timeout = '2s';
DROP FUNCTION public.refuse_oversized_text();
DROP FUNCTION public.text_row_exceeds_bounds(jsonb, jsonb, jsonb);
DROP FUNCTION public.text_values_exceed_bound(text[], text[], integer);
DROP FUNCTION public.text_value_exceeds_bound(text, text, integer);
