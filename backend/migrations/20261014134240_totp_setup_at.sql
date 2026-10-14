-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "totp_setup_at" timestamptz NULL;
