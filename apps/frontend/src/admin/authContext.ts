import { createContext, useContext } from "react";
import type { Me, PermissionKey } from "../api/types";

export interface AuthState {
  me: Me;
  /** Whether the signed-in user holds the permission (admins hold all). */
  can: (permission: PermissionKey) => boolean;
  /**
   * Whether the permission applies to a resource owned by `ownerTeamId`.
   * Global grants cover everything; team grants (team_permissions) only
   * cover resources owned by one of the user's teams.
   */
  canFor: (permission: PermissionKey, ownerTeamId: string | null | undefined) => boolean;
  /** Whether the permission is held only for the user's teams. */
  teamOnly: (permission: PermissionKey) => boolean;
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
