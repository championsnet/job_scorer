import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { ErrorState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { StatCard } from "../components/ui/StatCard";
import { Surface } from "../components/ui/Surface";

export default function Billing() {
  const summaryQuery = useQuery({
    queryKey: ["billing", "summary"],
    queryFn: api.getBillingSummary,
  });

  const checkoutMutation = useMutation({
    mutationFn: (packageId: string) => api.createCheckout(packageId),
    onSuccess: (result) => {
      window.location.href = result.url;
    },
  });

  if (summaryQuery.isLoading) {
    return <LoadingState title="Loading billing" description="Fetching credit balance and packages." />;
  }
  if (summaryQuery.error || !summaryQuery.data) {
    return <ErrorState title="Billing failed to load" description={String(summaryQuery.error)} />;
  }

  const summary = summaryQuery.data;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Billing"
        title="Credits"
        description="Each run consumes credits. Add credits to keep scheduled and manual runs active."
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <StatCard label="Current balance" value={summary.balance} tone="emerald" helper="Credits available for future runs" />
        <StatCard label="Cost per run" value={1} tone="cyan" helper="Configured on the backend (RUN_CREDIT_COST)" />
        <StatCard
          label="Available packages"
          value={summary.packages.length}
          tone="violet"
          helper="Stripe checkout packages currently configured"
        />
      </div>

      <Surface className="p-5">
        <h2 className="text-lg font-semibold text-white">Buy credits</h2>
        <p className="mt-1 text-sm text-slate-400">Choose a package to open Stripe Checkout.</p>
        <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {summary.packages.map((pkg) => (
            <div key={pkg.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
              <h3 className="text-base font-semibold text-white">{pkg.name}</h3>
              <p className="mt-1 text-sm text-slate-400">{pkg.description || "Credit package"}</p>
              <p className="mt-4 text-3xl font-semibold tracking-tight text-cyan-200">{pkg.credits}</p>
              <p className="mt-1 text-xs uppercase tracking-[0.18em] text-slate-500">credits</p>
              <button
                type="button"
                onClick={() => checkoutMutation.mutate(pkg.id)}
                disabled={checkoutMutation.isPending}
                className="mt-4 w-full rounded-xl bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-cyan-300 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400"
              >
                {checkoutMutation.isPending ? "Starting checkout..." : "Buy package"}
              </button>
            </div>
          ))}
        </div>
        {checkoutMutation.error ? <p className="mt-4 text-sm text-rose-300">{String(checkoutMutation.error)}</p> : null}
      </Surface>
    </div>
  );
}
