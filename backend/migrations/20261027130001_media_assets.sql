-- Create "media_assets" table
CREATE TABLE "public"."media_assets" (
  "id" uuid NOT NULL,
  "provider" character varying NOT NULL,
  "key" character varying NOT NULL,
  "size_bytes" bigint NOT NULL,
  "original_name" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "uploaded_by" uuid NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "media_assets_users_mediaAssets" FOREIGN KEY ("uploaded_by") REFERENCES "public"."users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- Create index "media_assets_key_key" to table: "media_assets"
CREATE UNIQUE INDEX "media_assets_key_key" ON "public"."media_assets" ("key");
-- Create index "mediaasset_uploaded_by" to table: "media_assets"
CREATE INDEX "mediaasset_uploaded_by" ON "public"."media_assets" ("uploaded_by");
