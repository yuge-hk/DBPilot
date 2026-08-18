<script setup lang="ts">
import { h, onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NSelect,
  NTag,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { backupsApi, type BackupRecord } from '@/api/backups'
import { databasesApi, type Database } from '@/api/databases'
import { storagesApi, type StorageTarget } from '@/api/storages'
import { useInstanceStore } from '@/stores/instance'
import { useRouter } from 'vue-router'
import { ApiRequestError } from '@/api/client'
import InstanceSelector from '@/components/InstanceSelector.vue'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const store = useInstanceStore()

const backups = ref<BackupRecord[]>([])
const loading = ref(false)
const databases = ref<Database[]>([])
const storages = ref<StorageTarget[]>([])
const selectedDatabase = ref<string | null>(null)
const selectedStorages = ref<number[]>([])

const dbOptions = computed(() =>
  databases.value.map((d) => ({ label: d.name, value: d.name })),
)
const storageOptions = computed(() =>
  storages.value.map((s) => ({ label: s.name, value: s.id })),
)

async function loadBackups() {
  if (!store.selectedId) return
  loading.value = true
  try {
    backups.value = await backupsApi.list({ instance_id: store.selectedId })
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}

async function loadRefs() {
  if (!store.selectedId) return
  try {
    databases.value = await databasesApi.list(store.selectedId)
    storages.value = await storagesApi.list()
    if (selectedDatabase.value && !databases.value.some((d) => d.name === selectedDatabase.value)) {
      selectedDatabase.value = null
    }
  } catch (e) {
    handleError(e)
  }
}

function handleError(e: unknown) {
  if (e instanceof ApiRequestError) message.error(t(`errors.${e.code}`))
  else message.error(t('common.failed'))
}

onMounted(async () => {
  await store.load()
  await loadRefs()
  await loadBackups()
})

async function onInstanceChange() {
  selectedDatabase.value = null
  selectedStorages.value = []
  await loadRefs()
  await loadBackups()
}

async function triggerBackup() {
  if (!selectedDatabase.value) {
    message.warning(t('backups.selectDatabase'))
    return
  }
  if (selectedStorages.value.length === 0) {
    message.warning(t('backups.selectStorage'))
    return
  }
  try {
    await backupsApi.create({
      instance_id: store.selectedId,
      database: selectedDatabase.value,
      storage_ids: selectedStorages.value,
    })
    message.success(t('common.success'))
    router.push('/tasks')
  } catch (e) {
    handleError(e)
  }
}

async function triggerGlobals() {
  if (selectedStorages.value.length === 0) {
    message.warning(t('backups.selectStorage'))
    return
  }
  try {
    await backupsApi.createGlobals({
      instance_id: store.selectedId,
      storage_ids: selectedStorages.value,
    })
    message.success(t('common.success'))
    router.push('/tasks')
  } catch (e) {
    handleError(e)
  }
}

async function removeBackup(row: BackupRecord) {
  try {
    await backupsApi.remove(row.id)
    message.success(t('common.success'))
    await loadBackups()
  } catch (e) {
    handleError(e)
  }
}

function statusTag(status: string) {
  const map: Record<string, { type: any; label: string }> = {
    success: { type: 'success', label: t('backups.statusSuccess') },
    partial_success: { type: 'warning', label: t('backups.statusPartial') },
    failed: { type: 'error', label: t('backups.statusFailed') },
    running: { type: 'info', label: t('backups.statusRunning') },
  }
  const m = map[status] || { type: 'default', label: status }
  return h(NTag, { size: 'small', type: m.type }, () => m.label)
}

function formatSize(bytes: number): string {
  if (!bytes) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}

const columns: DataTableColumns<BackupRecord> = [
  {
    title: t('backups.database'),
    key: 'database',
    render: (row) => (row.backup_type === 'globals' ? t('backups.typeGlobals') : row.database),
  },
  {
    title: t('backups.backupType'),
    key: 'backup_type',
    render: (row) => (row.backup_type === 'globals' ? t('backups.typeGlobals') : t('backups.typeDatabase')),
  },
  { title: t('backups.status'), key: 'status', render: (row) => statusTag(row.status) },
  { title: t('backups.startedAt'), key: 'started_at', render: (row) => row.started_at.replace('T', ' ').slice(0, 19) },
  {
    title: t('backups.size'),
    key: 'final_size',
    render: (row) => formatSize(row.final_size),
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NPopconfirm,
        { onPositiveClick: () => removeBackup(row) },
        {
          trigger: () => h(NButton, { size: 'small', type: 'error', tertiary: true }, () => t('common.delete')),
          default: () => t('backups.deleteConfirm'),
        },
      ),
  },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <InstanceSelector @update:value="onInstanceChange" />
      <n-select
        v-model:value="selectedDatabase"
        :options="dbOptions"
        :placeholder="t('backups.selectDatabase')"
        style="width: 200px"
      />
      <n-select
        v-model:value="selectedStorages"
        :options="storageOptions"
        multiple
        :placeholder="t('backups.selectStorage')"
        style="width: 240px"
      />
      <n-button type="primary" @click="triggerBackup">{{ t('backups.manualBackup') }}</n-button>
      <n-button @click="triggerGlobals">{{ t('backups.globalsBackup') }}</n-button>
      <n-button quaternary @click="loadBackups">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="backups"
      :loading="loading"
      :empty-description="t('common.empty')"
    />
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  align-items: center;
}
</style>
