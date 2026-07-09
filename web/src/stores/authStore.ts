import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { UserInfo } from '@/types/models';
import { useChatStore } from '@/chat/stores/chatStore';

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
        useChatStore.getState().reset();
        set({ token: null, user: null, isAuthenticated: false });
      },

      setUser: (user) => set({ user }),
    }),
    { name: 'reimbee-auth' },
  ),
);
