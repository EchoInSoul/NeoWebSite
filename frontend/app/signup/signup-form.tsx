"use client";

import { useId, useRef, useState, type FormEvent } from "react";
import { Eye, EyeOff } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";

const PASSWORD_MIN_LENGTH = 8;

export function SignupForm() {
  const emailFieldId = useId();
  const nameFieldId = useId();
  const passwordFieldId = useId();
  const confirmFieldId = useId();
  const mismatchId = useId();

  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isPasswordVisible, setIsPasswordVisible] = useState(false);
  const [isConfirmVisible, setIsConfirmVisible] = useState(false);
  const confirmInputRef = useRef<HTMLInputElement>(null);

  const hasMismatch =
    confirmPassword.length > 0 && confirmPassword !== password;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (hasMismatch) {
      confirmInputRef.current?.focus();
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={handleSubmit} noValidate>
      <fieldset className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor={emailFieldId}>邮箱</Label>
          <Input
            id={emailFieldId}
            name="email"
            type="email"
            autoComplete="email"
            required
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={nameFieldId}>用户名</Label>
          <Input
            id={nameFieldId}
            name="name"
            type="text"
            autoComplete="username"
            required
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={passwordFieldId}>密码</Label>
          <InputGroup>
            <InputGroupInput
              id={passwordFieldId}
              name="password"
              type={isPasswordVisible ? "text" : "password"}
              autoComplete="new-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                aria-label={isPasswordVisible ? "隐藏密码" : "显示密码"}
                onClick={() => setIsPasswordVisible((visible) => !visible)}
              >
                {isPasswordVisible ? <EyeOff /> : <Eye />}
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          <p className="text-xs text-muted-foreground">
            密码长度至少 {PASSWORD_MIN_LENGTH} 位，须包含：字母/数字
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={confirmFieldId}>确认密码</Label>
          <InputGroup>
            <InputGroupInput
              id={confirmFieldId}
              ref={confirmInputRef}
              name="confirmPassword"
              type={isConfirmVisible ? "text" : "password"}
              autoComplete="new-password"
              required
              aria-invalid={hasMismatch}
              aria-describedby={hasMismatch ? mismatchId : undefined}
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                aria-label={isConfirmVisible ? "隐藏密码" : "显示密码"}
                onClick={() => setIsConfirmVisible((visible) => !visible)}
              >
                {isConfirmVisible ? <EyeOff /> : <Eye />}
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          {hasMismatch ? (
            <p
              className="text-xs font-medium text-destructive"
              id={mismatchId}
              role="alert"
            >
              两次输入的密码不一致
            </p>
          ) : null}
        </div>
      </fieldset>

      <Button type="submit" className="w-full">
        注册
      </Button>
    </form>
  );
}
