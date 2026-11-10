<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { appVersion, repositoryURL } from '../appMeta'
import { currentLocale, setLocale, type Locale } from '../i18n'

const { t } = useI18n()
const emit = defineEmits<{ close: [] }>()
const runtimeInfo = ref<RuntimeInfo | null>(null), aboutError = ref('')
const selectedLocale = ref<Locale>(currentLocale())

function api() { return window.go?.main?.App }

onMounted(async () => {
  const a = api()
  if (!a) return
  try { runtimeInfo.value = await a.GetRuntimeInfo() } catch (e) { aboutError.value = '读取运行信息失败：' + String(e) }
})

function changeLocale(loc: Locale) {
  setLocale(loc)
  selectedLocale.value = loc
}

const aboutSummary = computed(() => {
  const info = runtimeInfo.value
  return ['MeteorDesk v' + appVersion, 'Backend ' + (info?.appVersion ?? 'unknown'), info?.goVersion ?? '', 'Platform ' + (info?.platform ?? 'unknown'), 'Config ' + (info?.dataDir ?? 'unknown')].filter(Boolean).join('\n')
})

async function copyAboutInfo() {
  try { await navigator.clipboard.writeText(aboutSummary.value); aboutError.value = t('about.copied') }
  catch { aboutError.value = t('about.copyFailed') }
}
</script>

<template>
 <div class="settings-overlay" @click.self="emit('close')">
  <section class="settings-dialog about-dialog" role="dialog" aria-modal="true" aria-labelledby="about-title">
   <div class="settings-dialog-heading"><div><span class="eyebrow">ABOUT</span><h2 id="about-title">MeteorDesk</h2><p>{{ $t('about.description') }}</p></div><button class="icon-button" type="button" aria-label="关闭" @click="emit('close')"><X :size="17"/></button></div>
   <dl class="about-list">
    <div><dt>{{ $t('about.version') }}</dt><dd>v{{appVersion}}</dd></div>
    <div><dt>Backend</dt><dd>{{runtimeInfo?.appVersion||'dev'}}</dd></div>
    <div><dt>Go</dt><dd>{{runtimeInfo?.goVersion||'—'}}</dd></div>
    <div><dt>Platform</dt><dd>{{runtimeInfo?.platform||'—'}}</dd></div>
    <div><dt>Config</dt><dd class="mono">{{runtimeInfo?.dataDir||'—'}}</dd></div>
    <div><dt>Repo</dt><dd class="mono">{{repositoryURL}}</dd></div>
   </dl>
   <div class="language-row">
    <label class="field-label">{{ $t('about.language') }}</label>
    <div class="language-toggle">
     <button type="button" class="lang-btn" :class="{active: selectedLocale==='zh'}" @click="changeLocale('zh')">{{ $t('about.langZh') }}</button>
     <button type="button" class="lang-btn" :class="{active: selectedLocale==='en'}" @click="changeLocale('en')">{{ $t('about.langEn') }}</button>
    </div>
   </div>
   <p class="about-note">{{ $t('about.note') }}</p>
   <p v-if="aboutError" class="settings-error" role="status">{{aboutError}}</p>
   <div class="settings-dialog-actions"><button class="button button-secondary" type="button" @click="copyAboutInfo">Copy diagnostics</button><span class="option-spacer"></span><button class="button button-primary" type="button" @click="emit('close')">{{ $t('cache.close') }}</button></div>
  </section>
 </div>
</template>

<style scoped>
.language-row { display:flex; align-items:center; gap:12px; margin-top:12px; padding:10px 14px; background:var(--panel-surface-soft); border-radius:10px; }
.language-row .field-label { flex:0 0 auto; font-size:13px; color:var(--muted); margin:0; }
.language-toggle { display:flex; gap:4px; background:var(--input-bg); padding:3px; border-radius:8px; }
.lang-btn { padding:5px 14px; font-size:13px; border:none; background:transparent; color:var(--muted); border-radius:6px; cursor:pointer; transition:background .15s, color .15s; }
.lang-btn.active { background:var(--accent); color:#fff; }
</style>