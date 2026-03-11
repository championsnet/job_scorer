import type { ReactNode } from "react";
import { Surface } from "./Surface";

type StatCardProps = {
  label: string;
  value: ReactNode;
  helper?: ReactNode;
  tone?: "default" | "cyan" | "emerald" | "violet" | "amber" | "rose";
};

const toneStyles = {
  default: "text-slate-100",
  cyan: "text-cyan-300",
  emerald: "text-emerald-300",
  violet: "text-violet-300",
  amber: "text-amber-300",
  rose: "text-rose-300",
};

export function StatCard({ label, value, helper, tone = "default" }: StatCardProps) {
  return (
    <Surface className="p-5">
      <p className="text-sm text-slate-400">{label}</p>
      <p className={`mt-3 text-3xl font-semibold tracking-tight ${toneStyles[tone]}`}>{value}</p>
      {helper ? <p className="mt-2 text-sm text-slate-500">{helper}</p> : null}
    </Surface>
  );
}
