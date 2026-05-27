"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import type { Issue, IssueStatus } from "@multica/core/types";
import { cn } from "@multica/ui/lib/utils";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import { Button } from "@multica/ui/components/ui/button";
import { AppLink } from "../../navigation";
import { ActorAvatar } from "../../common/actor-avatar";
import { ProjectIcon } from "../../projects/components/project-icon";
import { StatusIcon } from "./status-icon";
import { PriorityIcon } from "./priority-icon";
import { sortIssues } from "../utils/sort";

const MS_PER_DAY = 24 * 60 * 60 * 1000;
function startOfDayUTC(d: Date): Date { return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())); }
function addDays(d: Date, days: number): Date { return new Date(d.getTime() + days * MS_PER_DAY); }
function daysBetween(a: Date, b: Date): number { return Math.round((b.getTime() - a.getTime()) / MS_PER_DAY); }
function parseDay(iso: string | null): Date | null { if (!iso) return null; const d = new Date(iso); if (Number.isNaN(d.getTime())) return null; return startOfDayUTC(d); }
function isWeekendUTC(d: Date): boolean { const wd = d.getUTCDay(); return wd === 0 || wd === 6; }
function isMonthStartUTC(d: Date): boolean { return d.getUTCDate() === 1; }
function isWeekStartUTC(d: Date): boolean { return d.getUTCDay() === 1; }

const ROW_HEIGHT = 36;
const HEADER_HEIGHT = 56;
const LEFT_COL_WIDTH = 320;

type GanttZoom = "day" | "week" | "month";
const DAY_PX_BY_ZOOM: Record<GanttZoom, number> = { day: 36, week: 14, month: 6 };

interface Range { start: Date; end: Date; }

function computeRange(issues: Issue[], today: Date, zoom: GanttZoom): Range {
  const defaultPad: Record<GanttZoom, number> = { day: 21, week: 60, month: 180 };
  let minTs = today.getTime() - defaultPad[zoom] * MS_PER_DAY;
  let maxTs = today.getTime() + defaultPad[zoom] * MS_PER_DAY;
  for (const i of issues) {
    const s = parseDay(i.start_date ?? null); const e = parseDay(i.due_date);
    if (s && s.getTime() < minTs) minTs = s.getTime();
    if (e && e.getTime() > maxTs) maxTs = e.getTime();
    if (s && s.getTime() > maxTs) maxTs = s.getTime();
    if (e && e.getTime() < minTs) minTs = e.getTime();
  }
  const pad = Math.max(2, Math.round(defaultPad[zoom] / 6));
  return { start: addDays(startOfDayUTC(new Date(minTs)), -pad), end: addDays(startOfDayUTC(new Date(maxTs)), pad + 1) };
}

function GanttAxis({ range, dayPx, zoom, todayOffsetDays, width }: { range: Range; dayPx: number; zoom: GanttZoom; todayOffsetDays: number; width: number }) {
  const locale = typeof navigator !== "undefined" ? navigator.language : "en";
  const totalDays = daysBetween(range.start, range.end);
  const monthBlocks = useMemo(() => {
    const out: { label: string; left: number; width: number }[] = [];
    let cursor = startOfDayUTC(range.start);
    while (cursor.getTime() < range.end.getTime()) {
      const monthEnd = new Date(Date.UTC(cursor.getUTCFullYear(), cursor.getUTCMonth() + 1, 1));
      const blockEnd = monthEnd.getTime() > range.end.getTime() ? range.end : monthEnd;
      const startDays = daysBetween(range.start, cursor);
      const widthDays = daysBetween(cursor, blockEnd);
      out.push({ label: cursor.toLocaleDateString(locale, { month: "short", year: "numeric", timeZone: "UTC" }), left: startDays * dayPx, width: widthDays * dayPx });
      cursor = monthEnd;
    }
    return out;
  }, [range, dayPx, locale]);
  return (
    <div className="relative shrink-0 border-b bg-background" style={{ height: HEADER_HEIGHT, width }}>
      <div className="relative h-7 border-b">
        {monthBlocks.map((b, i) => (<div key={i} className="absolute top-0 bottom-0 flex items-center px-2 text-xs font-medium text-foreground/80" style={{ left: b.left, width: b.width }}>{b.width > 40 && <span className="truncate">{b.label}</span>}</div>))}
      </div>
      <div className="relative h-7">
        {Array.from({ length: totalDays }, (_, i) => {
          const date = addDays(range.start, i);
          const isMonth = isMonthStartUTC(date); const isWeek = isWeekStartUTC(date);
          const showLabel = zoom === "day" || (zoom === "week" && isWeek) || (zoom === "month" && isMonth);
          return (<div key={i} className={cn("absolute top-0 bottom-0 flex items-center justify-center text-[10px] text-muted-foreground border-l", isMonth ? "border-foreground/15" : isWeek ? "border-foreground/10" : "border-foreground/5")} style={{ left: i * dayPx, width: dayPx }}>
            {showLabel && (<div className="flex flex-col items-center leading-tight">
              {zoom === "day" && (<><span className="tabular-nums">{date.getUTCDate()}</span><span className="text-[9px] opacity-70">{date.toLocaleDateString(locale, { weekday: "short", timeZone: "UTC" })}</span></>)}
              {zoom === "week" && <span className="tabular-nums">{date.getUTCDate()}</span>}
              {zoom === "month" && <span className="tabular-nums whitespace-nowrap">{date.toLocaleDateString(locale, { month: "short", day: "numeric", timeZone: "UTC" })}</span>}
            </div>)}
          </div>);
        })}
        {todayOffsetDays >= 0 && todayOffsetDays <= totalDays && <div className="absolute top-0 bottom-0 w-px bg-brand" style={{ left: todayOffsetDays * dayPx }} />}
      </div>
    </div>
  );
}

