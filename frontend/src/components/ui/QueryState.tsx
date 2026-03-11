import type { ReactNode } from "react";
import { Surface } from "./Surface";

type StateProps = {
  title: string;
  description?: ReactNode;
};

export function LoadingState({ title, description }: StateProps) {
  return (
    <Surface className="p-12 text-center">
      <div className="mx-auto mb-4 h-10 w-10 animate-pulse rounded-full bg-cyan-400/20" />
      <h2 className="text-lg font-semibold text-white">{title}</h2>
      {description ? <p className="mt-2 text-sm text-slate-400">{description}</p> : null}
    </Surface>
  );
}

export function ErrorState({ title, description }: StateProps) {
  return (
    <Surface className="border-rose-500/20 bg-rose-950/20 p-12 text-center">
      <h2 className="text-lg font-semibold text-rose-200">{title}</h2>
      {description ? <p className="mt-2 text-sm text-rose-200/80">{description}</p> : null}
    </Surface>
  );
}

export function EmptyState({ title, description }: StateProps) {
  return (
    <Surface className="p-12 text-center">
      <h2 className="text-lg font-semibold text-white">{title}</h2>
      {description ? <p className="mt-2 text-sm text-slate-400">{description}</p> : null}
    </Surface>
  );
}
