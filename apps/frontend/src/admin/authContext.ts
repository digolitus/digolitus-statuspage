import { createContext, useContext } from "react";
import type { Me, PermissionKey } from "../api/types";

export interface AuthState {
  me: Me;
  /** Whether the signed-in user holds the permission (admins hold all). */
  can: (permission: PermissionKey) => boolean;
  /** Reloads the session (e.g. after the user's own roles change). */
  refresh: () => void;
  logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthState | null>(null);

/** Session of the admin area; only usable below AuthGate. */
export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthGate");
  return ctx;
}

export function useCan(permission: PermissionKey): boolean {
  return useAuth().can(permission);
}
