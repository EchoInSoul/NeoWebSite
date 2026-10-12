-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "totp_last_counter" bigint NULL;
