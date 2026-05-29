// Pending attachment state for the widget. Errors carry a `code` string so
// useIntake.ts can map them to user-readable banner text (Phase 6 design §8.3).

export interface PendingAttachment {
  type: 'screenshot';
  mimeType: string;
  dataUrl: string;
  label?: string;
  sizeBytes: number;
}

export interface AttachmentLimits {
  maxSizeBytes: number;
  maxTotalBytes: number;
  allowedMimeTypes: string[];
}

/**
 * AttachmentTooLargeError — single attachment exceeds maxSizeBytes.
 * Maps to relay code "attachment_too_large".
 */
export class AttachmentTooLargeError extends Error {
  readonly code = 'attachment_too_large';
  constructor(message?: string) {
    super(message ?? 'attachment exceeds max_size_bytes');
    this.name = 'AttachmentTooLargeError';
  }
}

/**
 * AggregateTooLargeError — adding an attachment would push total past maxTotalBytes.
 * Maps to relay code "attachments_exceed_total".
 */
export class AggregateTooLargeError extends Error {
  readonly code = 'attachments_exceed_total';
  constructor(message?: string) {
    super(message ?? 'attachments exceed total cap');
    this.name = 'AggregateTooLargeError';
  }
}

/**
 * MimeNotAllowedError — declared MIME not in allowedMimeTypes.
 * Maps to relay code "attachment_mime_not_allowed".
 */
export class MimeNotAllowedError extends Error {
  readonly code = 'attachment_mime_not_allowed';
  constructor(message?: string) {
    super(message ?? 'attachment mime_type not allowed');
    this.name = 'MimeNotAllowedError';
  }
}

export class AttachmentList {
  private readonly limits: AttachmentLimits;
  private list: PendingAttachment[] = [];

  constructor(limits: AttachmentLimits) {
    this.limits = limits;
  }

  add(att: PendingAttachment): void {
    if (!this.limits.allowedMimeTypes.includes(att.mimeType)) {
      throw new MimeNotAllowedError();
    }
    if (att.sizeBytes > this.limits.maxSizeBytes) {
      throw new AttachmentTooLargeError();
    }
    const projected = this.aggregateSizeBytes() + att.sizeBytes;
    if (projected > this.limits.maxTotalBytes) {
      throw new AggregateTooLargeError();
    }
    this.list = [...this.list, att];
  }

  remove(index: number): void {
    if (index < 0 || index >= this.list.length) {
      throw new RangeError(`AttachmentList.remove: index ${index} out of range`);
    }
    this.list = this.list.filter((_, i) => i !== index);
  }

  clear(): void {
    this.list = [];
  }

  items(): PendingAttachment[] {
    return this.list;
  }

  aggregateSizeBytes(): number {
    return this.list.reduce((s, a) => s + a.sizeBytes, 0);
  }
}
