INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at") VALUES
(gen_random_uuid(), 'auth', 'captcha_provider', '"none"'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO UPDATE SET
"value" = EXCLUDED."value",
"updated_at" = now()
WHERE "setting_values"."value" = '""'::jsonb;
