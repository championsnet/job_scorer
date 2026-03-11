import type { ReactNode } from "react";

type SurfaceProps = {
  children: ReactNode;
  className?: string;
};

export function Surface({ children, className = "" }: SurfaceProps) {
  return (
    <section
      className={`rounded-2xl border border-white/10 bg-slate-900/70 shadow-[0_12px_48px_rgba(2,6,23,0.35)] backdrop-blur ${className}`}
    >
      {children}
    </section>
  );
}
