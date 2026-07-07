// ============================================
// HTTP 客户端 —— 基于 fetch 封装
// 自动注入 JWT token，统一错误处理
// ============================================

import { useAuthStore } from '@/stores/authStore';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

interface RequestConfig extends Omit<RequestInit, 'body'> {
  public?: boolean;
  body?: unknown;
  params?: Record<string, string | number | undefined>;
}

async function request<T = unknown>(
  method: string,
  path: string,
  config: RequestConfig = {},
): Promise<T> {
  const { public: isPublic, body, params, ...init } = config;

  const url = new URL(path, BASE_URL);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined) url.searchParams.set(k, String(v));
    });
  }

  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');

  if (!isPublic) {
    const token = useAuthStore.getState().token;
    if (token) headers.set('Authorization', `Bearer ${token}`);
  }

  const response = await fetch(url.toString(), {
    ...init,
    method,
    headers,
    body: body != null ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    if (response.status === 401) {
      useAuthStore.getState().logout();
      window.location.href = '/login';
      throw new Error('认证已过期，请重新登录');
    }
    const errorBody = await response.json().catch(() => ({}));
    throw new Error(errorBody.error ?? `请求失败 (${response.status})`);
  }

  if (response.status === 204) return undefined as T;
  return response.json();
}

export const api = {
  get: <T>(path: string, config?: RequestConfig) =>
    request<T>('GET', path, config),
  post: <T>(path: string, body?: unknown, config?: RequestConfig) =>
    request<T>('POST', path, { ...config, body }),
  put: <T>(path: string, body?: unknown, config?: RequestConfig) =>
    request<T>('PUT', path, { ...config, body }),
  delete: <T>(path: string, config?: RequestConfig) =>
    request<T>('DELETE', path, config),
};

export type { RequestConfig };
