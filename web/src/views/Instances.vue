<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
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
  NGrid,
  NGi,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  instancesApi,
  type Instance,
  type InstancePayload,
  type TestResult,
  type ToolInfo,
  type FirewallState,
  type HbaRule,
} from '@/api/instances'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()

const instances = ref<Instance[]>([])
const loading = ref(false)

const sslModes = computed(() => [
  { label: t('instances.sslDisable'), value: 'disable' },
  { label: t('instances.sslPrefer'), value: 'prefer' },
  { label: t('instances.sslRequire'), value: 'require' },
  { label: t('instances.sslVerifyCA'), value: 'verify-ca' },
  { label: t('instances.sslVerifyFull'), value: 'verify-full' },
])

function sslModeLabel(value: string) {
  const key = sslModes.value.find((mode) => mode.value === value)?.label
  return key || value
}

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
  connection_host: '',
})
const saving = ref(false)

// ---- test connection ----
const testing = ref(false)
const testResult = ref<TestResult | null>(null)

function openCreate() {
  editingId.value = null
  Object.assign(form, {
    name: '', host: '', port: 5432, username: '', password: '',
    ssl_mode: 'prefer', unix_socket: '', systemd_unit: '', connection_host: '',
  })
  testResult.value = null
  showModal.value = true
}

function openLocalCreate() {
  openCreate()
  Object.assign(form, {
    name: t('instances.localName'),
    host: '127.0.0.1',
    port: 5432,
    username: 'postgres',
    ssl_mode: 'disable',
    systemd_unit: 'postgresql',
  })
}

const fillingHost = ref(false)

async function fillLocalConnectionHost() {
  fillingHost.value = true
  try {
    const addresses = await instancesApi.systemAddresses()
    if (addresses.length === 0) {
      message.warning(t('instances.noLocalIP'))
      return
    }
    form.connection_host = addresses[0]
  } catch (e) {
    handleError(e)
  } finally {
    fillingHost.value = false
  }
}

