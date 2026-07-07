import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { UserInfo } from '@/types/models';

interface AuthState {
  token: string | null;
  user: UserInfo | null;
  isAuthenticated: boolean;

  login: (token: string, user: UserInfo) => void;
  logout: () => void;
  setUser: (user: UserInfo) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,

      login: (token, user) =>
        set({ token, user, isAuthenticated: true }),

      logout: () => {
        localStorage.clear();
        set({ token: null, user: null, isAuthenticated: false });
      },

      setUser: (user) => set({ user }),
    }),
    { name: 'reimbee-auth' },
  ),
);
