<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { appVersion, repositoryURL } from '../appMeta'

const emit = defineEmits<{ close: [] }>()
const runtimeInfo = ref<RuntimeInfo | null>(null), aboutError = ref('')

function api() { return window.go?.main?.App }

onMounted(async () => {
  const a = api()
  if (!a) return
  try { runtimeInfo.value = await a.GetRuntimeInfo() } catch (e) { aboutError.value = '读取运行信息失败：' + String(e) }
})

function aboutSummary() {
  const info = runtimeInfo.value
  return ['MeteorDesk v' + appVersion, '后端版本 ' + (info?.appVersion ?? '未知'), info?.goVersion ?? '', '平台 ' + (info?.platform ?? '未知'), '配置目录 ' + (info?.dataDir ?? '未知')].filter(Boolean).join('\n')
}

async function copyAboutInfo() {
  try { await navigator.clipboard.writeText(aboutSummary()); aboutError.value = '诊断信息已复制。' }
  catch { aboutError.value = '复制失败，请检查剪贴板权限。' }
}
</script>

<template>
 <div class="settings-overlay" @click.self="emit('close')">
  <section class="settings-dialog about-dialog" role="dialog" aria-modal="true" aria-labelledby="about-title">
   <div class="settings-dialog-heading"><div><span class="eyebrow">ABOUT</span><h2 id="about-title">MeteorDesk</h2><p>本地开发与运维工作台。日志、请求和配置都留在本机，不会自动上传。</p></div><button class="icon-button" type="button" aria-label="关闭" @click="emit('close')"><X :size="17"/></button></div>
   <dl class="about-list">
    <div><dt>应用版本</dt><dd>v{{appVersion}}</dd></div>
    <div><dt>后端版本</dt><dd>{{runtimeInfo?.appVersion||'dev'}}</dd></div>
    <div><dt>Go 运行时</dt><dd>{{runtimeInfo?.goVersion||'—'}}</dd></div>
    <div><dt>运行平台</dt><dd>{{runtimeInfo?.platform||'—'}}</dd></div>
    <div><dt>配置目录</dt><dd class="mono">{{runtimeInfo?.dataDir||'—'}}</dd></div>
    <div><dt>项目仓库</dt><dd class="mono">{{repositoryURL}}</dd></div>
   </dl>
   <p class="about-note">发布包与校验和发布在 GitHub Releases；应用不会在后台检查更新。</p>
   <p v-if="aboutError" class="settings-error" role="status">{{aboutError}}</p>
   <div class="settings-dialog-actions"><button class="button button-secondary" type="button" @click="copyAboutInfo">复制诊断信息</button><span class="option-spacer"></span><button class="button button-primary" type="button" @click="emit('close')">关闭</button></div>
  </section>
 </div>
</template>