function openEdit(inst: Instance) {
  editingId.value = inst.id
  Object.assign(form, {
    name: inst.name, host: inst.host, port: inst.port, username: inst.username,
    password: '', ssl_mode: inst.ssl_mode || 'prefer',
    unix_socket: inst.unix_socket || '', systemd_unit: inst.systemd_unit || '',
    connection_host: inst.connection_host || '',
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

// ---- firewall ----
const showFirewall = ref(false)
const firewall = ref<FirewallState | null>(null)
const firewallLoading = ref(false)
const firewallInstance = ref<Instance | null>(null)
const ruleForm = reactive({
  database: 'all',
  user: 'all',
  address: '',
  method: 'scram-sha-256',
})
const ruleSaving = ref(false)

async function openFirewall(inst: Instance) {
  firewallInstance.value = inst
  showFirewall.value = true
  ruleForm.address = ''
  ruleForm.database = 'all'
  ruleForm.user = 'all'
  ruleForm.method = 'scram-sha-256'
  await loadFirewall()
}

async function loadFirewall() {
  if (!firewallInstance.value) return
  firewallLoading.value = true
  try {
    firewall.value = await instancesApi.firewall(firewallInstance.value.id)
  } catch (e) {
    handleError(e)
  } finally {
    firewallLoading.value = false
  }
}

async function addRule() {
  if (!firewallInstance.value || !ruleForm.address) return
  ruleSaving.value = true
  try {
    await instancesApi.addFirewallRule(firewallInstance.value.id, { ...ruleForm })
    message.success(t('common.success'))
    ruleForm.address = ''
    await loadFirewall()
  } catch (e) {
    handleError(e)
  } finally {
    ruleSaving.value = false
  }
}

async function removeRule(rule: HbaRule) {
  if (!firewallInstance.value) return
  try {
    await instancesApi.removeFirewallRule(firewallInstance.value.id, {
      database: rule.database,
      user: rule.user,
      address: rule.address,
    })
    message.success(t('common.success'))
    await loadFirewall()
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
  {
    title: t('instances.connectionHost'),
    key: 'connection_host',
    render: (row) => row.connection_host || '-',
  },
  { title: t('instances.username'), key: 'username' },
  {
    title: t('instances.sslMode'),
    key: 'ssl_mode',
    render: (row) => h(NTag, { size: 'small' }, { default: () => sslModeLabel(row.ssl_mode) }),
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 260,
    render: (row) =>
      h(NSpace, { size: 4 }, () => [
        h(NButton, { size: 'small', onClick: () => testById(row) }, () => t('instances.testConnection')),
        h(NButton, { size: 'small', onClick: () => openTools(row) }, () => t('instances.tools')),
        h(NButton, { size: 'small', onClick: () => openFirewall(row) }, () => t('instances.firewall')),
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
      <n-button @click="openLocalCreate">{{ t('instances.addLocal') }}</n-button>
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
        <n-form-item :label="t('instances.connectionHost')">
          <div class="host-input">
            <n-input
              v-model:value="form.connection_host"
              :placeholder="t('instances.connectionHostPlaceholder')"
            />
            <n-button size="small" :loading="fillingHost" @click="fillLocalConnectionHost">
              {{ t('instances.useLocalIP') }}
            </n-button>
          </div>
          <div class="hint">{{ t('instances.connectionHostHint') }}</div>
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
          { title: t('common.version'), key: 'version', render: (row: ToolInfo) => row.version || '-' },
        ]"
        :data="tools"
        :loading="toolsLoading"
      />
      <n-alert type="warning" style="margin-top: 12px" :title="t('instances.toolsMissing')">
        {{ t('instances.toolsHint') }}
      </n-alert>
    </n-modal>

    <n-modal v-model:show="showFirewall" preset="card" :title="t('instances.firewall')" style="width: 960px; max-width: 96vw">
      <n-alert type="warning" style="margin-bottom: 12px" :title="t('instances.firewallHint')" />

      <n-descriptions v-if="firewall" bordered :column="2" size="small" style="margin-bottom: 12px">
        <n-descriptions-item :label="t('instances.listenAddresses')">
          {{ firewall.listen_addresses || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="pg_hba.conf">
          {{ firewall.hba_file || '-' }}
        </n-descriptions-item>
      </n-descriptions>

      <n-alert v-if="firewall && !firewall.writable" type="error" style="margin-bottom: 12px">
        {{ firewall.error || t('instances.firewallNoAccess') }}
      </n-alert>

      <template v-if="firewall && firewall.writable">
        <n-grid :cols="4" :x-gap="12" :y-gap="8" style="margin-bottom: 12px">
          <n-gi>
            <div class="fw-label">{{ t('instances.ruleDatabase') }}</div>
            <n-input v-model:value="ruleForm.database" />
          </n-gi>
          <n-gi>
            <div class="fw-label">{{ t('instances.ruleUser') }}</div>
            <n-input v-model:value="ruleForm.user" />
          </n-gi>
          <n-gi>
            <div class="fw-label">{{ t('instances.ruleAddress') }}</div>
            <n-input v-model:value="ruleForm.address" placeholder="192.168.0.0/24" />
          </n-gi>
          <n-gi>
            <div class="fw-label">{{ t('instances.ruleMethod') }}</div>
            <n-select
              v-model:value="ruleForm.method"
              :options="[
                { label: 'scram-sha-256', value: 'scram-sha-256' },
                { label: 'md5', value: 'md5' },
                { label: 'trust', value: 'trust' },
                { label: 'reject', value: 'reject' },
              ]"
            />
          </n-gi>
        </n-grid>
        <n-button
          type="primary"
          :loading="ruleSaving"
          :disabled="!ruleForm.address"
          @click="addRule"
          style="margin-bottom: 12px"
        >
          {{ t('instances.addRule') }}
        </n-button>

        <n-data-table
          :columns="[
            { title: t('instances.ruleType'), key: 'type', width: 90 },
            { title: t('instances.ruleDatabase'), key: 'database', width: 120 },
            { title: t('instances.ruleUser'), key: 'user', width: 120 },
            { title: t('instances.ruleAddress'), key: 'address' },
            { title: t('instances.ruleMethod'), key: 'method', width: 120 },
            {
              title: t('common.actions'),
              key: 'actions',
              width: 90,
              render: (row: HbaRule) =>
                row.type === 'host' || row.type === 'hostssl' || row.type === 'hostnossl'
                  ? h(
                      NButton,
                      { size: 'tiny', type: 'error', tertiary: true, onClick: () => removeRule(row) },
                      () => t('instances.removeRule'),
                    )
                  : '',
            },
          ]"
          :data="firewall.rules"
          :loading="firewallLoading"
          size="small"
          :bordered="false"
        />
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
.fw-label {
  font-size: 12px;
  opacity: 0.7;
  margin-bottom: 4px;
}
.host-input {
  display: flex;
  gap: 8px;
  width: 100%;
}
.hint {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  opacity: 0.7;
}
</style>
