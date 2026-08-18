<script setup lang="ts">
import { h, onMounted, ref, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NModal,
  NTag,
  NProgress,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { tasksApi, type Task } from '@/api/tasks'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()

const tasks = ref<Task[]>([])
const loading = ref(false)
let timer: number | undefined

async function load() {
  loading.value = true
  try {
    tasks.value = await tasksApi.list()
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}

function handleError(e: unknown) {
  if (e instanceof ApiRequestError) message.error(t(`errors.${e.code}`))
  else message.error(t('common.failed'))
}

onMounted(() => {
  load()
  timer = window.setInterval(() => {
    if (tasks.value.some((t) => t.status === 'queued' || t.status === 'running')) {
      load()
    }
  }, 3000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})

const detail = ref<Task | null>(null)

function openDetail(row: Task) {
  detail.value = row
}

function statusTag(status: string) {
  const map: Record<string, { type: any; label: string }> = {
    queued: { type: 'default', label: t('tasks.statusQueued') },
    running: { type: 'info', label: t('tasks.statusRunning') },
    success: { type: 'success', label: t('tasks.statusSuccess') },
    partial_success: { type: 'warning', label: t('tasks.statusPartial') },
    failed: { type: 'error', label: t('tasks.statusFailed') },
    cancelled: { type: 'default', label: t('tasks.statusCancelled') },
  }
  const m = map[status] || { type: 'default', label: status }
  return h(NTag, { size: 'small', type: m.type }, () => m.label)
}

const columns: DataTableColumns<Task> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: t('tasks.type'), key: 'type' },
  { title: t('tasks.resource'), key: 'resource_id' },
  { title: t('tasks.status'), key: 'status', render: (row) => statusTag(row.status) },
  {
    title: t('tasks.progress'),
    key: 'progress',
    render: (row) => h(NProgress, { type: 'line', percentage: row.progress, height: 8 }),
  },
  { title: t('tasks.error'), key: 'error_message', ellipsis: { tooltip: true } },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 90,
    render: (row) =>
      h(NButton, { size: 'small', onClick: () => openDetail(row) }, () => t('tasks.log')),
  },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="tasks"
      :loading="loading"
      :empty-description="t('common.empty')"
    />

    <n-modal
      :show="detail !== null"
      @update:show="(v) => { if (!v) detail = null }"
      preset="card"
      :title="`Task #${detail?.id ?? ''}`"
      style="width: 640px"
    >
      <pre class="log">{{ detail?.log || t('common.empty') }}</pre>
    </n-modal>
  </div>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}
.log {
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
  max-height: 400px;
  overflow: auto;
  background: rgba(0, 0, 0, 0.04);
  padding: 12px;
  border-radius: 4px;
}
</style>
