<script setup lang="ts">
import { ref } from 'vue'
import { Copy, FileText } from 'lucide-vue-next'
import { useCopyFeedback } from '../../composables/useCopyFeedback'
import { applyKeywordCase, compactSql, formatSql, stripSqlComments, type KeywordCase } from '../../utils/sqlFormat'

const { copyText } = useCopyFeedback()

const sqlInput = ref('SELECT u.id, u.name, COUNT(o.id) AS order_count FROM users u LEFT JOIN orders o ON o.user_id = u.id WHERE u.status = ? AND o.created_at >= ? GROUP BY u.id, u.name ORDER BY order_count DESC LIMIT 10;')
const sqlOutput = ref('')
const sqlMessage = ref('')
const keywordCase = ref<KeywordCase>('upper')
const indentSize = ref(2)

function run(action: 'format' | 'compact' | 'strip' | 'case') {
  sqlMessage.value = ''
  if (!sqlInput.value.trim()) { sqlMessage.value = '请输入 SQL 语句。'; return }
  try {
    if (action === 'format') sqlOutput.value = formatSql(sqlInput.value, { case: keywordCase.value, indent: indentSize.value })
    else if (action === 'compact') sqlOutput.value = compactSql(sqlInput.value)
    else if (action === 'strip') sqlOutput.value = stripSqlComments(sqlInput.value)
    else if (action === 'case') sqlOutput.value = applyKeywordCase(sqlInput.value, keywordCase.value)
    sqlMessage.value = action === 'compact' ? '已压缩为单行。' : action === 'strip' ? '已移除注释。' : action === 'case' ? '关键字大小写已切换。' : '格式化完成。'
  } catch (e) {
    sqlMessage.value = '处理失败：' + String(e)
  }
}
function resetSql() { sqlInput.value = ''; sqlOutput.value = ''; sqlMessage.value = '' }
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>SQL 格式化工具</h2><p>多行展开、单行压缩、去注释、关键字大小写切换；纯前端处理，零数据库连接。</p></div><FileText :size="18" /></div>

    <div class="toolkit-action-row">
      <button class="button button-primary" @click="run('format')"><FileText :size="14" />格式化展开</button>
      <button class="button button-secondary" @click="run('compact')">压缩为单行</button>
      <button class="button button-secondary" @click="run('strip')">移除注释</button>
      <button class="button button-secondary" @click="run('case')">应用大小写</button>
      <button class="button button-quiet" @click="copyText(sqlOutput || sqlInput, 'SQL')"><Copy :size="14" />复制最近输出</button>
      <button class="button button-quiet" @click="resetSql">清空</button>
    </div>

    <div class="toolkit-input-row">
      <label class="toolkit-label" for="sql-case">关键字大小写</label>
      <select id="sql-case" class="toolkit-select" v-model="keywordCase" aria-label="关键字大小写">
        <option value="upper">全部大写（SELECT, FROM）</option>
        <option value="lower">全部小写（select, from）</option>
        <option value="keep">保持原样</option>
      </select>
      <label class="toolkit-label" for="sql-indent">缩进</label>
      <select id="sql-indent" class="toolkit-select" v-model.number="indentSize" aria-label="缩进空格数">
        <option :value="2">2 空格</option>
        <option :value="4">4 空格</option>
        <option :value="0">不缩进</option>
      </select>
    </div>

    <label class="toolkit-label" for="sql-input">输入 SQL</label>
    <textarea id="sql-input" v-model="sqlInput" class="toolkit-textarea tall mono" spellcheck="false" placeholder="SELECT * FROM users WHERE id = 1;" />

    <p v-if="sqlMessage" class="toolkit-inline-message">{{ sqlMessage }}</p>

    <label class="toolkit-label" for="sql-output">输出</label>
    <textarea id="sql-output" :value="sqlOutput" class="toolkit-textarea tall mono" readonly spellcheck="false" placeholder="输出会显示在这里……" />
  </div>
</template>