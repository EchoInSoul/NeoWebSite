import type { Metadata } from "next";
import Link from "next/link";

import { AuthFrame } from "@/components/auth/auth-frame";
import { SigninForm } from "./signin-form";

export const metadata: Metadata = {
  title: "登录",
};

export default function SigninPage() {
  return (
    <AuthFrame
      footer={
        <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
          没有账号？
          <Link
            href="/signup"
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            去注册
          </Link>
        </p>
      }
    >
      <div>
        <h1 className="text-xl font-semibold">欢迎回来</h1>
        <p className="mt-1.5 text-sm text-muted-foreground">登录以继续</p>
      </div>
      <div className="mt-6">
        <SigninForm />
      </div>
    </AuthFrame>
  );
}
