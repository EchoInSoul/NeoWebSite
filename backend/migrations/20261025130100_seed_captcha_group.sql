INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at") VALUES
(gen_random_uuid(), 'captcha', 'captcha_enabled', 'false'::jsonb, now()),
(gen_random_uuid(), 'captcha', 'captcha_provider', '"none"'::jsonb, now()),
(gen_random_uuid(), 'captcha', 'captcha_scopes', '"login,register"'::jsonb, now()),
(gen_random_uuid(), 'captcha', 'turnstile_site_key', '""'::jsonb, now()),
(gen_random_uuid(), 'captcha', 'turnstile_secret_key', '""'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO NOTHING;
