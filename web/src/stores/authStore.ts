import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { Employee } from '@/types/models';

interface AuthState {
  token: string | null;
  user: Employee | null;
  isAuthenticated: boolean;

  login: (token: string, user: Employee) => void;
  logout: () => void;
  setUser: (user: Employee) => void;
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
