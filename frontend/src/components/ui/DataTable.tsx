import type { ReactNode } from "react";
import { Surface } from "./Surface";

type DataTableProps = {
  title?: string;
  description?: ReactNode;
  toolbar?: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
};

export function DataTable({ title, description, toolbar, summary, children }: DataTableProps) {
  return (
    <Surface className="overflow-hidden">
      {(title || toolbar) && (
        <div className="flex flex-col gap-4 border-b border-white/10 px-5 py-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            {title ? <h2 className="text-lg font-semibold text-white">{title}</h2> : null}
            {description ? <div className="mt-1 text-sm text-slate-400">{description}</div> : null}
          </div>
          {toolbar ? <div className="flex flex-wrap items-center gap-3">{toolbar}</div> : null}
        </div>
      )}
      <div className="overflow-x-auto">{children}</div>
      {summary ? <div className="border-t border-white/10 px-5 py-3 text-sm text-slate-500">{summary}</div> : null}
    </Surface>
  );
}
