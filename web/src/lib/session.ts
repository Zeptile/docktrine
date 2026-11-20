import { writable, derived, get } from "svelte/store";
import { api, auth, type Me, type Role } from "./api";

// Who is signed in; null before loading and when signed out
export const me = writable<Me | null>(null);

const rank: Record<Role, number> = { viewer: 1, deployer: 2, admin: 3 };

// $can("deployer") is true for deployers and admins
export const can = derived(me, ($me) => (role: Role) => !!$me && rank[$me.role] >= rank[role]);

export async function loadMe(): Promise<Me | null> {
  try {
    const m = await api.me.get();
    me.set(m);
    return m;
  } catch {
    me.set(null);
    return null;
  }
}

export async function signOut() {
  if (get(me)?.via === "session") {
    await api.auth.logout().catch(() => {});
  }
  auth.clearKey();
  me.set(null);
}

export function displayName(m: Me | null): string {
  if (!m) return "";
  if (m.user) return m.user.display_name || m.user.username;
  return m.key_name ?? "API key";
}
