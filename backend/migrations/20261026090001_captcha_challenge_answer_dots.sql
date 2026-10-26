-- 存量行的答案列形状已变，全部作废（一次性、分钟级过期的挑战数据）
DELETE FROM "public"."captcha_challenges";

-- Modify "captcha_challenges" table
ALTER TABLE "public"."captcha_challenges" DROP COLUMN "answer_x", DROP COLUMN "answer_y", ADD COLUMN "answer_dots" jsonb NOT NULL, ADD COLUMN "prompt_text" character varying NOT NULL DEFAULT '';
