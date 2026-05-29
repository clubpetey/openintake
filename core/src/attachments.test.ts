import { describe, it, expect } from 'vitest';
import {
  AttachmentList,
  AttachmentTooLargeError,
  AggregateTooLargeError,
  MimeNotAllowedError,
  type PendingAttachment,
} from './attachments.js';

const LIMITS = {
  maxSizeBytes: 5_000_000,
  maxTotalBytes: 10_000_000,
  allowedMimeTypes: ['image/png', 'image/jpeg', 'image/webp'],
};

function mkAttachment(sizeBytes: number, mime = 'image/png'): PendingAttachment {
  return {
    type: 'screenshot',
    mimeType: mime,
    dataUrl: `data:${mime};base64,AAAA`,
    sizeBytes,
  };
}

describe('AttachmentList', () => {
  it('starts empty', () => {
    const list = new AttachmentList(LIMITS);
    expect(list.items()).toEqual([]);
    expect(list.aggregateSizeBytes()).toBe(0);
  });

  it('add() appends and tracks aggregate', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.add(mkAttachment(2000));
    expect(list.items()).toHaveLength(2);
    expect(list.aggregateSizeBytes()).toBe(3000);
  });

  it('remove(index) removes the entry at index', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.add(mkAttachment(2000));
    list.remove(0);
    expect(list.items()).toHaveLength(1);
    expect(list.items()[0].sizeBytes).toBe(2000);
    expect(list.aggregateSizeBytes()).toBe(2000);
  });

  it('clear() empties the list and resets aggregate', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.clear();
    expect(list.items()).toEqual([]);
    expect(list.aggregateSizeBytes()).toBe(0);
  });

  it('add() throws AttachmentTooLargeError when single exceeds maxSizeBytes', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.add(mkAttachment(6_000_000))).toThrow(AttachmentTooLargeError);
    expect(list.items()).toEqual([]);
  });

  it('AttachmentTooLargeError carries code "attachment_too_large"', () => {
    const list = new AttachmentList(LIMITS);
    try {
      list.add(mkAttachment(6_000_000));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(AttachmentTooLargeError);
      expect((e as AttachmentTooLargeError).code).toBe('attachment_too_large');
    }
  });

  it('add() throws AggregateTooLargeError when adding would push past maxTotalBytes', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(4_000_000));
    list.add(mkAttachment(4_000_000));
    expect(() => list.add(mkAttachment(4_000_000))).toThrow(AggregateTooLargeError);
    expect(list.items()).toHaveLength(2);
    expect(list.aggregateSizeBytes()).toBe(8_000_000);
  });

  it('AggregateTooLargeError carries code "attachments_exceed_total"', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(4_000_000));
    list.add(mkAttachment(4_000_000));
    try {
      list.add(mkAttachment(4_000_000));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(AggregateTooLargeError);
      expect((e as AggregateTooLargeError).code).toBe('attachments_exceed_total');
    }
  });

  it('add() throws MimeNotAllowedError when mime not in allowedMimeTypes', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.add(mkAttachment(1000, 'image/heic'))).toThrow(MimeNotAllowedError);
  });

  it('MimeNotAllowedError carries code "attachment_mime_not_allowed"', () => {
    const list = new AttachmentList(LIMITS);
    try {
      list.add(mkAttachment(1000, 'image/heic'));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(MimeNotAllowedError);
      expect((e as MimeNotAllowedError).code).toBe('attachment_mime_not_allowed');
    }
  });

  it('boundary: single attachment at exactly maxSizeBytes is accepted', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(5_000_000));
    expect(list.items()).toHaveLength(1);
  });

  it('boundary: aggregate at exactly maxTotalBytes is accepted', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(5_000_000));
    list.add(mkAttachment(5_000_000));
    expect(list.aggregateSizeBytes()).toBe(10_000_000);
  });

  it('remove(index) on out-of-range index throws RangeError', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.remove(0)).toThrow(RangeError);
    list.add(mkAttachment(100));
    expect(() => list.remove(5)).toThrow(RangeError);
  });
});
