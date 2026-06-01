// eslint.config.js — Phase 7 (7-i) flat config (ESLint 9+).
// Curated rulesets: @typescript-eslint/recommended + vue/vue3-recommended.
// See ai/tasks/phase-7/README.md §6 build-fail item 5.
import js from '@eslint/js';
import tseslint from '@typescript-eslint/eslint-plugin';
import tsparser from '@typescript-eslint/parser';
import vueplugin from 'eslint-plugin-vue';
import vueparser from 'vue-eslint-parser';
import globals from 'globals';

const sharedGlobals = {
  ...globals.browser,
  ...globals.node,
  // Vitest
  describe: 'readonly',
  it: 'readonly',
  test: 'readonly',
  expect: 'readonly',
  beforeAll: 'readonly',
  beforeEach: 'readonly',
  afterAll: 'readonly',
  afterEach: 'readonly',
  vi: 'readonly',
  // TS-only types that look like globals in JSDoc/type annotations
  RequestInit: 'readonly',
  BlobCallback: 'readonly',
};

export default [
  {
    ignores: [
      'node_modules/',
      'dist/',
      '**/dist/',
      '**/generated/',
      'local-dev/',
      '**/*.generated.*',
      'relay/',
      'schema/',
      'docs/',
      'ai/',
      '.github/',
      'examples/',
    ],
  },
  js.configs.recommended,
  {
    files: ['**/*.ts'],
    languageOptions: {
      parser: tsparser,
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: sharedGlobals,
    },
    plugins: {
      '@typescript-eslint': tseslint,
    },
    rules: {
      ...tseslint.configs.recommended.rules,
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/no-explicit-any': 'warn',
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'no-unused-vars': 'off',
    },
  },
  // Vue files — apply the plugin's recommended rules.
  {
    files: ['**/*.vue'],
    plugins: {
      vue: vueplugin,
      '@typescript-eslint': tseslint,
    },
    languageOptions: {
      parser: vueparser,
      parserOptions: {
        parser: tsparser,
        ecmaVersion: 2022,
        sourceType: 'module',
        extraFileExtensions: ['.vue'],
      },
      globals: sharedGlobals,
    },
    rules: {
      ...(vueplugin.configs.base ? vueplugin.configs.base.rules : {}),
      ...(vueplugin.configs['vue3-recommended'] ? vueplugin.configs['vue3-recommended'].rules : {}),
      'vue/multi-word-component-names': 'off',
      // vue/comment-directive trips on internal "clear" markers the parser
      // emits at SFC block boundaries when no user directive is present; it's
      // not a meaningful signal for our codebase.
      'vue/comment-directive': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      'no-unused-vars': 'off',
    },
  },
  // Smoke drivers may use console.log for progress output.
  {
    files: ['core/smoke/**/*.ts', 'vue/smoke/**/*.ts'],
    rules: {
      'no-console': 'off',
    },
  },
];
