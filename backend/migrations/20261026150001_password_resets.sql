-- Create "password_resets" table
CREATE TABLE "public"."password_resets" (
  "id" uuid NOT NULL,
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  "user_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "password_resets_users_passwordResets" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "password_resets_token_hash_key" to table: "password_resets"
CREATE UNIQUE INDEX "password_resets_token_hash_key" ON "public"."password_resets" ("token_hash");
-- Create index "passwordreset_expires_at" to table: "password_resets"
CREATE INDEX "passwordreset_expires_at" ON "public"."password_resets" ("expires_at");
-- Create index "passwordreset_user_id" to table: "password_resets"
CREATE UNIQUE INDEX "passwordreset_user_id" ON "public"."password_resets" ("user_id");
