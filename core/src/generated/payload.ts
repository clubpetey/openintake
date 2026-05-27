// GENERATED from schema/payload.v1.json — DO NOT EDIT. Run: npm run codegen

export interface IntakePayload {
  schema_version: "1.0";
  submission: Submission;
  client: Client;
  user: User;
  conversation: Conversation;
  context?: Context;
  attachments?: Attachment[];
  routing_hint?: string | null;
}
export interface Submission {
  id: string;
  submitted_at: string;
  tenant_id?: string | null;
}
export interface Client {
  widget_version: string;
  session_id: string;
  url: string;
  referrer?: string | null;
  user_agent: string;
  viewport: Viewport;
  locale: string;
}
export interface Viewport {
  w: number;
  h: number;
}
export interface User {
  auth_mode: "anonymous" | "email" | "sso";
  id?: string | null;
  email?: string | null;
  display_name?: string | null;
  verified: boolean;
  custom?: {
    [k: string]: unknown;
  };
}
export interface Conversation {
  messages: Message[];
  summary: string;
  title_suggestion: string;
  classification: "bug" | "feature_request" | "question" | "other";
  severity_guess: "low" | "medium" | "high" | "critical" | "unknown";
  tags_suggested: string[];
  language: string;
}
export interface Message {
  role: "user" | "assistant";
  content: string;
  ts: string;
}
export interface Context {
  app_context?: {
    [k: string]: unknown;
  };
  page_metadata?: {
    [k: string]: unknown;
  };
}
export interface Attachment {
  type: "screenshot" | "file";
  mime_type: string;
  size_bytes: number;
  url: string;
  label?: string;
}
