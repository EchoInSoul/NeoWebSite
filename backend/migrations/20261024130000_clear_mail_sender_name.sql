UPDATE "public"."setting_values"
SET "value" = '""'::jsonb,
    "updated_at" = now()
WHERE "setting_group" = 'mail'
  AND "name" = 'sender_name'
  AND "value" = '"NeoBlog"'::jsonb;
