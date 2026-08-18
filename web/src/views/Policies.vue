<script setup lang="ts">
import { h, onMounted, reactive, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NModal,
  NInput,
  NForm,
  NFormItem,
  NSwitch,
  NSelect,
  NInputNumber,
  NTag,
  NSpace,
  NTimePicker,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { policiesApi, type BackupPolicy } from '@/api/policies'
import { databasesApi } from '@/api/databases'
import { storagesApi, type StorageTarget } from '@/api/storages'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()
const instanceStore = useInstanceStore()

const policies = ref<BackupPolicy[]>([])
const loading = ref(false)
const databases = ref<{ name: string }[]>([])
const storages = ref<StorageTarget[]>([])

async function load() {
  loading.value = true
  try {
    policies.value = await policiesApi.list()
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

onMounted(async () => {
  await instanceStore.load()
  await load()
  try {
    storages.value = await storagesApi.list()
  } catch {
    /* ignore */
  }
})

const instanceOptions = computed(() => instanceStore.instances.map((i) => ({ label: i.name, value: i.id })))
const storageOptions = computed(() => storages.value.map((s) => ({ label: s.name, value: s.id })))
const dbOptions = computed(() => databases.value.map((d) => ({ label: d.name, value: d.name })))

const showModal = ref(false)
const editingId = ref<number | null>(null)
const form = reactive({
  name: '',
  instance_id: 0,
  database_name: '',
  enabled: true,
  schedule_type: 'daily' as 'daily' | 'weekly' | 'monthly',
  time: 3 * 3600 * 1000,
  day: 1,
  retention_count: 14,
  globals_enabled: false,
  storage_targets: [] as number[],
})
const saving = ref(false)

function msToHM(ms: number): string {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function hmToMs(hm: string): number {
  const [h, m] = hm.split(':').map(Number)
  return (h * 3600 + m * 60) * 1000
}

const dayOptions = computed(() => {
  if (form.schedule_type === 'weekly') {
    return ['日', '一', '二', '三', '四', '五', '六'].map((d, i) => ({ label: d, value: i }))
  }
  return Array.from({ length: 31 }, (_, i) => ({ label: String(i + 1), value: i + 1 }))
})

async function onInstanceChange(id: number) {
  try {
    databases.value = await databasesApi.list(id)
  } catch {
    databases.value = []
  }
}

function openCreate() {
  editingId.value = null
  Object.assign(form, {
    name: '', instance_id: instanceStore.selectedId || 0, database_name: '',
    enabled: true, schedule_type: 'daily', time: 3 * 3600 * 1000, day: 1,
    retention_count: 14, globals_enabled: false, storage_targets: [],
  })
  showModal.value = true
  if (form.instance_id) onInstanceChange(form.instance_id)
}

function openEdit(row: BackupPolicy) {
  editingId.value = row.id
  Object.assign(form, {
    name: row.name, instance_id: row.instance_id, database_name: row.database_name,
    enabled: row.enabled, schedule_type: row.schedule.type, time: hmToMs(row.schedule.time),
    day: row.schedule.day, retention_count: row.retention_count,
    globals_enabled: row.globals_enabled, storage_targets: [...row.storage_targets],
  })
  showModal.value = true
  onInstanceChange(row.instance_id)
}

async function save() {
  saving.value = true
  try {
    const payload = {
      name: form.name,
      instance_id: form.instance_id,
      database_name: form.database_name,
      enabled: form.enabled,
      schedule: { type: form.schedule_type, time: msToHM(form.time), day: form.day },
      retention_count: form.retention_count,
      globals_enabled: form.globals_enabled,
      storage_targets: form.storage_targets,
    }
    if (editingId.value) await policiesApi.update(editingId.value, payload)
    else await policiesApi.create(payload)
    message.success(t('common.success'))
    showModal.value = false
    await load()
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function remove(row: BackupPolicy) {
  try {
    await policiesApi.remove(row.id)
    message.success(t('common.success'))
    await load()
  } catch (e) {
    handleError(e)
  }
}

function scheduleLabel(p: BackupPolicy): string {
  const s = p.schedule
  if (s.type === 'daily') return `${t('policies.scheduleDaily')} ${s.time}`
  if (s.type === 'weekly') return `${t('policies.scheduleWeekly')} ${dayOptions.value.find((d) => d.value === s.day)?.label ?? s.day} ${s.time}`
  return `${t('policies.scheduleMonthly')} ${s.day} ${s.time}`
}

function instanceName(id: number) {
  return instanceStore.instances.find((i) => i.id === id)?.name ?? String(id)
}

const columns: DataTableColumns<BackupPolicy> = [
  { title: t('policies.name'), key: 'name' },
  { title: t('policies.instance'), key: 'instance_id', render: (row) => instanceName(row.instance_id) },
  { title: t('policies.database'), key: 'database_name', render: (row) => row.database_name || t('backups.typeGlobals') },
  {
    title: t('policies.enabled'),
    key: 'enabled',
    render: (row) => h(NTag, { size: 'small', type: row.enabled ? 'success' : 'default' }, () => (row.enabled ? t('common.enabled') : t('common.disabled'))),
  },
  { title: t('policies.schedule'), key: 'schedule', render: (row) => scheduleLabel(row) },
  { title: t('policies.retention'), key: 'retention_count' },
  { title: t('policies.nextRun'), key: 'next_run_at', render: (row) => (row.next_run_at ? row.next_run_at.replace('T', ' ').slice(0, 16) : t('policies.neverRun')) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 140,
    render: (row) =>
      h(NSpace, { size: 4 }, () => [
        h(NButton, { size: 'small', onClick: () => openEdit(row) }, () => t('common.edit')),
        h(NButton, { size: 'small', type: 'error', tertiary: true, onClick: () => remove(row) }, () => t('common.delete')),
      ]),
  },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <n-button type="primary" @click="openCreate">{{ t('policies.add') }}</n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="policies"
      :loading="loading"
      :empty-description="t('common.empty')"
    />

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingId ? t('policies.edit') : t('policies.add')"
      style="width: 540px"
    >
      <n-form label-placement="left" label-width="130">
        <n-form-item :label="t('policies.name')" required>
          <n-input v-model:value="form.name" />
        </n-form-item>
        <n-form-item :label="t('policies.instance')" required>
          <n-select v-model:value="form.instance_id" :options="instanceOptions" @update:value="onInstanceChange" />
        </n-form-item>
        <n-form-item :label="t('policies.database')">
          <n-select
            v-model:value="form.database_name"
            :options="dbOptions"
            :disabled="form.globals_enabled"
          />
        </n-form-item>
        <n-form-item :label="t('policies.globals')">
          <n-switch v-model:value="form.globals_enabled" />
        </n-form-item>
        <n-form-item :label="t('policies.schedule')">
          <n-select v-model:value="form.schedule_type" :options="[
            { label: t('policies.scheduleDaily'), value: 'daily' },
            { label: t('policies.scheduleWeekly'), value: 'weekly' },
            { label: t('policies.scheduleMonthly'), value: 'monthly' },
          ]" style="width: 120px" />
          <n-time-picker v-model:value="form.time" format="HH:mm" style="width: 120px" />
        </n-form-item>
        <n-form-item v-if="form.schedule_type !== 'daily'" :label="form.schedule_type === 'weekly' ? t('policies.dayOfWeek') : t('policies.day')">
          <n-select v-model:value="form.day" :options="dayOptions" style="width: 120px" />
        </n-form-item>
        <n-form-item :label="t('policies.retention')">
          <n-input-number v-model:value="form.retention_count" :min="1" :max="999" />
          <span class="hint">{{ t('policies.retentionHint') }}</span>
        </n-form-item>
        <n-form-item :label="t('policies.storageTargets')" required>
          <n-select v-model:value="form.storage_targets" :options="storageOptions" multiple />
        </n-form-item>
        <n-form-item :label="t('policies.enabled')">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="save" :loading="saving">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}
.hint {
  margin-left: 8px;
  font-size: 12px;
  opacity: 0.6;
}
</style>
