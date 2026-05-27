// Public-facing types for IntakeClient (extracted to avoid circular imports).
// Frozen in sub-plan 1-v. Do NOT alter without re-smoking 1-vi.

export interface IntakeConfig {
  /** Base URL of the relay, e.g. "http://localhost:8080". No trailing slash. */
  relayUrl: string;
  /** Semver string of the widget embedding this client, e.g. "0.1.0". */
  widgetVersion: string;
  /** Arbitrary key-value data to include in context.app_context on submit. */
  appContext?: Record<string, unknown>;
}

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface SubmitResult {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}
