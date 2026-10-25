INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at")
SELECT gen_random_uuid(), 'captcha', "name", "value", now()
FROM "public"."setting_values"
WHERE "setting_group" = 'auth'
  AND "name" IN ('captcha_enabled', 'captcha_provider', 'captcha_scopes', 'turnstile_site_key', 'turnstile_secret_key')
ON CONFLICT ("setting_group", "name") DO NOTHING;

DELETE FROM "public"."setting_values"
WHERE "setting_group" = 'auth'
  AND "name" IN ('captcha_enabled', 'captcha_provider', 'captcha_scopes', 'turnstile_site_key', 'turnstile_secret_key');
