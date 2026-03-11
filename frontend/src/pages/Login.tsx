import { useState } from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../auth/AuthProvider";
import { Surface } from "../components/ui/Surface";

export default function Login() {
  const {
    user,
    loading,
    isFirebaseConfigured,
    loginWithEmail,
    signupWithEmail,
    loginWithGoogle,
    loginDebug,
  } = useAuth();

  const [mode, setMode] = useState<"signin" | "signup">("signin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (loading) {
    return <div className="flex min-h-screen items-center justify-center text-slate-300">Loading auth session...</div>;
  }

  if (user) {
    return <Navigate to="/" replace />;
  }

  const submitEmail = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      if (isFirebaseConfigured) {
        if (mode === "signin") {
          await loginWithEmail(email, password);
        } else {
          await signupWithEmail(email, password);
        }
      } else {
        await loginDebug(email);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  };

  const submitGoogle = async () => {
    setPending(true);
    setError(null);
    try {
      await loginWithGoogle();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-6xl items-center justify-center px-6 py-10">
      <Surface className="w-full max-w-lg p-8">
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-300">Job Scorer</p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight text-white">
          {isFirebaseConfigured ? "Sign in to your workspace" : "Debug login"}
        </h1>
        <p className="mt-2 text-sm text-slate-400">
          {isFirebaseConfigured
            ? "Use your account to manage personal settings, schedules, CV uploads, and run history."
            : "Firebase env vars are missing. Enter an email to authenticate through local debug mode."}
        </p>

        {isFirebaseConfigured ? (
          <div className="mt-6 flex gap-2 rounded-xl bg-white/[0.03] p-1">
            <button
              type="button"
              onClick={() => setMode("signin")}
              className={`flex-1 rounded-lg px-3 py-2 text-sm ${
                mode === "signin" ? "bg-cyan-400 text-slate-950" : "text-slate-300 hover:bg-white/[0.05]"
              }`}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => setMode("signup")}
              className={`flex-1 rounded-lg px-3 py-2 text-sm ${
                mode === "signup" ? "bg-cyan-400 text-slate-950" : "text-slate-300 hover:bg-white/[0.05]"
              }`}
            >
              Sign up
            </button>
          </div>
        ) : null}

        <form onSubmit={submitEmail} className="mt-6 space-y-4">
          <label className="block space-y-2">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Email</span>
            <input
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              type="email"
              required
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none placeholder:text-slate-500"
              placeholder="you@example.com"
            />
          </label>
          {isFirebaseConfigured ? (
            <label className="block space-y-2">
              <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Password</span>
              <input
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                type="password"
                required
                minLength={6}
                className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none placeholder:text-slate-500"
                placeholder="••••••••"
              />
            </label>
          ) : null}

          {error ? <p className="text-sm text-rose-300">{error}</p> : null}

          <button
            type="submit"
            disabled={pending}
            className="w-full rounded-xl bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-cyan-300 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400"
          >
            {pending
              ? "Please wait..."
              : isFirebaseConfigured
              ? mode === "signin"
                ? "Sign in with email"
                : "Create account"
              : "Continue in debug mode"}
          </button>
        </form>

        {isFirebaseConfigured ? (
          <>
            <div className="my-6 h-px bg-white/10" />
            <button
              type="button"
              disabled={pending}
              onClick={submitGoogle}
              className="w-full rounded-xl border border-white/15 bg-white/[0.04] px-4 py-2 text-sm font-medium text-slate-200 transition hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:text-slate-500"
            >
              Continue with Google
            </button>
          </>
        ) : null}
      </Surface>
    </div>
  );
}
