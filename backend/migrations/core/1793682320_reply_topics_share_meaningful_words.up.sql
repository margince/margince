SET LOCAL lock_timeout = '3s';

CREATE FUNCTION mail_topic_words(subject text) RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT coalesce(array_agg(DISTINCT word), ARRAY[]::text[])
      FROM regexp_split_to_table(lower(upper(regexp_replace(coalesce(subject, '') COLLATE "und-x-icu",
           '\[[^]]*\]', ' ', 'g'))), '[^[:alnum:]]+') AS word
     WHERE length(word) > 1 AND word ~ '[[:alpha:]]'
       AND word <> ALL (ARRAY[
           're', 'aw', 'antw', 'fwd', 'fw', 'wg', 'sv', 'vs',
           'the', 'and', 'for', 'with', 'from', 'this', 'that', 'of', 'to', 'in', 'on', 'an', 'is', 'it', 'at', 'by', 'or',
           'der', 'die', 'das', 'ein', 'eine', 'und', 'oder', 'für', 'mit', 'von', 'zu', 'im', 'am', 'den', 'dem', 'des', 'ist',
           'và', 'của', 'cho', 'với', 'từ', 'các', 'những', 'là'
       ]);
$$;

CREATE FUNCTION mail_topics_differ(parent_subject text, reply_subject text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT cardinality(parent_words) > 0 AND cardinality(reply_words) > 0
           AND NOT parent_words && reply_words
      FROM (SELECT mail_topic_words(parent_subject) AS parent_words,
                   mail_topic_words(reply_subject) AS reply_words) AS topics;
$$;
