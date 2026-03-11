import type { ReactNode } from "react";
import { Link, useLocation } from "react-router-dom";
import { useAuth } from "../../auth/AuthProvider";

type AppShellProps = {
  children: ReactNode;
};

const navItems = [
  { path: "/", label: "Dashboard", description: "Overview and analytics" },
  { path: "/runs", label: "Runs", description: "Trigger and inspect pipeline runs" },
  { path: "/jobs", label: "Jobs", description: "Explore stage outputs" },
  { path: "/settings", label: "Settings", description: "Policy, CV, schedule, notifications" },
  { path: "/billing", label: "Billing", description: "Credits and checkout packages" },
];

export function AppShell({ children }: AppShellProps) {
  const location = useLocation();
  const { user, logout } = useAuth();

  return (
    <div className="min-h-screen bg-transparent text-slate-200">
      <div className="mx-auto grid min-h-screen max-w-[1600px] lg:grid-cols-[260px_1fr]">
        <aside className="border-r border-white/10 bg-slate-950/55 px-5 py-6 backdrop-blur xl:px-6">
          <div className="mb-8">
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-300">Job Scorer</p>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight text-white">Internal command center</h1>
            <p className="mt-2 text-sm leading-6 text-slate-400">
              Run searches, inspect every pipeline stage, and understand what made it to email.
            </p>
          </div>

          <nav className="space-y-2">
            {navItems.map((item) => {
              const active =
                location.pathname === item.path ||
                (item.path !== "/" && location.pathname.startsWith(item.path));

              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={`block rounded-2xl border px-4 py-3 transition ${
                    active
                      ? "border-cyan-400/30 bg-cyan-400/10 text-white"
                      : "border-transparent bg-white/3 text-slate-300 hover:border-white/10 hover:bg-white/6"
                  }`}
                >
                  <p className="text-sm font-medium">{item.label}</p>
                  <p className="mt-1 text-xs leading-5 text-slate-400">{item.description}</p>
                </Link>
              );
            })}
          </nav>

          <div className="mt-8 rounded-2xl border border-white/10 bg-white/4 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Workspace</p>
            <p className="mt-2 text-sm text-white">Production tenant</p>
            <p className="mt-1 text-xs leading-5 text-slate-500">
              Isolated account settings, pipeline history, and billing controls.
            </p>
          </div>
        </aside>

        <div className="min-w-0">
          <header className="sticky top-0 z-20 border-b border-white/10 bg-slate-950/45 px-6 py-4 backdrop-blur xl:px-10">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Pipeline analytics</p>
                <p className="mt-1 text-sm text-slate-400">Monitor scraping, filtering, evaluation, validation, and notifications in one place.</p>
              </div>
              <div className="flex items-center gap-3">
                <div className="inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-xs font-medium text-emerald-300">
                  Service ready
                </div>
                <div className="rounded-full border border-white/10 bg-white/3 px-3 py-1 text-xs text-slate-300">
                  {user?.email ?? "unknown user"}
                </div>
                <button
                  type="button"
                  onClick={() => void logout()}
                  className="rounded-full border border-white/15 bg-white/3 px-3 py-1 text-xs text-slate-300 transition hover:bg-white/8"
                >
                  Sign out
                </button>
              </div>
            </div>
          </header>

          <main className="px-6 py-8 xl:px-10">{children}</main>
        </div>
      </div>
    </div>
  );
}
