"use client";

import { useTheme } from "next-themes";
import { Toaster as Sonner, type ToasterProps } from "sonner";
import {
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
  OctagonXIcon,
  Loader2Icon,
} from "lucide-react";

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme();

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="mt-[3.5px] size-4" />,
        warning: <TriangleAlertIcon className="mt-[3.5px] size-4" />,
        error: <OctagonXIcon className="mt-[3.5px] size-4" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
          "--width": "360px",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "cn-toast",
        },
        style: {
          width: "max-content",
          maxWidth: "360px",
          padding: "12px 14px",
          fontSize: "13px",
          gap: "6px",
          alignItems: "flex-start",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
