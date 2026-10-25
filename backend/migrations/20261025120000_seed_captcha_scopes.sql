INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at") VALUES
(gen_random_uuid(), 'captcha', 'captcha_scopes', '"login,register"'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO NOTHING;
