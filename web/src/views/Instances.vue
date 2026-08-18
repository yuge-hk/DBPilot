<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NTag,
  NPopconfirm,
  NAlert,
  NDescriptions,
  NDescriptionsItem,
  NSpace,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  instancesApi,
  type Instance,
  type InstancePayload,
  type TestResult,
  type ToolInfo,
} from '@/api/instances'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()

const instances = ref<Instance[]>([])
const loading = ref(false)

const sslModes = [
  { label: 'disable', value: 'disable' },
  { label: 'prefer', value: 'prefer' },
  { label: 'require', value: 'require' },
  { label: 'verify-ca', value: 'verify-ca' },
  { label: 'verify-full', value: 'verify-full' },
]

async function load() {
  loading.value = true
  try {
    instances.value = await instancesApi.list()
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}

function handleError(e: unknown) {
  if (e instanceof ApiRequestError) {
    message.error(t(`errors.${e.code}`))
  } else {
    message.error(t('common.failed'))
  }
}

onMounted(load)

// ---- create / edit modal ----
const showModal = ref(false)
const editingId = ref<number | null>(null)
const form = reactive<InstancePayload>({
  name: '',
  host: '',
  port: 5432,
  username: '',
  password: '',
  ssl_mode: 'prefer',
  unix_socket: '',
  systemd_unit: '',
})
const saving = ref(false)

// ---- test connection ----
const testing = ref(false)
const testResult = ref<TestResult | null>(null)

function openCreate() {
  editingId.value = null
  Object.assign(form, {
    name: '', host: '', port: 5432, username: '', password: '',
    ssl_mode: 'prefer', unix_socket: '', systemd_unit: '',
  })
  testResult.value = null
  showModal.value = true
}

function openEdit(inst: Instance) {
  editingId.value = inst.id
  Object.assign(form, {
    name: inst.name, host: inst.host, port: inst.port, username: inst.username,
    password: '', ssl_mode: inst.ssl_mode || 'prefer',
    unix_socket: inst.unix_socket || '', systemd_unit: inst.systemd_unit || '',
  })
  testResult.value = null
  showModal.value = true
}

async function runTest() {
  if (!form.host && !form.unix_socket) {
    message.warning(t('instances.testHint'))
    return
  }
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await instancesApi.test({
      host: form.host,
      port: form.port,
      username: form.username,
      password: form.password,
      ssl_mode: form.ssl_mode,
      unix_socket: form.unix_socket,
    })
  } catch (e) {
    handleError(e)
  } finally {
    testing.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const payload = { ...form }
    if (editingId.value) {
      await instancesApi.update(editingId.value, payload)
      message.success(t('common.success'))
    } else {
      await instancesApi.create(payload)
      message.success(t('common.success'))
    }
    showModal.value = false
    await load()
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function remove(inst: Instance) {
  try {
    await instancesApi.remove(inst.id)
    message.success(t('common.success'))
    await load()
  } catch (e) {
    handleError(e)
  }
}

// ---- tools ----
const showTools = ref(false)
const tools = ref<ToolInfo[]>([])
const toolsLoading = ref(false)

async function openTools(inst: Instance) {
  showTools.value = true
  toolsLoading.value = true
  try {
    tools.value = await instancesApi.tools(inst.id)
  } catch (e) {
    handleError(e)
  } finally {
    toolsLoading.value = false
  }
}

async function testById(inst: Instance) {
  try {
    const res = await instancesApi.testById(inst.id)
    if (res.ok) {
      message.success(`${t('instances.testSuccess')} (${res.version})`)
    } else {
      message.error(`${t('instances.testFailed')}: ${res.error || ''}`)
    }
  } catch (e) {
    handleError(e)
  }
}

const columns: DataTableColumns<Instance> = [
  { title: t('instances.name'), key: 'name' },
  {
    title: t('instances.host'),
    key: 'host',
    render: (row) => (row.unix_socket ? row.unix_socket : `${row.host}:${row.port}`),
  },
  { title: t('instances.username'), key: 'username' },
  {
    title: t('instances.sslMode'),
    key: 'ssl_mode',
    render: (row) => h(NTag, { size: 'small' }, { default: () => row.ssl_mode }),
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 260,
    render: (row) =>
      h(NSpace, { size: 4 }, () => [
        h(NButton, { size: 'small', onClick: () => testById(row) }, () => t('instances.testConnection')),
        h(NButton, { size: 'small', onClick: () => openTools(row) }, () => t('instances.tools')),
        h(NButton, { size: 'small', onClick: () => openEdit(row) }, () => t('common.edit')),
        h(
          NPopconfirm,
          { onPositiveClick: () => remove(row) },
          {
            trigger: () => h(NButton, { size: 'small', type: 'error', tertiary: true }, () => t('common.delete')),
            default: () => t('instances.deleteConfirm'),
          },
        ),
      ]),
  },
]

function testResultType(): 'success' | 'error' {
  return testResult.value?.ok ? 'success' : 'error'
}
</script>

<template>
  <div>
    <div class="toolbar">
      <n-button type="primary" @click="openCreate">{{ t('instances.add') }}</n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="instances"
      :loading="loading"
      :bordered="false"
      :empty-description="t('common.empty')"
    />

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingId ? t('instances.edit') : t('instances.add')"
      style="width: 560px"
    >
      <n-form label-placement="left" label-width="120">
        <n-form-item :label="t('instances.name')" required>
          <n-input v-model:value="form.name" :placeholder="t('instances.name')" />
        </n-form-item>
        <n-form-item :label="t('instances.host')">
          <n-input v-model:value="form.host" :placeholder="'127.0.0.1'" />
        </n-form-item>
        <n-form-item :label="t('instances.port')">
          <n-input-number v-model:value="form.port" :min="1" :max="65535" />
        </n-form-item>
        <n-form-item :label="t('instances.username')" required>
          <n-input v-model:value="form.username" placeholder="postgres" />
        </n-form-item>
        <n-form-item :label="t('instances.password')" :required="!editingId">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="editingId ? t('instances.passwordPlaceholder') : ''"
          />
        </n-form-item>
        <n-form-item :label="t('instances.sslMode')">
          <n-select v-model:value="form.ssl_mode" :options="sslModes" />
        </n-form-item>
        <n-form-item :label="t('instances.unixSocket')">
          <n-input v-model:value="form.unix_socket" :placeholder="t('instances.unixSocketPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('instances.systemdUnit')">
          <n-input v-model:value="form.systemd_unit" :placeholder="t('instances.systemdUnitPlaceholder')" />
        </n-form-item>
      </n-form>

      <n-alert v-if="testResult" :type="testResultType()" style="margin-bottom: 12px">
        <template v-if="testResult.ok">
          <n-descriptions :column="2" size="small" bordered>
            <n-descriptions-item :label="t('instances.serverVersion')">
              {{ testResult.version }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('instances.serverAddress')">
              {{ testResult.server_address || '-' }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('instances.currentUser')">
              {{ testResult.current_user }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('instances.databaseCount')">
              {{ testResult.database_count }}
            </n-descriptions-item>
          </n-descriptions>
        </template>
        <template v-else>{{ t('instances.testFailed') }}: {{ testResult.error }}</template>
      </n-alert>

      <template #footer>
        <n-space justify="end">
          <n-button @click="runTest" :loading="testing">{{ t('instances.testConnection') }}</n-button>
          <n-button type="primary" @click="save" :loading="saving">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="showTools" preset="card" :title="t('instances.tools')" style="width: 560px">
      <n-data-table
        :columns="[
          { title: t('common.name'), key: 'name' },
          {
            title: t('common.status'),
            key: 'found',
            render: (row: ToolInfo) =>
              h(
                NTag,
                { size: 'small', type: row.found ? 'success' : 'error' },
                { default: () => (row.found ? t('instances.toolFound') : t('instances.toolMissing')) },
              ),
          },
          { title: 'Version', key: 'version', render: (row: ToolInfo) => row.version || '-' },
        ]"
        :data="tools"
        :loading="toolsLoading"
      />
      <n-alert type="warning" style="margin-top: 12px" :title="t('instances.toolsMissing')">
        {{ t('instances.toolsHint') }}
      </n-alert>
    </n-modal>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}
</style>
