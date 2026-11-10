<script setup lang="ts">
import { computed, ref } from 'vue'
import { Activity, Copy } from 'lucide-vue-next'
import { testRegex } from '../../utils/regexTester'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const pattern = ref('(\\w+)=(\\S+)')
const flags = ref('gi')
const inputText = ref('server started mode=production port=8080\nserver started mode=staging port=9090')

const result = computed(() => testRegex(pattern.value, flags.value, inputText.value))

const flagOptions = [
  { value: '', label: '无附加标记（默认会补 g）' },
  { value: 'i', label: 'i 忽略大小写' },
  { value: 'gi', label: 'gi 忽略大小写 + 全局' },
  { value: 'gm', label: 'gm 多行模式' },
  { value: 'gim', label: 'gim 多行 + 忽略大小写' },
  { value: 'gu', label: 'gu Unicode 模式' },
]

const presetRegexes = [
  { label: '自定义', pattern: '', flags: 'gi', sample: '' },
  { label: '邮箱地址', pattern: '[\\w.+-]+@[\\w-]+\\.[\\w.-]+', flags: 'gi', sample: 'contact@meteordesk.cn\nno-reply@github.com\ninvalid@@bad' },
  { label: 'URL（http/https）', pattern: 'https?://[\\w-._~:/?#\\[\\]@!$&\'()*+,;=%]+', flags: 'gi', sample: 'https://github.com/meteor-desk\nhttp://localhost:8080/api?x=1' },
  { label: 'IPv4 地址', pattern: '(?:25[0-5]|2[0-4]\\d|1\\d{2}|[1-9]?\\d)(?:\\.(?:25[0-5]|2[0-4]\\d|1\\d{2}|[1-9]?\\d)){3}', flags: 'g', sample: 'server: 192.168.1.100\ngateway 10.0.0.1\ninvalid 256.1.1.1' },
  { label: '端口号（1-65535）', pattern: '(?:\\[[a-fA-F0-9:]+\]:)?(?:localhost|\\d{1,3}(?:\\.\\d{1,3}){3}|[\\w.-]+):(\\d{1,5})', flags: 'g', sample: 'http://localhost:8080\nlisten 192.168.0.1:3000' },
  { label: 'ISO 日期时间', pattern: '\\d{4}-\\d{2}-\\d{2}[T ]\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:?\\d{2})?', flags: 'g', sample: '2025-11-06T14:30:00Z\n2025-11-06 14:30:00.123 +0800' },
  { label: '十六进制颜色', pattern: '#(?:[0-9a-fA-F]{3}){1,2}\\b', flags: 'gi', sample: 'primary: #ff5733\naccent: #1abc9c\nbad: #gggggg' },
  { label: 'UUID v4', pattern: '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}', flags: 'g', sample: 'token: 550e8400-e29b-41d4-a716-446655440000' },
  { label: '中文字符', pattern: '[\\u4e00-\\u9fa5]+', flags: 'g', sample: '这是中文hello world再一段' },
  { label: '日志级别', pattern: '\\b(DEBUG|INFO|WARN|WARNING|ERROR|FATAL|TRACE)\\b', flags: 'gi', sample: '[2025-11-06 14:30:00] INFO server started\n[2025-11-06 14:30:01] ERROR connection refused' },
]
const selectedPreset = ref(0)
function applyPreset(index: number) {
  if (!index) return
  const p = presetRegexes[index]
  pattern.value = p.pattern
  flags.value = p.flags
  inputText.value = p.sample
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>正则表达式调试器</h2><p>实时匹配、分组与命中时长；正则来源是您自己的输入，不会执行任意代码。</p></div><Activity :size="18" /></div>

    <div class="toolkit-input-row regex-pattern-row">
      <label class="toolkit-label" for="regex-pattern">正则</label>
      <input id="regex-pattern" v-model="pattern" class="toolkit-input mono regex-pattern-input" placeholder="例如 (\\w+)=(\\S+)" />
    </div>
    <div class="toolkit-input-row regex-flags-row">
      <label class="toolkit-label" for="regex-flags">标记</label>
      <input id="regex-flags" v-model="flags" class="toolkit-input mono" placeholder="g i m u s..." />
      <select class="toolkit-select" :value="flags" @change="flags = ($event.target as HTMLSelectElement).value" aria-label="快捷标记">
        <option v-for="opt in flagOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
      </select>
    </div>
    <div class="toolkit-input-row regex-preset-row">
      <label class="toolkit-label" for="regex-preset">常用模板</label>
      <select id="regex-preset" class="toolkit-select" :value="selectedPreset" @change="selectedPreset = Number(($event.target as HTMLSelectElement).value); applyPreset(selectedPreset)" aria-label="常用正则模板">
        <option v-for="(p, idx) in presetRegexes" :key="idx" :value="idx">{{ p.label }}</option>
      </select>
      <span class="toolkit-help">点击填充预定义正则、标记与示例文本</span>
    </div>

    <label class="toolkit-label" for="regex-input">测试文本</label>
    <textarea id="regex-input" v-model="inputText" class="toolkit-textarea tall mono" placeholder="输入要匹配的文本……" />

    <div class="toolkit-result-panel regex-summary">
      <div v-if="result.error" class="toolkit-error">{{ result.error }}</div>
      <template v-else>
        <div class="regex-summary-stats">
          <div class="regex-stat"><span>命中</span><strong>{{ result.matchCount }}</strong></div>
          <div class="regex-stat"><span>耗时</span><strong>{{ result.durationMs }} ms</strong></div>
          <div class="regex-stat"><span>标记</span><code>{{ result.flags || '（无）' }}</code></div>
        </div>

        <div v-if="result.hits.length" class="regex-hits">
          <table class="regex-hits-table">
            <thead><tr><th>#</th><th>位置</th><th>文本</th><th>分组</th></tr></thead>
            <tbody>
              <tr v-for="(hit, idx) in result.hits.slice(0, 200)" :key="idx">
                <td class="mono">{{ idx + 1 }}</td>
                <td class="mono">{{ hit.start }} – {{ hit.end }}</td>
                <td class="mono regex-hit-text"><code>{{ hit.text }}</code><button :aria-label="'复制第 ' + (idx + 1) + ' 个命中'" @click="copyText(hit.text, '命中文本')"><Copy :size="11" /></button></td>
                <td class="mono">
                  <template v-if="hit.groups.length">
                    <span v-for="(g, gi) in hit.groups" :key="gi" class="regex-group">{{ g === undefined ? '—' : g }}</span>
                  </template>
                  <span v-else class="toolkit-muted">无分组</span>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="result.hits.length > 200" class="toolkit-help">共 {{ result.matchCount }} 个命中，表格仅展示前 200 个。</p>
        </div>
        <p v-else class="toolkit-help">没有匹配项。检查标记或正则语法。</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.regex-pattern-row .toolkit-input { font-family: "Cascadia Code", Consolas, monospace; }
.regex-pattern-input { font-weight: 600; color: #111; }
.regex-flags-row { margin-bottom: 8px; }
.regex-summary { margin-top: 10px; }
.regex-summary-stats { display: flex; gap: 18px; margin-bottom: 12px; flex-wrap: wrap; }
.regex-stat { display: inline-flex; align-items: baseline; gap: 6px; font-size: 12.5px; color: #374151; }
.regex-stat span { color: var(--toolkit-muted); }
.regex-stat strong { font-size: 14px; color: #111; }
.regex-stat code { font-family: "Cascadia Code", Consolas, monospace; font-size: 12px; background: #fff; padding: 2px 6px; border-radius: 4px; border: 1px solid var(--toolkit-border); }
.regex-hits-table { width: 100%; border-collapse: collapse; font-size: 12.5px; }
.regex-hits-table th, .regex-hits-table td { padding: 6px 8px; border-bottom: 1px solid var(--toolkit-border); text-align: left; vertical-align: top; }
.regex-hits-table th { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: var(--toolkit-muted); background: #fff; }
.regex-hit-text code { background: #fff3cd; padding: 1px 4px; border-radius: 3px; font-family: "Cascadia Code", Consolas, monospace; }
.regex-hit-text button { background: none; border: none; cursor: pointer; margin-left: 4px; color: var(--toolkit-muted); }
.regex-hit-text button:hover { color: var(--toolkit-accent); }
.regex-group { display: inline-block; background: #e0f2fe; color: #0c4a6e; padding: 1px 5px; border-radius: 3px; margin-right: 3px; font-size: 11.5px; }
.toolkit-muted { color: var(--toolkit-muted); }
.mono { font-family: "Cascadia Code", Consolas, monospace; }
</style>