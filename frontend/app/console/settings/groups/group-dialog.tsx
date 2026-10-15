"use client";

import { Fragment, useState } from "react";
import { useTranslations } from "next-intl";
import { cn } from "cn";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  actionLabelKey,
  applyPermissionToggle,
  buildMatrixSections,
  permissionColumnState,
  resourceLabelKey,
  samePermissionSet,
  togglePermissionColumn,
  type AdminGroup,
  type PermissionMatrix,
  type PermissionMatrixCell,
} from "@/lib/groups";

type GroupDialogProperties = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group: AdminGroup | null;
  matrix: PermissionMatrix;
  onSubmit: (input: {
    name: string;
    permissions: string[];
  }) => Promise<boolean>;
};

type Baseline = { name: string; permissions: string[] };

export function GroupDialog({
  open,
  onOpenChange,
  group,
  matrix,
  onSubmit,
}: GroupDialogProperties) {
  const t = useTranslations("groups");
  const ts = useTranslations("sidebar");
  const tCommon = useTranslations("common");
  const initialName = group?.name ?? "";
  const initialPermissions = group?.permissions ?? [];
  const [name, setName] = useState(initialName);
  const [selected, setSelected] = useState<string[]>(() => [
    ...initialPermissions,
  ]);
  const [baseline] = useState<Baseline>(() => ({
    name: initialName,
    permissions: [...initialPermissions],
  }));
  const [isDiscardOpen, setIsDiscardOpen] = useState(false);
  const [isPending, setIsPending] = useState(false);

  const isReadOnly = group?.isPreset ?? false;
  const isDirty =
    name.trim() !== baseline.name.trim() ||
    !samePermissionSet(selected, baseline.permissions);

  const sections = buildMatrixSections(matrix);
  const visibleRows = sections.flatMap((section) => section.rows);

  function columnKeys(action: string): string[] {
    const index = matrix.actions.indexOf(action);
    return visibleRows
      .map((row) => row.cells[index])
      .filter(
        (cell): cell is NonNullable<PermissionMatrixCell> => cell !== null,
      )
      .map((cell) => cell.key);
  }

  function actionText(action: string): string {
    const labelKey = actionLabelKey(action);
    return labelKey === null ? action : t(labelKey);
  }

  function resourceText(resource: string): string {
    const labelKey = resourceLabelKey(resource);
    return labelKey === null ? resource : t(labelKey);
  }

  async function handleSave() {
    setIsPending(true);
    try {
      const succeeded = await onSubmit({ name, permissions: selected });
      if (succeeded) {
        onOpenChange(false);
      }
    } finally {
      setIsPending(false);
    }
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(nextOpen, eventDetails) => {
          if (!nextOpen && (isPending || isDirty)) {
            eventDetails.cancel();
            if (!isPending) {
              setIsDiscardOpen(true);
            }
            return;
          }
          onOpenChange(nextOpen);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {group === null ? t("dialogCreateTitle") : t("dialogEditTitle")}
            </DialogTitle>
          </DialogHeader>
          <div className="flex items-center gap-3">
            <Label
              htmlFor="group-name"
              className="shrink-0 text-muted-foreground"
            >
              {t("nameLabel")}
            </Label>
            <Input
              id="group-name"
              className="flex-1"
              value={name}
              maxLength={64}
              placeholder={t("namePlaceholder")}
              disabled={isReadOnly || isPending}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div className="text-sm font-medium">{t("permissionsLabel")}</div>
          <div
            className="grid items-center gap-x-2 gap-y-1.5"
            style={{
              gridTemplateColumns: `minmax(0, 1fr) repeat(${matrix.actions.length}, 3.5rem)`,
            }}
          >
            <span aria-hidden="true" />
            {matrix.actions.map((action) => {
              const keys = columnKeys(action);
              const state = permissionColumnState(selected, keys);

              return (
                <div
                  key={action}
                  className="flex flex-col items-center gap-1 text-xs text-muted-foreground"
                >
                  <span>{actionText(action)}</span>
                  {isReadOnly ? null : (
                    <Checkbox
                      checked={state === "all"}
                      indeterminate={state === "some"}
                      disabled={isPending || keys.length === 0}
                      aria-label={t("selectAllColumn", {
                        action: actionText(action),
                      })}
                      onCheckedChange={(checked) =>
                        setSelected((previous) =>
                          togglePermissionColumn(previous, keys, checked),
                        )
                      }
                    />
                  )}
                </div>
              );
            })}
            {sections.map((section, sectionIndex) => (
              <Fragment key={section.labelKey}>
                <span
                  className={cn(
                    "col-span-full text-xs font-medium text-muted-foreground",
                    sectionIndex === 0 ? "pt-2" : "pt-4",
                  )}
                >
                  {ts(section.labelKey)}
                </span>
                {section.rows.map((row) => (
                  <Fragment key={row.resource}>
                    <span className="py-0.5 text-sm">
                      {resourceText(row.resource)}
                    </span>
                    {row.cells.map((cell, index) =>
                      cell === null ? (
                        <span
                          key={`${row.resource}-${matrix.actions[index]}`}
                        />
                      ) : (
                        <span
                          key={cell.key}
                          className="flex items-center justify-center"
                        >
                          <Checkbox
                            checked={selected.includes(cell.key)}
                            disabled={isReadOnly || isPending}
                            aria-label={`${resourceText(row.resource)} ${actionText(cell.action)}`}
                            onCheckedChange={(checked) =>
                              setSelected((previous) =>
                                applyPermissionToggle(
                                  previous,
                                  cell.key,
                                  checked,
                                ),
                              )
                            }
                          />
                        </span>
                      ),
                    )}
                  </Fragment>
                ))}
              </Fragment>
            ))}
          </div>
          {isReadOnly ? null : (
            <p className="text-xs text-muted-foreground">
              {t("hintEditImpliesView")}
            </p>
          )}
          <DialogFooter>
            {isReadOnly ? (
              <DialogClose render={<Button type="button" variant="outline" />}>
                {t("close")}
              </DialogClose>
            ) : (
              <>
                <DialogClose
                  render={<Button type="button" variant="outline" />}
                >
                  {tCommon("cancel")}
                </DialogClose>
                <Button
                  type="button"
                  disabled={isPending || name.trim() === "" || !isDirty}
                  onClick={() => void handleSave()}
                >
                  {t("save")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={isDiscardOpen} onOpenChange={setIsDiscardOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("discardTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("discardDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{tCommon("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                setIsDiscardOpen(false);
                onOpenChange(false);
              }}
            >
              {t("discard")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
