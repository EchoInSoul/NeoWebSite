"use client";

import { useEffect, useRef } from "react";
import { Save } from "lucide-react";

import { Button } from "@/components/ui/button";

export type ConsoleFloatingSaveButtonProperties = {
  label: string;
  isDirty: boolean;
  isPending?: boolean;
};

const floatingContainerClassName =
  "pointer-events-none fixed inset-x-0 bottom-0 z-30 flex justify-end px-5 pb-5 min-[1200px]:px-7 min-[1200px]:pb-7";

export function ConsoleFloatingSaveButton({
  label,
  isDirty,
  isPending = false,
}: ConsoleFloatingSaveButtonProperties) {
  const anchorReference = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleSaveShortcut(event: KeyboardEvent) {
      if (event.repeat) {
        return;
      }

      const isSaveShortcut =
        (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s";

      if (!isSaveShortcut) {
        return;
      }

      if (
        event.target instanceof Element &&
        event.target.closest("[role=dialog], [role=alertdialog]") !== null
      ) {
        return;
      }

      const owningForm = anchorReference.current?.closest("form");

      if (owningForm === null || owningForm === undefined) {
        return;
      }

      event.preventDefault();

      if (isDirty && !isPending) {
        owningForm.requestSubmit();
      }
    }

    window.addEventListener("keydown", handleSaveShortcut);

    return () => window.removeEventListener("keydown", handleSaveShortcut);
  }, [isDirty, isPending]);

  return (
    <>
      <div ref={anchorReference} aria-hidden="true" className="h-10" />
      {isDirty ? (
        <div className={floatingContainerClassName}>
          <Button
            className="floating-save-enter pointer-events-auto shadow-lg"
            disabled={isPending}
            size="lg"
            type="submit"
          >
            <Save aria-hidden="true" />
            {label}
          </Button>
        </div>
      ) : null}
    </>
  );
}
