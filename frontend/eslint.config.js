import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'

// 第一阶段只做“告警不阻断”的接入：规则集取官方推荐，命中项以真实缺陷为主，
// 界面模板里大量单行写法属于既有风格，先关闭对应的排版类规则，避免噪声淹没问题。
export default defineConfigWithVueTs(
  {
    name: 'meteor-desk/ignores',
    ignores: ['dist/**', 'node_modules/**', 'wailsjs/**'],
  },
  js.configs.recommended,
  pluginVue.configs['flat/recommended'],
  vueTsConfigs.recommended,
  {
    name: 'meteor-desk/rules',
    rules: {
      // 组件名保持现有单词语义（App、Overview 等页面级组件）。
      'vue/multi-word-component-names': 'off',
      // 排版类规则交给 gofmt 风格的“不阻断”阶段处理。
      'vue/max-attributes-per-line': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/multiline-html-element-content-newline': 'off',
      'vue/html-self-closing': 'off',
      'vue/html-indent': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/first-attribute-linebreak': 'off',
      'vue/attributes-order': 'off',
      // 模板采用“一个元素一行”的紧凑写法，以下三条纯空格/引号排版规则会和它冲突。
      'vue/mustache-interpolation-spacing': 'off',
      'vue/html-closing-bracket-spacing': 'off',
      'vue/html-quotes': 'off',
      // 下面几条降级为告警：多为存量写法，需要人工确认后再逐条收紧，
      // 不应和真正的错误混在一起刷屏。
      'vue/no-v-html': 'warn',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'no-console': ['warn', { allow: ['warn', 'error'] }],
    },
  },
)
