import { createI18n } from 'vue-i18n'
import zh from './zh'
import en from './en'

export type Locale = 'zh' | 'en'

const STORAGE_KEY = 'meteordeskt.locale'

function detectInitial(): Locale {
  const saved = localStorage.getItem(STORAGE_KEY) as Locale | null
  if (saved === 'zh' || saved === 'en') return saved
  const nav = typeof navigator !== 'undefined' ? navigator.language : 'zh-CN'
  return nav.toLowerCase().startsWith('en') ? 'en' : 'zh'
}

export const i18n = createI18n({
  legacy: false,
  locale: detectInitial(),
  fallbackLocale: 'zh',
  messages: { zh, en },
})

export function setLocale(locale: Locale) {
  localStorage.setItem(STORAGE_KEY, locale)
  ;(i18n.global as unknown as { locale: { value: Locale } }).locale.value = locale
  document.documentElement.lang = locale === 'en' ? 'en' : 'zh-CN'
}

export function currentLocale(): Locale {
  return (i18n.global as unknown as { locale: { value: Locale } }).locale.value
}