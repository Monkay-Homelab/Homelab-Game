import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import eslintConfigPrettier from 'eslint-config-prettier';

export default tseslint.config(
  { ignores: ['dist/', 'node_modules/'] },

  // Base JS recommended rules
  js.configs.recommended,

  // TypeScript recommended rules (type-aware where possible)
  ...tseslint.configs.recommended,

  // React hooks rules
  {
    plugins: {
      'react-hooks': reactHooks,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
    },
  },

  // React refresh rules (Vite HMR)
  {
    plugins: {
      'react-refresh': reactRefresh,
    },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },

  // Project-specific overrides
  {
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      // Allow unused vars prefixed with underscore (common pattern for intentional ignores)
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],

      // Allow explicit any as warning, not error — existing codebase uses it in places
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },

  // Prettier must be last — disables formatting rules that conflict with Prettier
  eslintConfigPrettier,
);
