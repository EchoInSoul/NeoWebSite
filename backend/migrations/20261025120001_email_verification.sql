-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "email_verification_status" character varying;
UPDATE "public"."users" SET "email_verification_status" = 'not_required';
ALTER TABLE "public"."users" ALTER COLUMN "email_verification_status" SET NOT NULL;
-- Create "pending_registrations" table
CREATE TABLE "public"."pending_registrations" (
  "id" uuid NOT NULL,
  "email" character varying NOT NULL,
  "nickname" character varying NOT NULL,
  "password_hash" character varying NOT NULL,
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "pending_registrations_email_key" to table: "pending_registrations"
CREATE UNIQUE INDEX "pending_registrations_email_key" ON "public"."pending_registrations" ("email");
-- Create index "pending_registrations_token_hash_key" to table: "pending_registrations"
CREATE UNIQUE INDEX "pending_registrations_token_hash_key" ON "public"."pending_registrations" ("token_hash");
-- Create index "pendingregistration_expires_at" to table: "pending_registrations"
CREATE INDEX "pendingregistration_expires_at" ON "public"."pending_registrations" ("expires_at");
