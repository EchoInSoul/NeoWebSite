"use client";

import { Ellipsis, UserRound } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Checkbox } from "@/components/ui/checkbox";

const CHECKBOX_CLASS =
  "data-checked:border-[oklch(57.72%_0.2324_260)] data-checked:bg-[oklch(57.72%_0.2324_260)] data-checked:text-white dark:data-checked:bg-[oklch(57.72%_0.2324_260)]";

export type CfUser = {
  id: string;
  name: string;
  email: string;
  group: string;
  status: "verified" | "not_required";
  lastActive: string;
  isSelf?: boolean;
};

const STATUS_LABEL: Record<CfUser["status"], string> = {
  verified: "已验证",
  not_required: "不需要验证",
};

export const CF_FONT_FAMILY =
  '"Open Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif';

const HEADER_CELL =
  "h-[46px] border-b border-[oklch(92.2%_0_0)] px-3 text-left align-middle text-xs font-semibold leading-4 whitespace-nowrap text-[oklch(14.5%_0_0)]";

export function CfUserTable({ users }: { users: readonly CfUser[] }) {
  return (
    <div
      className="overflow-hidden rounded-[8px] border border-[oklch(92.2%_0_0)] bg-[oklch(98%_0_0)] pt-[46px]"
      style={{ fontFamily: CF_FONT_FAMILY }}
    >
      <div className="overflow-hidden rounded-[8px] border-t border-[oklch(92.2%_0_0)] bg-white">
        <table
          aria-label="用户列表"
          className="w-full table-fixed border-separate border-spacing-0 text-left text-sm text-[#36393a]"
        >
          <colgroup>
            <col className="w-11" />
            <col />
            <col className="w-40" />
            <col className="w-[130px]" />
            <col className="w-[150px]" />
            <col className="w-[60px]" />
          </colgroup>
          <thead>
            <tr>
              <th className="h-[46px] border-b border-[oklch(92.2%_0_0)] pr-0 pl-4 text-left align-middle">
                <Checkbox aria-label="全选用户" className={CHECKBOX_CLASS} />
              </th>
              <th className={HEADER_CELL}>用户</th>
              <th className={HEADER_CELL}>用户组</th>
              <th className={HEADER_CELL}>邮箱验证</th>
              <th className={HEADER_CELL}>最近登录</th>
              <th className="h-[46px] border-b border-[oklch(92.2%_0_0)] pr-4 pl-3 text-right align-middle text-xs leading-4 font-semibold whitespace-nowrap text-[oklch(14.5%_0_0)]">
                操作
              </th>
            </tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <tr key={user.id} className="h-[59px] even:bg-[oklch(98%_0_0)]">
                <td className="pr-0 pl-4 align-middle">
                  <Checkbox
                    aria-label={`选择「${user.name}」`}
                    className={CHECKBOX_CLASS}
                  />
                </td>
                <td className="px-3 align-middle">
                  <div className="flex min-w-0 items-center gap-2.5">
                    <Avatar>
                      <AvatarFallback>
                        <UserRound aria-hidden="true" className="size-4" />
                      </AvatarFallback>
                    </Avatar>
                    <div className="flex min-w-0 flex-col gap-px">
                      <span className="flex items-center gap-1.5 text-sm leading-[18px] font-medium whitespace-nowrap text-[oklch(14.5%_0_0)]">
                        {user.name}
                        {user.isSelf ? (
                          <span className="inline-flex h-3.5 items-center rounded-[3px] bg-[oklch(97%_0_0)] px-1 text-[10px] leading-none font-medium text-[oklch(55.6%_0_0)] shadow-[inset_0_0_0_1px_oklch(91%_0_0)]">
                            你
                          </span>
                        ) : null}
                      </span>
                      <span className="truncate text-xs leading-4 text-[oklch(55.6%_0_0)]">
                        {user.email}
                      </span>
                    </div>
                  </div>
                </td>
                <td className="px-3 align-middle text-sm">{user.group}</td>
                <td className="px-3 align-middle text-[13px]">
                  <span
                    className={
                      user.status === "verified"
                        ? "text-[oklch(45%_0.13_163.225)]"
                        : "text-[oklch(55.6%_0_0)]"
                    }
                  >
                    {STATUS_LABEL[user.status]}
                  </span>
                </td>
                <td className="px-3 align-middle text-[13px] whitespace-nowrap text-[oklch(55.6%_0_0)]">
                  {user.lastActive}
                </td>
                <td className="pr-4 pl-3 text-right align-middle">
                  <button
                    type="button"
                    aria-label={`「${user.name}」的操作`}
                    className="inline-grid size-7 place-items-center rounded-[6px] text-[oklch(55.6%_0_0)] hover:bg-[oklch(97%_0_0)] hover:text-[oklch(14.5%_0_0)]"
                  >
                    <Ellipsis className="size-4" />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
