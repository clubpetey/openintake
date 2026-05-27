// HTTP DTOs — mirror of relay/internal/server package shapes (README §6.4).
// This file is the single source of truth for the client↔relay TS contract.
// Frozen in sub-plan 1-v. Do NOT alter without re-smoking 1-vi.

export interface Viewport {
  w: number;
  h: number;
}

export interface ClientInfo {
  widget_version: string;
  url: string;
  referrer: string | null;
  user_agent: string;
  viewport: Viewport;
  locale: string;
}

export interface ContextInfo {
  app_context: Record<string, unknown>;
  page_metadata: Record<string, unknown>;
}

export interface InitResponse {
  session_id: string;
  capabilities: {
    auth_modes: string[];
    streaming: boolean;
  };
}

export interface TurnMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface TurnRequest {
  messages: TurnMessage[];
}

// SSE frame shapes (data: <json>\n\n)
export interface SSEDelta {
  delta: string;
}

export interface SSEDone {
  done: true;
  input_tokens: number;
  output_tokens: number;
}

export interface SSEError {
  error: string;
}

export type SSEFrame = SSEDelta | SSEDone | SSEError;

export interface SubmitRequest {
  messages: TurnMessage[];
  client: ClientInfo;
  user_claims: Record<string, unknown>;
  context: ContextInfo;
  routing_hint: string | null;
}

export interface SubmitResponse {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}

export interface ErrorEnvelope {
  error: {
    code: string;
    message: string;
  };
}
