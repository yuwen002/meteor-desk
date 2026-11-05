<script setup lang="ts">
import { onMounted, ref, type Component } from 'vue'
import { ArrowLeftRight, Braces, CheckCircle2, Clock3, FileJson, KeyRound, Link2, Send, ShieldAlert, Activity, Dices, Fingerprint } from 'lucide-vue-next'
import { useCopyFeedback } from '../composables/useCopyFeedback'
import ToolkitTimestamp from './toolkit/ToolkitTimestamp.vue'
import ToolkitJson from './toolkit/ToolkitJson.vue'
import ToolkitRedact from './toolkit/ToolkitRedact.vue'
import ToolkitCron from './toolkit/ToolkitCron.vue'
import ToolkitDiff from './toolkit/ToolkitDiff.vue'
import ToolkitHttp from './toolkit/ToolkitHttp.vue'
import ToolkitBase64 from './toolkit/ToolkitBase64.vue'
import ToolkitJwt from './toolkit/ToolkitJwt.vue'
import ToolkitUrl from './toolkit/ToolkitUrl.vue'
import ToolkitHash from './toolkit/ToolkitHash.vue'
import ToolkitRandom from './toolkit/ToolkitRandom.vue'
import ToolkitRegex from './toolkit/ToolkitRegex.vue'

// 工具箱只负责标签页调度、共享复制提示与页面级说明；
// 每个面板的输入状态与换算逻辑都在 components/toolkit 下的独立组件里。
type ToolKey = 'timestamp' | 'json' | 'redact' | 'cron' | 'diff' | 'http' | 'base64' | 'jwt' | 'url' | 'hash' | 'random' | 'regex'

const toolTabs: { key: ToolKey; label: string; icon: Component }[] = [
  { key: 'timestamp', label: '时间戳 / 时区', icon: Clock3 },
  { key: 'json', label: 'JSON 工具', icon: Braces },
  { key: 'redact', label: '日志脱敏', icon: ShieldAlert },
  { key: 'cron', label: 'Cron 解析', icon: Clock3 },
  { key: 'diff', label: '文本差异', icon: ArrowLeftRight },
  { key: 'http', label: 'HTTP 调试', icon: Send },
  { key: 'base64', label: 'Base64', icon: FileJson },
  { key: 'jwt', label: 'JWT 载荷', icon: KeyRound },
  { key: 'url', label: 'URL / 参数', icon: Link2 },
  { key: 'hash', label: '哈希 / HMAC', icon: Fingerprint },
  { key: 'random', label: '随机 / UUID', icon: Dices },
  { key: 'regex', label: '正则调试', icon: Activity },
]

const panelComponents: Record<ToolKey, Component> = {
  timestamp: ToolkitTimestamp,
  json: ToolkitJson,
  redact: ToolkitRedact,
  cron: ToolkitCron,
  diff: ToolkitDiff,
  http: ToolkitHttp,
  base64: ToolkitBase64,
  jwt: ToolkitJwt,
  url: ToolkitUrl,
  hash: ToolkitHash,
  random: ToolkitRandom,
  regex: ToolkitRegex,
}

const activeTool = ref<ToolKey>('timestamp')
const { copyStatus, dismissCopyStatus } = useCopyFeedback()
// 复制提示是模块级共享状态，重新进入工具箱时清掉上一次的残留。
onMounted(dismissCopyStatus)
</script>

<template>
  <section class="page-content toolkit-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow"><span class="eyebrow-line"></span>开发者效率 / 本地工具</div>
        <h1>开发工具<span class="title-period">。</span></h1>
        <p class="page-subtitle">常用数据转换与排查工具，全部在本机运行，不会上传输入内容。</p>
      </div>
    </div>

    <div class="toolkit-tabs" role="tablist" aria-label="开发工具">
      <button v-for="tool in toolTabs" :key="tool.key" role="tab" :aria-selected="activeTool === tool.key" class="toolkit-tab" :class="{ active: activeTool === tool.key }" @click="activeTool = tool.key">
        <component :is="tool.icon" :size="16" />
        <span>{{ tool.label }}</span>
      </button>
    </div>

    <div v-if="copyStatus" class="toolkit-feedback" role="status"><CheckCircle2 :size="14" />{{ copyStatus }}<button type="button" @click="dismissCopyStatus">关闭</button></div>

    <!-- KeepAlive 让各面板的输入在切换标签页后仍然保留，与拆分前的单组件行为一致。 -->
    <KeepAlive>
      <component :is="panelComponents[activeTool]" />
    </KeepAlive>

    <div class="toolkit-footer-note"><ShieldAlert :size="15" /><span>所有转换均在桌面端本地完成。JWT 查看器仅负责解码，不验证令牌签名，也不代表其内容可信。</span></div>
  </section>
</template>