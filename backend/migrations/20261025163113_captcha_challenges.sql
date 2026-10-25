-- Create "captcha_challenges" table
CREATE TABLE "public"."captcha_challenges" (
  "id" uuid NOT NULL,
  "scope" character varying NOT NULL,
  "answer_x" bigint NOT NULL,
  "answer_y" bigint NOT NULL,
  "status" bigint NOT NULL DEFAULT 0,
  "token_hash" character varying NULL,
  "created_at" timestamptz NOT NULL,
  "expires_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "captcha_challenges_token_hash_key" to table: "captcha_challenges"
CREATE UNIQUE INDEX "captcha_challenges_token_hash_key" ON "public"."captcha_challenges" ("token_hash");
-- Create index "captchachallenge_expires_at" to table: "captcha_challenges"
CREATE INDEX "captchachallenge_expires_at" ON "public"."captcha_challenges" ("expires_at");
