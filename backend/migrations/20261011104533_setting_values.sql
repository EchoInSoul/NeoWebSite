-- Create "setting_values" table
CREATE TABLE "public"."setting_values" (
  "id" uuid NOT NULL,
  "setting_group" character varying NOT NULL,
  "name" character varying NOT NULL,
  "value" jsonb NOT NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "settingvalue_setting_group_name" to table: "setting_values"
CREATE UNIQUE INDEX "settingvalue_setting_group_name" ON "public"."setting_values" ("setting_group", "name");
