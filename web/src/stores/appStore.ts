import { create } from 'zustand';

interface AppState {
  sidebarCollapsed: boolean;
  darkMode: boolean;

  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleDarkMode: () => void;
}

export const useAppStore = create<AppState>()((set) => ({
  sidebarCollapsed: false,
  darkMode: false,

  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
  toggleDarkMode: () => set((s) => ({ darkMode: !s.darkMode })),
}));
