import { create } from "zustand";

interface NotificationStore {
	count: number;
	increment: () => void;
	decrement: (by?: number) => void;
	clear: () => void;
	setCount: (n: number) => void;
}

export const useNotificationStore = create<NotificationStore>((set) => ({
	count: 0,
	increment: () => set((s) => ({ count: s.count + 1 })),
	decrement: (by = 1) => set((s) => ({ count: Math.max(0, s.count - by) })),
	clear: () => set({ count: 0 }),
	setCount: (n) => set({ count: n }),
}));
