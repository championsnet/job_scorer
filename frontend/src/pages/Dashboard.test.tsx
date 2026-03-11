import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import Dashboard from "./Dashboard";
import { api } from "../api/client";

vi.mock("../api/client", () => ({
  api: {
    getAnalyticsOverview: vi.fn(),
  },
}));

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  BarChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Bar: () => null,
  CartesianGrid: () => null,
  Cell: () => null,
  Pie: () => null,
  PieChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Tooltip: () => null,
  XAxis: () => null,
  YAxis: () => null,
}));

function renderWithProviders() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("Dashboard", () => {
  it("renders a loading state", () => {
    vi.mocked(api.getAnalyticsOverview).mockImplementation(() => new Promise(() => {}));

    renderWithProviders();

    expect(screen.getByText(/loading dashboard/i)).toBeInTheDocument();
  });

  it("renders analytics content", async () => {
    vi.mocked(api.getAnalyticsOverview).mockResolvedValue({
      runs: { total: 5, success: 4, failed: 1, avg_duration_ms: 60000 },
      funnel: {
        all_jobs: 500,
        prefiltered: 200,
        evaluated: 120,
        promising: 40,
        final_evaluated: 20,
        notification: 18,
        validated: 14,
        email_sent: 10,
      },
      llm_usage: { calls: 120, input_tokens: 5000, output_tokens: 1000 },
      recent_runs: [
        {
          run_id: "run-123",
          status: "success",
          started_at: "2026-03-10T10:00:00Z",
          completed_at: "2026-03-10T10:01:00Z",
          duration_ms: 60000,
          stage_counts: {
            all_jobs: 100,
            prefiltered: 50,
            evaluated: 40,
            promising: 10,
            final_evaluated: 5,
            notification: 3,
            validated: 2,
            email_sent: 1,
          },
          config: {
            locations: ["Athens"],
            max_jobs: 100,
          },
          llm_usage: {
            calls: 12,
            input_tokens: 500,
            cached_input_tokens: 100,
            non_cached_input_tokens: 400,
            billable_input_tokens: 400,
            output_tokens: 100,
            total_tokens: 600,
          },
        },
      ],
    });

    renderWithProviders();

    expect(await screen.findByText(/pipeline dashboard/i)).toBeInTheDocument();
    expect(screen.getByText(/open latest run/i)).toBeInTheDocument();
    expect(screen.getAllByText("run-123")).toHaveLength(2);
  });
});
