/**
 * HTTP API 客户端：与 Go 后端 REST API 通信。
 * CLI 模式使用 X-CLI-Token header 旁路认证。
 */

import type {
  Session,
  SessionListResponse,
  SessionHistoryPayload,
  LLMConfig,
  MCPConfig,
  Skill,
  Settings,
} from "../types.js";

export class APIClient {
  private baseURL: string;
  private cliToken: string;

  constructor(baseURL: string, cliToken: string) {
    this.baseURL = baseURL;
    this.cliToken = cliToken;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "X-CLI-Token": this.cliToken,
      ...(options.headers as Record<string, string> ?? {}),
    };

    const res = await fetch(url, { ...options, headers });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`API ${res.status}: ${body || res.statusText}`);
    }
    if (res.status === 204) {
      return undefined as T;
    }

    const text = await res.text();
    if (!text.trim()) {
      return undefined as T;
    }
    return JSON.parse(text) as T;
  }

  // ===== Sessions =====

  listSessions(limit = 200, offset = 0, q = ""): Promise<SessionListResponse> {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    if (offset) params.set("offset", String(offset));
    if (q) params.set("q", q);
    return this.request(`/api/sessions?${params}`);
  }

  createSession(name?: string): Promise<Session> {
    return this.request("/api/sessions", {
      method: "POST",
      body: JSON.stringify({ name: name || "New Chat" }),
    });
  }

  deleteSession(id: string): Promise<void> {
    return this.request(`/api/sessions/${id}`, { method: "DELETE" }).then(() => {});
  }

  renameSession(id: string, name: string): Promise<void> {
    return this.request(`/api/sessions/${id}/name`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    }).then(() => {});
  }

  getSessionMessages(id: string, limit = 500): Promise<SessionHistoryPayload> {
    return this.request(`/api/sessions/${id}/messages?limit=${limit}`);
  }

  // ===== Settings =====

  getSettings(): Promise<Settings> {
    return this.request("/api/settings");
  }

  updateSettings(data: Partial<Settings>): Promise<void> {
    return this.request("/api/settings", {
      method: "PUT",
      body: JSON.stringify(data),
    }).then(() => {});
  }

  // ===== LLM Configs =====

  listLLMConfigs(): Promise<LLMConfig[]> {
    return this.request("/api/llm-configs");
  }

  // ===== MCP Configs =====

  listMCPConfigs(): Promise<MCPConfig[]> {
    return this.request("/api/mcp-configs");
  }

  createMCPConfig(data: { name: string; config: string; isEnabled: boolean }): Promise<MCPConfig> {
    return this.request("/api/mcp-configs", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  updateMCPConfig(id: string, data: { name: string; config: string; isEnabled: boolean }): Promise<void> {
    return this.request(`/api/mcp-configs/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }).then(() => {});
  }

  deleteMCPConfig(id: string): Promise<void> {
    return this.request(`/api/mcp-configs/${id}`, { method: "DELETE" }).then(() => {});
  }

  // ===== Skills =====

  listSkills(): Promise<Skill[]> {
    return this.request("/api/skills");
  }

  deleteSkill(id: string): Promise<void> {
    return this.request(`/api/skills/${id}`, { method: "DELETE" }).then(() => {});
  }
}
