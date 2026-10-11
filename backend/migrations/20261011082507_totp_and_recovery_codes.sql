-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "totp_secret" character varying NULL, ADD COLUMN "totp_enabled_at" timestamptz NULL;
-- Create "recovery_codes" table
CREATE TABLE "public"."recovery_codes" (
  "id" uuid NOT NULL,
  "code_hash" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "used_at" timestamptz NULL,
  "user_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "recovery_codes_users_recoveryCodes" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "recoverycode_user_id" to table: "recovery_codes"
CREATE INDEX "recoverycode_user_id" ON "public"."recovery_codes" ("user_id");