function BackgroundLayer({ range, dayPx, height, todayOffsetDays }: { range: Range; dayPx: number; height: number; todayOffsetDays: number }) {
  const totalDays = daysBetween(range.start, range.end);
  return (
    <div className="pointer-events-none absolute inset-0" style={{ height, width: totalDays * dayPx }}>
      {Array.from({ length: totalDays }, (_, i) => {
        const date = addDays(range.start, i); const weekend = isWeekendUTC(date); const isMonth = isMonthStartUTC(date); const isWeek = isWeekStartUTC(date);
        return (<div key={i} className="absolute top-0 bottom-0" style={{ left: i * dayPx, width: dayPx }}>
          {weekend && <div className="absolute inset-0 bg-muted/40" />}
          {(isMonth || isWeek) && <div className={cn("absolute top-0 bottom-0 left-0 w-px", isMonth ? "bg-foreground/10" : "bg-foreground/5")} />}
        </div>);
      })}
      {todayOffsetDays >= 0 && todayOffsetDays <= totalDays && <div className="absolute top-0 bottom-0 w-px bg-brand/70" style={{ left: todayOffsetDays * dayPx }} />}
    </div>
  );
}

const STATUS_BAR_BG: Record<IssueStatus, string> = {
  backlog: "bg-muted-foreground/60", todo: "bg-muted-foreground/70", in_progress: "bg-warning",
  in_review: "bg-success", done: "bg-info", blocked: "bg-destructive", cancelled: "bg-muted-foreground/40",
};

