SET LOCAL lock_timeout = '3s';
DROP TRIGGER source_change_is_remembered ON public.company;
DROP TRIGGER source_change_is_remembered ON public.contact;
DROP TRIGGER source_change_is_remembered ON public.deal;
DROP TRIGGER source_change_is_remembered ON public.lead;
DROP TRIGGER source_change_is_remembered ON public.offer;
DROP TRIGGER source_change_is_remembered ON public.project;
DROP FUNCTION public.remember_a_triggered_source_change();
