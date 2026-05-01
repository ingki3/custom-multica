"use client";

import { useState, useEffect, useCallback } from "react";
import { ChevronRight, Eye, EyeOff, Folder, FolderOpen, ArrowUp, Loader2, Home } from "lucide-react";
import { api } from "@multica/core/api";
import { cn } from "@multica/ui/lib/utils";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";

interface DirEntry {
  name: string;
  path: string;
}

interface FolderPickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (path: string) => void;
  initialPath?: string;
}

export function FolderPickerDialog({
  open,
  onOpenChange,
  onSelect,
  initialPath,
}: FolderPickerDialogProps) {
  const [currentPath, setCurrentPath] = useState("");
  const [parentPath, setParentPath] = useState("");
  const [entries, setEntries] = useState<DirEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);

  const loadDirectory = useCallback(async (path?: string, hidden?: boolean) => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.listDirectories(path, hidden);
      setCurrentPath(res.path);
      setParentPath(res.parent);
      setEntries(res.entries);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load directory");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (open) {
      loadDirectory(initialPath || undefined, showHidden);
    }
  }, [open, initialPath, loadDirectory, showHidden]);

  const pathSegments = currentPath.split("/").filter(Boolean);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="p-0 gap-0 flex flex-col !max-w-lg !w-full !h-[28rem]">
        <DialogTitle className="sr-only">Select Folder</DialogTitle>

        {/* Breadcrumb navigation */}
        <div className="flex items-center gap-1 px-3 py-2 border-b text-xs overflow-x-auto shrink-0">
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  className="shrink-0 rounded p-1 hover:bg-accent/60 transition-colors cursor-pointer"
                  onClick={() => loadDirectory(undefined, showHidden)}
                >
                  <Home className="size-3.5 text-muted-foreground" />
                </button>
              }
            />
            <TooltipContent side="bottom">Home</TooltipContent>
          </Tooltip>
          <button
            type="button"
            className="shrink-0 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            onClick={() => loadDirectory("/", showHidden)}
          >
            /
          </button>
          {pathSegments.map((seg, i) => {
            const segPath = "/" + pathSegments.slice(0, i + 1).join("/");
            const isLast = i === pathSegments.length - 1;
            return (
              <span key={segPath} className="flex items-center gap-1 shrink-0">
                <ChevronRight className="size-3 text-muted-foreground/50" />
                <button
                  type="button"
                  className={cn(
                    "transition-colors cursor-pointer",
                    isLast
                      ? "font-medium text-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                  onClick={() => loadDirectory(segPath, showHidden)}
                >
                  {seg}
                </button>
              </span>
            );
          })}
          <div className="ml-auto shrink-0">
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    className={cn(
                      "rounded p-1 transition-colors cursor-pointer",
                      showHidden ? "bg-accent text-foreground" : "hover:bg-accent/60 text-muted-foreground",
                    )}
                    onClick={() => {
                      const next = !showHidden;
                      setShowHidden(next);
                      loadDirectory(currentPath || undefined, next);
                    }}
                  >
                    {showHidden ? <Eye className="size-3.5" /> : <EyeOff className="size-3.5" />}
                  </button>
                }
              />
              <TooltipContent side="bottom">
                {showHidden ? "Hide hidden folders" : "Show hidden folders"}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* Directory listing */}
        <div className="flex-1 min-h-0 overflow-y-auto">
          {loading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="size-5 animate-spin text-muted-foreground" />
            </div>
          ) : error ? (
            <div className="flex flex-col items-center justify-center h-full gap-2 px-4">
              <p className="text-sm text-destructive">{error}</p>
              <Button variant="outline" size="sm" onClick={() => loadDirectory(parentPath || undefined, showHidden)}>
                Go back
              </Button>
            </div>
          ) : (
            <div className="p-1">
              {parentPath && (
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent transition-colors"
                  onClick={() => loadDirectory(parentPath, showHidden)}
                >
                  <ArrowUp className="size-4 text-muted-foreground" />
                  <span className="text-muted-foreground">..</span>
                </button>
              )}
              {entries.length === 0 && !parentPath && (
                <div className="px-2 py-6 text-center text-sm text-muted-foreground">
                  No subdirectories
                </div>
              )}
              {entries.map((entry) => (
                <button
                  key={entry.path}
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent transition-colors group"
                  onClick={() => loadDirectory(entry.path, showHidden)}
                >
                  <Folder className="size-4 text-muted-foreground group-hover:hidden" />
                  <FolderOpen className="size-4 text-muted-foreground hidden group-hover:block" />
                  <span className="truncate">{entry.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-3 py-2.5 border-t shrink-0">
          <p className="text-xs text-muted-foreground truncate mr-3">
            {currentPath}
          </p>
          <div className="flex items-center gap-2 shrink-0">
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => {
                onSelect(currentPath);
                onOpenChange(false);
              }}
              disabled={!currentPath}
            >
              Select
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
