import type { App } from 'vue';
import IntakeWidget from './components/IntakeWidget.vue';
import ConversationView from './components/ConversationView.vue';
import ScreenshotRedactor from './components/ScreenshotRedactor.vue';
import AttachmentStrip from './components/AttachmentStrip.vue';
import { useIntake } from './composables/useIntake';
import type { UseIntakeOptions } from './composables/useIntake';

export { IntakeWidget, ConversationView, ScreenshotRedactor, AttachmentStrip, useIntake };
export type { UseIntakeOptions };

// Re-export the core types consumers will need
export type {
  ChatMessage,
  SubmitResult,
  IntakeConfig,
  PendingAttachment,
  AttachmentLimits,
} from '@intake/core';

/**
 * Vue plugin — optional. Registers IntakeWidget globally.
 * Usage: app.use(IntakePlugin)
 */
export const IntakePlugin = {
  install(app: App) {
    app.component('IntakeWidget', IntakeWidget);
  },
};
