import {
  GoogleAuthProvider,
  createUserWithEmailAndPassword,
  onAuthStateChanged,
  signInWithEmailAndPassword,
  signInWithPopup,
  signOut,
  type User,
} from "firebase/auth";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { setAuthTokenProvider } from "../api/client";
import { firebaseAuth, isFirebaseConfigured } from "../lib/firebase";

const DEBUG_USER_STORAGE_KEY = "job-scorer-debug-user-email";

export type SessionUser = {
  uid: string;
  email: string;
  emailVerified: boolean;
  source: "firebase" | "debug";
};

type AuthContextValue = {
  user: SessionUser | null;
  loading: boolean;
  isFirebaseConfigured: boolean;
  loginWithGoogle: () => Promise<void>;
  loginWithEmail: (email: string, password: string) => Promise<void>;
  signupWithEmail: (email: string, password: string) => Promise<void>;
  loginDebug: (email: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

function mapFirebaseUser(user: User): SessionUser {
  return {
    uid: user.uid,
    email: user.email || "",
    emailVerified: user.emailVerified,
    source: "firebase",
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<SessionUser | null>(null);
  const [loading, setLoading] = useState(true);

  const getAccessToken = useCallback(async () => {
    if (isFirebaseConfigured && firebaseAuth?.currentUser) {
      return firebaseAuth.currentUser.getIdToken();
    }
    if (user?.source === "debug" && user.email) {
      return `dev:${user.email}`;
    }
    return null;
  }, [user]);

  useEffect(() => {
    setAuthTokenProvider(getAccessToken);
  }, [getAccessToken]);

  useEffect(() => {
    if (isFirebaseConfigured && firebaseAuth) {
      const unsubscribe = onAuthStateChanged(firebaseAuth, (currentUser) => {
        if (!currentUser) {
          setUser(null);
          setLoading(false);
          return;
        }
        setUser(mapFirebaseUser(currentUser));
        setLoading(false);
      });
      return () => unsubscribe();
    }

    const storedEmail = window.localStorage.getItem(DEBUG_USER_STORAGE_KEY);
    if (storedEmail) {
      setUser({
        uid: "debug-user",
        email: storedEmail,
        emailVerified: true,
        source: "debug",
      });
    } else {
      setUser(null);
    }
    setLoading(false);
    return () => undefined;
  }, []);

  const loginWithGoogle = useCallback(async () => {
    if (!firebaseAuth || !isFirebaseConfigured) {
      throw new Error("Firebase auth is not configured in this environment.");
    }
    const provider = new GoogleAuthProvider();
    const result = await signInWithPopup(firebaseAuth, provider);
    setUser(mapFirebaseUser(result.user));
  }, []);

  const loginWithEmail = useCallback(async (email: string, password: string) => {
    if (!firebaseAuth || !isFirebaseConfigured) {
      throw new Error("Firebase auth is not configured in this environment.");
    }
    const result = await signInWithEmailAndPassword(firebaseAuth, email, password);
    setUser(mapFirebaseUser(result.user));
  }, []);

  const signupWithEmail = useCallback(async (email: string, password: string) => {
    if (!firebaseAuth || !isFirebaseConfigured) {
      throw new Error("Firebase auth is not configured in this environment.");
    }
    const result = await createUserWithEmailAndPassword(firebaseAuth, email, password);
    setUser(mapFirebaseUser(result.user));
  }, []);

  const loginDebug = useCallback(async (email: string) => {
    const normalized = email.trim().toLowerCase();
    if (!normalized) {
      throw new Error("Email is required.");
    }
    window.localStorage.setItem(DEBUG_USER_STORAGE_KEY, normalized);
    setUser({
      uid: "debug-user",
      email: normalized,
      emailVerified: true,
      source: "debug",
    });
  }, []);

  const logout = useCallback(async () => {
    if (isFirebaseConfigured && firebaseAuth) {
      await signOut(firebaseAuth);
    }
    window.localStorage.removeItem(DEBUG_USER_STORAGE_KEY);
    setUser(null);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      loading,
      isFirebaseConfigured,
      loginWithGoogle,
      loginWithEmail,
      signupWithEmail,
      loginDebug,
      logout,
    }),
    [loading, loginDebug, loginWithEmail, loginWithGoogle, logout, signupWithEmail, user]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}
