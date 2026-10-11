-- Create "groups" table
CREATE TABLE "public"."groups" (
  "id" uuid NOT NULL,
  "system_key" character varying NULL,
  "name" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "groups_name_key" to table: "groups"
CREATE UNIQUE INDEX "groups_name_key" ON "public"."groups" ("name");
-- Create index "groups_system_key_key" to table: "groups"
CREATE UNIQUE INDEX "groups_system_key_key" ON "public"."groups" ("system_key");
-- Seed preset groups; the ids are fixed in internal/common/rbac
INSERT INTO "public"."groups" ("id", "system_key", "name", "created_at", "updated_at") VALUES
('00000000-0000-4000-8000-000000000001', 'owner', '所有者', now(), now()),
('00000000-0000-4000-8000-000000000002', 'admin', '管理员', now(), now()),
('00000000-0000-4000-8000-000000000003', 'user', '普通用户', now(), now())
ON CONFLICT ("id") DO NOTHING;
-- Create "group_permissions" table
CREATE TABLE "public"."group_permissions" (
  "id" uuid NOT NULL,
  "permission" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "group_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "group_permissions_groups_permissions" FOREIGN KEY ("group_id") REFERENCES "public"."groups" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "grouppermission_group_id_permission" to table: "group_permissions"
CREATE UNIQUE INDEX "grouppermission_group_id_permission" ON "public"."group_permissions" ("group_id", "permission");
-- Modify "users" table
ALTER TABLE "public"."users" ADD COLUMN "group_id" uuid;
UPDATE "public"."users" SET "group_id" = '00000000-0000-4000-8000-000000000003' WHERE "group_id" IS NULL;
ALTER TABLE "public"."users" ALTER COLUMN "group_id" SET NOT NULL;
ALTER TABLE "public"."users" ADD CONSTRAINT "users_groups_users" FOREIGN KEY ("group_id") REFERENCES "public"."groups" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
-- Create index "user_group_id" to table: "users"
CREATE INDEX "user_group_id" ON "public"."users" ("group_id");
-- Create "user_permissions" table
CREATE TABLE "public"."user_permissions" (
  "id" uuid NOT NULL,
  "permission" character varying NOT NULL,
  "created_at" timestamptz NOT NULL,
  "user_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "user_permissions_users_permissions" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "userpermission_user_id_permission" to table: "user_permissions"
CREATE UNIQUE INDEX "userpermission_user_id_permission" ON "public"."user_permissions" ("user_id", "permission");
