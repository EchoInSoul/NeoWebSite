INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at")
VALUES
(gen_random_uuid(), 'mail', 'recovery_codes_subject', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'recovery_codes_body', '""'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO NOTHING;
