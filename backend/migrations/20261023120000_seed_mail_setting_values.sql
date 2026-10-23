INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at") VALUES
(gen_random_uuid(), 'mail', 'enabled', 'true'::jsonb, now()),
(gen_random_uuid(), 'mail', 'smtp_host', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'smtp_port', '587'::jsonb, now()),
(gen_random_uuid(), 'mail', 'smtp_encryption', '"starttls"'::jsonb, now()),
(gen_random_uuid(), 'mail', 'smtp_username', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'smtp_password', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'sender_address', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'sender_name', '"NeoBlog"'::jsonb, now()),
(gen_random_uuid(), 'mail', 'signup_code_subject', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'signup_code_body', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'password_reset_subject', '""'::jsonb, now()),
(gen_random_uuid(), 'mail', 'password_reset_body', '""'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO NOTHING;
