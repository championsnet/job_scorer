type StatusBadgeProps = {
  status: "success" | "failed" | "running" | "queued";
  className?: string;
};

const statusStyles: Record<StatusBadgeProps["status"], string> = {
  success: "bg-emerald-500/15 text-emerald-300 ring-1 ring-emerald-500/25",
  failed: "bg-rose-500/15 text-rose-300 ring-1 ring-rose-500/25",
  running: "bg-amber-500/15 text-amber-300 ring-1 ring-amber-500/25",
  queued: "bg-sky-500/15 text-sky-300 ring-1 ring-sky-500/25",
};

export function StatusBadge({ status, className = "" }: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold capitalize ${statusStyles[status]} ${className}`}
    >
      {status}
    </span>
  );
}