function ScheduledRow({ issue, range, dayPx, totalDays }: { issue: Issue; range: Range; dayPx: number; totalDays: number }) {
  const p = useWorkspacePaths();
  const wsId = useWorkspaceId();
  const { data: projects = [] } = useQuery({ ...projectListOptions(wsId), enabled: !!issue.project_id });
  const project = issue.project_id ? projects.find((pr) => pr.id === issue.project_id) : undefined;
  const start = parseDay(issue.start_date ?? null); const due = parseDay(issue.due_date);
  const inverted = start !== null && due !== null && start.getTime() > due.getTime();
  const rangeStart = start && due ? (inverted ? due : start) : (start ?? due);
  const rangeEnd = start && due ? (inverted ? start : due) : (start ?? due);
  let bar: { left: number; width: number; isMarker: boolean } | null = null;
  if (rangeStart && rangeEnd) {
    const s = Math.max(daysBetween(range.start, rangeStart), 0);
    const e = Math.min(daysBetween(range.start, rangeEnd) + 1, totalDays);
    if (e > s) { const isSingle = !start || !due; bar = isSingle ? { left: s * dayPx, width: Math.max(dayPx, 12), isMarker: true } : { left: s * dayPx, width: (e - s) * dayPx, isMarker: false }; }
  }
  const locale = typeof navigator !== "undefined" ? navigator.language : "en";
  const fmt = (d: Date) => d.toLocaleDateString(locale, { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" });
  return (
    <div className="flex border-b border-foreground/5 hover:bg-accent/30 transition-colors" style={{ height: ROW_HEIGHT }}>
      <AppLink href={p.issueDetail(issue.id)} className="sticky left-0 z-[1] flex shrink-0 items-center gap-2 border-r bg-background px-3 text-sm min-w-0" style={{ width: LEFT_COL_WIDTH }}>
        <StatusIcon status={issue.status} className="h-3.5 w-3.5" />
        <PriorityIcon priority={issue.priority} />
        <span className="w-14 shrink-0 text-xs text-muted-foreground tabular-nums truncate">{issue.identifier}</span>
        <span className="truncate flex-1">{issue.title}</span>
        {project && <ProjectIcon project={project} size="sm" />}
        {issue.assignee_type && issue.assignee_id && <ActorAvatar actorType={issue.assignee_type} actorId={issue.assignee_id} size={18} enableHoverCard />}
      </AppLink>
      <div className="relative shrink-0" style={{ width: totalDays * dayPx }}>
        {bar && (
          <Tooltip>
            <TooltipTrigger render={
              <AppLink href={p.issueDetail(issue.id)} className={cn("absolute top-1/2 -translate-y-1/2 transition-opacity hover:opacity-90", bar.isMarker ? "h-3 w-3 rotate-45 rounded-[2px]" : "h-5 rounded-md", STATUS_BAR_BG[issue.status], inverted && "ring-2 ring-destructive ring-offset-1 ring-offset-background")} style={{ left: bar.left, width: bar.width }}>
                {!bar.isMarker && bar.width > 60 && <span className="block truncate px-2 py-[2px] text-[11px] leading-4 text-white/95">{issue.title}</span>}
              </AppLink>
            } />
            <TooltipContent side="top">
              <div className="flex flex-col gap-0.5 text-xs">
                <span className="font-medium">{issue.title}</span>
                <span className="text-muted-foreground">{start ? fmt(start) : "—"} → {due ? fmt(due) : "—"}</span>
                {inverted && <span className="text-destructive">Start date is after due date</span>}
              </div>
            </TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}

export function GanttView({ issues }: { issues: Issue[] }) {
  const [zoom, setZoom] = useState<GanttZoom>("week");
  const [showCompleted, setShowCompleted] = useState(false);
  const today = useMemo(() => startOfDayUTC(new Date()), []);
  const dayPx = DAY_PX_BY_ZOOM[zoom];

  const scheduled = useMemo(() => {
    const dated = issues.filter((i) => i.start_date || i.due_date);
    const filtered = showCompleted ? dated : dated.filter((i) => i.status !== "done" && i.status !== "cancelled");
    return sortIssues(filtered, "start_date", "asc");
  }, [issues, showCompleted]);

  const range = useMemo(() => computeRange(scheduled, today, zoom), [scheduled, today, zoom]);
  const totalDays = daysBetween(range.start, range.end);
  const timelineWidth = totalDays * dayPx;
  const todayOffsetDays = daysBetween(range.start, today);

  const scrollRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollLeft = Math.max(0, LEFT_COL_WIDTH + todayOffsetDays * dayPx - 240);
  }, [todayOffsetDays, dayPx]);

  if (scheduled.length === 0) {
    return <div className="flex-1 min-h-0 flex items-center justify-center text-sm text-muted-foreground">No scheduled issues. Set start or due dates to see them here.</div>;
  }

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b px-3">
        <div className="inline-flex items-center rounded-md border border-foreground/10 p-0.5">
          {(["day", "week", "month"] as const).map((opt) => (
            <Button key={opt} size="sm" variant={zoom === opt ? "secondary" : "ghost"} className={cn("h-6 px-2 text-xs", zoom !== opt && "text-muted-foreground")} onClick={() => setZoom(opt)}>
              {opt === "day" ? "Day" : opt === "week" ? "Week" : "Month"}
            </Button>
          ))}
        </div>
        <div className="flex-1" />
        <Button size="sm" variant={showCompleted ? "secondary" : "outline"} className={cn("h-7 text-xs", !showCompleted && "text-muted-foreground")} onClick={() => setShowCompleted(!showCompleted)}>
          Show completed
        </Button>
      </div>
      <div ref={scrollRef} className="flex-1 min-h-0 overflow-auto">
        <div style={{ minWidth: LEFT_COL_WIDTH + timelineWidth }}>
          <div className="sticky top-0 z-20 flex">
            <div className="sticky left-0 z-30 shrink-0 border-b border-r bg-background" style={{ width: LEFT_COL_WIDTH, height: HEADER_HEIGHT }}>
              <div className="flex h-full items-end px-3 pb-1.5 text-[11px] font-medium text-muted-foreground">Issue</div>
            </div>
            <GanttAxis range={range} dayPx={dayPx} zoom={zoom} todayOffsetDays={todayOffsetDays} width={timelineWidth} />
          </div>
          <div className="relative">
            <div className="pointer-events-none absolute top-0" style={{ left: LEFT_COL_WIDTH, width: timelineWidth }}>
              <BackgroundLayer range={range} dayPx={dayPx} height={scheduled.length * ROW_HEIGHT} todayOffsetDays={todayOffsetDays} />
            </div>
            {scheduled.map((issue) => <ScheduledRow key={issue.id} issue={issue} range={range} dayPx={dayPx} totalDays={totalDays} />)}
          </div>
        </div>
      </div>
    </div>
  );
}
