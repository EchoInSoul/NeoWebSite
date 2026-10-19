-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "auth_version" bigint NOT NULL DEFAULT 0;
