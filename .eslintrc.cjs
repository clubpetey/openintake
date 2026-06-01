// .eslintrc.cjs — Phase 7 (7-i) lint config for TypeScript + Vue.
// eslint-plugin-vue recommended + @typescript-eslint/recommended strict.
// See ai/tasks/phase-7/README.md §6 build-fail item 5.
module.exports = {
  root: true,
  env: {
    browser: true,
    node: true,
    es2022: true,
  },
  parser: 'vue-eslint-parser',
  parserOptions: {
    parser: '@typescript-eslint/parser',
    ecmaVersion: 2022,
    sourceType: 'module',
    extraFileExtensions: ['.vue'],
  },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:vue/vue3-recommended',
  ],
  plugins: ['@typescript-eslint', 'vue'],
  rules: {
    // Curated overrides — narrow rather than blanket-disable.
    '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    '@typescript-eslint/no-explicit-any': 'warn',
    'vue/multi-word-component-names': 'off', // Widget components are single-word by convention.
    'no-console': ['warn', { allow: ['warn', 'error'] }],
  },
  overrides: [
    {
      // Smoke drivers may use console.log for progress output.
      files: ['core/smoke/**/*.ts', 'vue/smoke/**/*.ts'],
      rules: {
        'no-console': 'off',
      },
    },
    {
      // Generated codegen output is exempt — covered by .eslintignore.
      files: ['**/generated/**/*.ts'],
      rules: {
        '@typescript-eslint/no-explicit-any': 'off',
      },
    },
  ],
};
