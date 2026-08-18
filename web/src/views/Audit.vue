<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NDataTable, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { auditApi, type AuditEntry } from '@/api/audit'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()

const entries = ref<AuditEntry[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    entries.value = await auditApi.list()
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

onMounted(load)

const columns: DataTableColumns<AuditEntry> = [
  { title: t('audit.time'), key: 'created_at', render: (row) => row.created_at.replace('T', ' ').slice(0, 19) },
  { title: t('audit.actor'), key: 'actor' },
  { title: t('audit.action'), key: 'action' },
  { title: t('audit.resource'), key: 'resource_id' },
  {
    title: t('audit.result'),
    key: 'result',
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.result === 'success' ? 'success' : 'error' },
        () => (row.result === 'success' ? t('audit.success') : t('audit.failure')),
      ),
  },
  { title: t('audit.remoteIp'), key: 'remote_ip' },
  { title: t('audit.detail'), key: 'detail' },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>
    <n-data-table
      :columns="columns"
      :data="entries"
      :loading="loading"
      :empty-description="t('common.empty')"
    />
  </div>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}
</style>
