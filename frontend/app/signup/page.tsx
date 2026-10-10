import type { Metadata } from "next";
import Link from "next/link";

import { AuthFrame } from "@/components/auth/auth-frame";
import { SignupForm } from "./signup-form";

export const metadata: Metadata = {
  title: "创建账号",
};

export default function SignupPage() {
  return (
    <AuthFrame
      footer={
        <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
          已有账号？
          <Link
            href="/signin"
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            去登录
          </Link>
        </p>
      }
    >
      <div>
        <h1 className="text-xl font-semibold">创建账号</h1>
        <p className="mt-1.5 text-sm text-muted-foreground">注册新账号</p>
      </div>
      <div className="mt-6">
        <SignupForm />
      </div>
    </AuthFrame>
  );
}
