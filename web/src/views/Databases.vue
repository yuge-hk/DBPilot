<script setup lang="ts">
import { h, onMounted, ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NButton,
  NDataTable,
  NModal,
  NInput,
  NAlert,
  NSpace,
  NRadioGroup,
  NRadio,
  NSelect,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NSwitch,
  NSteps,
  NStep,
  NCheckbox,
  NTag,
  NEmpty,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { databasesApi, type Database, type CreateDatabaseResult } from '@/api/databases'
import { accountsApi, type Account } from '@/api/accounts'
import { instancesApi } from '@/api/instances'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'
import { copyText } from '@/utils/clipboard'
import InstanceSelector from '@/components/InstanceSelector.vue'

const { t } = useI18n()
const router = useRouter()
const message = useMessage()
const store = useInstanceStore()

const databases = ref<Database[]>([])
const loading = ref(false)
const hideSystem = ref(true)

const systemDatabases = new Set(['postgres', 'template0', 'template1'])

function isSystemDatabase(name: string) {
  return systemDatabases.has(name)
}

const visibleDatabases = computed(() =>
  hideSystem.value ? databases.value.filter((db) => !isSystemDatabase(db.name)) : databases.value,
)

function formatSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}

async function load() {
  if (!store.selectedId) return
  loading.value = true
  try {
    databases.value = await databasesApi.list(store.selectedId)
  } catch (e) {
    handleError(e)
  } finally {
    loading.value = false
  }
}

function handleError(e: unknown) {
  if (e instanceof ApiRequestError) {
    const detailCodes = [
      'create_database_failed',
      'account_operation_failed',
      'account_has_dependencies',
      'grant_failed',
      'revoke_failed',
      'delete_failed',
    ]
    const detail = detailCodes.includes(e.code) && e.message ? `: ${e.message}` : ''
    message.error(t(`errors.${e.code}`) + detail)
  } else {
    message.error(t('common.failed'))
  }
}

onMounted(() => load())
watch(() => store.selectedId, () => load())

// ---- create database (one-click) ----
const showCreate = ref(false)
const creating = ref(false)
const accounts = ref<Account[]>([])
const createForm = reactive({
  database: '',
  account_mode: 'create' as 'create' | 'existing',
  account: '',
  password: '',
  permission: 'read_write' as 'owner' | 'read_write' | 'read_only',
  remote_access: false,
  remote_address: '',
})
const createResult = ref<CreateDatabaseResult | null>(null)
const showPassword = ref(false)
const hostOptions = ref<string[]>([])
const selectedHost = ref('')
const autoAccount = ref('')
const currentStep = ref(1)
const namePattern = /^[a-z_][a-z0-9_]{0,62}$/

const basicValid = computed(() => namePattern.test(createForm.database))
const accountValid = computed(() => {
  if (!namePattern.test(createForm.account)) return false
  if (createForm.account_mode === 'create' && !createForm.password) return false
  if (createForm.remote_access && !remoteAddressValid(createForm.remote_address)) return false
  return true
})
const remoteAllWarning = computed(() => createForm.remote_access && createForm.remote_address.trim() === '0.0.0.0/0')

function remoteAddressValid(value: string): boolean {
  const addr = value.trim()
  if (['all', 'samehost', 'samenet'].includes(addr)) return true
  if (/^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/.test(addr)) return true
  return /^[0-9a-fA-F:]+(\/\d{1,3})?$/.test(addr)
}

function normalizeAccountName(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9_]/g, '_')
    .replace(/^[^a-z_]+/, '')
    .slice(0, 63)
}

watch(
  () => createForm.database,
  (value) => {
    if (createForm.account_mode !== 'create') return
    const suggested = normalizeAccountName(value)
    if (!createForm.account || createForm.account === autoAccount.value) {
      createForm.account = suggested
      autoAccount.value = suggested
    }
  },
)

watch(
  () => createForm.account_mode,
  (mode) => {
    if (mode === 'create' && !createForm.account) {
      createForm.account = normalizeAccountName(createForm.database)
      autoAccount.value = createForm.account
    }
  },
)

const accountOptions = computed(() =>
  accounts.value
    .filter((a) => a.can_login && !a.protected)
    .map((a) => ({ label: a.name, value: a.name })),
)

function generatePassword(reveal = false) {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*'
  const values = new Uint32Array(18)
  crypto.getRandomValues(values)
  let password = ''
  for (const value of values) password += chars[value % chars.length]
  createForm.password = password
  if (reveal) showPassword.value = true
}

async function openCreate() {
  if (!store.selectedId) {
    message.warning(t('databases.selectInstance'))
    return
  }
  createForm.database = ''
  createForm.account_mode = 'create'
  createForm.account = ''
  createForm.password = ''
  createForm.permission = 'read_write'
  createForm.remote_access = false
  createForm.remote_address = ''
  createResult.value = null
  showPassword.value = false
  autoAccount.value = ''
  currentStep.value = 1
  generatePassword(false)
  showCreate.value = true
  try {
    accounts.value = await accountsApi.list(store.selectedId)
  } catch {
    accounts.value = []
  }
}

async function submitCreate() {
  creating.value = true
  try {
    const res = await databasesApi.create(store.selectedId, {
      database: createForm.database,
      account_mode: createForm.account_mode,
      account: createForm.account,
      password: createForm.password,
      permission: createForm.permission,
      remote_access: createForm.remote_access,
      remote_address: createForm.remote_access ? createForm.remote_address.trim() : '',
    })
    createResult.value = res
    currentStep.value = 3
    selectedHost.value = res.host
    await loadHostOptions(res.host)
  } catch (e) {
    handleError(e)
  } finally {
    creating.value = false
  }
}

async function loadHostOptions(defaultHost: string) {
  const hosts = new Set<string>([defaultHost])
  try {
    const addrs = await instancesApi.systemAddresses()
    for (const a of addrs) hosts.add(a)
  } catch {
    // ignore
  }
  hostOptions.value = Array.from(hosts)
}

function formatHost(host: string): string {
  if (host.includes(':') && !host.startsWith('[')) return `[${host}]`
  return host
}

function buildConnectionString(password: string | null): string {
  if (!createResult.value) return ''
  const result = createResult.value
  const query = new URLSearchParams()
  if (result.unix_socket) query.set('host', result.unix_socket)
  if (result.ssl_mode) query.set('sslmode', result.ssl_mode)
  const user = encodeURIComponent(result.account)
  const database = encodeURIComponent(result.database)
  const auth = password ? `${user}:${encodeURIComponent(password)}@` : `${user}@`
  const authority = result.unix_socket
    ? ''
    : `${formatHost(selectedHost.value || result.host)}:${result.port}`
  const suffix = query.toString() ? `?${query.toString()}` : ''
  return `postgresql://${auth}${authority}/${database}${suffix}`
}

const displayConnectionString = computed(() => {
  if (!createResult.value) return ''
  if (createForm.account_mode !== 'create') return buildConnectionString(null)
  return buildConnectionString(showPassword.value ? createForm.password : '********')
})

const actualConnectionString = computed(() => {
  if (!createResult.value) return ''
  if (createForm.account_mode !== 'create') return buildConnectionString(null)
  return buildConnectionString(createForm.password)
})

const resultHost = computed(() => {
  if (!createResult.value) return ''
  return createResult.value.unix_socket || selectedHost.value || createResult.value.host
})

function sslModeLabel(value: string) {
  switch (value) {
    case 'disable':
      return t('instances.sslDisable')
    case 'prefer':
      return t('instances.sslPrefer')
    case 'require':
      return t('instances.sslRequire')
    case 'verify-ca':
      return t('instances.sslVerifyCA')
    case 'verify-full':
      return t('instances.sslVerifyFull')
    default:
      return value || 'prefer'
  }
}

async function copyUrl() {
  const ok = await copyText(actualConnectionString.value)
  if (ok) message.success(t('common.copied'))
  else message.error(t('common.failed'))
}

async function copyPassword() {
  const ok = await copyText(createForm.password)
  if (ok) message.success(t('common.copied'))
  else message.error(t('common.failed'))
}

function closeCreate() {
  showCreate.value = false
  createResult.value = null
  currentStep.value = 1
  load()
}

function nextStep() {
  if (currentStep.value === 1 && !basicValid.value) {
    message.warning(t('errors.invalid_name'))
    return
  }
  if (currentStep.value === 2 && !accountValid.value) {
    message.warning(t('errors.invalid_name'))
    return
  }
  currentStep.value = Math.min(3, currentStep.value + 1)
}

function prevStep() {
  currentStep.value = Math.max(1, currentStep.value - 1)
}

function configureBackup() {
  closeCreate()
  router.push('/policies')
}

// ---- delete flow ----
const deleting = ref<Database | null>(null)
const confirmName = ref('')

function openDelete(row: Database) {
  deleting.value = row
  confirmName.value = ''
}

async function doDelete() {
  if (!deleting.value || !store.selectedId) return
  if (confirmName.value !== deleting.value.name) {
    message.warning(t('databases.deleteTypeName', { name: deleting.value.name }))
    return
  }
  try {
    await databasesApi.remove(store.selectedId, deleting.value.name)
    message.success(t('common.success'))
    deleting.value = null
    await load()
  } catch (e) {
    handleError(e)
  }
}

const columns: DataTableColumns<Database> = [
  {
    title: t('databases.name'),
    key: 'name',
    render: (row) =>
      h(NSpace, { size: 6, align: 'center' }, () => [
        row.name,
        isSystemDatabase(row.name)
          ? h(NTag, { size: 'small', type: 'warning' }, () => t('databases.systemDatabase'))
          : null,
      ]),
  },
  { title: t('databases.owner'), key: 'owner' },
  { title: t('databases.encoding'), key: 'encoding' },
  { title: t('databases.collation'), key: 'collation' },
  { title: t('databases.size'), key: 'size_bytes', render: (row) => formatSize(row.size_bytes) },
  { title: t('databases.connCount'), key: 'conn_count' },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row) =>
      h(
        NButton,
        { size: 'small', type: 'error', tertiary: true, onClick: () => openDelete(row) },
        () => t('common.delete'),
      ),
  },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <InstanceSelector />
      <n-button type="primary" :disabled="!store.selectedId" @click="openCreate">
        {{ t('databases.create') }}
      </n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
      <n-checkbox v-model:checked="hideSystem">{{ t('databases.hideSystem') }}</n-checkbox>
    </div>

    <n-empty v-if="!store.selectedId" :description="t('databases.noInstanceHint')">
      <template #extra>
        <n-button type="primary" @click="router.push('/instances')">
          {{ t('overview.addInstance') }}
        </n-button>
      </template>
    </n-empty>
    <n-data-table
      v-else
      :columns="columns"
      :data="visibleDatabases"
      :loading="loading"
      :empty-description="t('common.empty')"
    />

    <n-modal
      :show="showCreate"
      @update:show="(v) => { if (!v) closeCreate() }"
      preset="card"
      :title="t('databases.create')"
      style="width: 560px"
      :mask-closable="!createResult"
      :closable="!createResult"
    >
      <n-steps :current="createResult ? 3 : currentStep" size="small" style="margin-bottom: 20px">
        <n-step :title="t('databases.stepBasic')" />
        <n-step :title="t('databases.stepAccount')" />
        <n-step :title="t('databases.stepResult')" />
      </n-steps>

      <template v-if="!createResult">
        <n-form v-if="currentStep === 1" label-placement="left" label-width="120">
          <n-form-item :label="t('databases.name')" required>
            <n-input v-model:value="createForm.database" placeholder="my_database" />
            <div class="hint">{{ t('databases.nameHint') }}</div>
          </n-form-item>
          <n-alert type="info" :show-icon="false" style="margin-bottom: 12px">
            {{ t('databases.instanceLabel') }}：{{ store.selected?.name || '-' }}
          </n-alert>
          <n-alert v-if="createForm.database" type="default" :show-icon="false">
            {{ t('databases.accountWillBe', { name: normalizeAccountName(createForm.database) || '-' }) }}
          </n-alert>
        </n-form>

        <n-form v-else label-placement="left" label-width="120">
          <n-form-item :label="t('databases.accountSettings')">
            <n-radio-group v-model:value="createForm.account_mode">
              <n-radio value="create">{{ t('databases.createNewAccount') }}</n-radio>
              <n-radio value="existing">{{ t('databases.useExistingAccount') }}</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item :label="t('databases.account')" required>
            <n-select
              v-if="createForm.account_mode === 'existing'"
              v-model:value="createForm.account"
              :options="accountOptions"
              :placeholder="t('databases.chooseAccount')"
            />
            <n-input
              v-else
              v-model:value="createForm.account"
              placeholder="my_database_user"
            />
          </n-form-item>
          <n-form-item
            v-if="createForm.account_mode === 'create'"
            :label="t('databases.password')"
            required
          >
            <n-input
              v-model:value="createForm.password"
              type="password"
              show-password-on="click"
            >
              <template #suffix>
                <n-button text type="primary" size="tiny" @click="generatePassword(true)">
                  {{ t('databases.generatePassword') }}
                </n-button>
              </template>
            </n-input>
          </n-form-item>
          <n-form-item
            v-else
            :label="t('databases.password')"
            label-placement="left"
          >
            <span class="hint">{{ t('databases.existingAccountHint') }}</span>
          </n-form-item>
          <n-form-item :label="t('databases.permission')" required>
            <n-radio-group v-model:value="createForm.permission">
              <n-radio value="read_write">{{ t('databases.permissionReadWrite') }}</n-radio>
              <n-radio value="read_only">{{ t('databases.permissionReadOnly') }}</n-radio>
              <n-radio value="owner">{{ t('databases.permissionOwner') }}</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-alert v-if="createForm.permission === 'owner'" type="warning" style="margin-bottom: 8px">
            {{ t('databases.ownerRisk') }}
          </n-alert>
          <n-form-item :label="t('databases.remoteAccess')">
            <n-switch v-model:value="createForm.remote_access" />
          </n-form-item>
          <n-form-item
            v-if="createForm.remote_access"
            :label="t('databases.remoteAddress')"
            required
          >
            <n-input
              v-model:value="createForm.remote_address"
              :placeholder="t('databases.remoteAddressPlaceholder')"
            />
          </n-form-item>
          <n-alert v-if="remoteAllWarning" type="warning" style="margin-bottom: 8px">
            {{ t('databases.remoteAllWarning') }}
          </n-alert>
          <n-alert v-else-if="createForm.remote_access" type="info" style="margin-bottom: 8px">
            {{ t('databases.remoteAccessHint') }}
          </n-alert>
        </n-form>
      </template>

      <template v-else>
        <n-alert type="success" :title="t('databases.createSuccess')" style="margin-bottom: 16px" />
        <n-form-item
          v-if="!createResult.unix_socket"
          :label="t('databases.host')"
          label-placement="left"
          style="margin-bottom: 12px"
        >
          <n-select
            v-model:value="selectedHost"
            :options="hostOptions.map((h) => ({ label: h, value: h }))"
            style="max-width: 320px"
          />
        </n-form-item>
        <n-descriptions bordered :column="2">
          <n-descriptions-item :label="t('databases.database')">
            {{ createResult.database }}
          </n-descriptions-item>
          <n-descriptions-item :label="t('databases.account')">
            {{ createResult.account }}
          </n-descriptions-item>
          <n-descriptions-item :label="createResult.unix_socket ? t('databases.unixSocket') : t('databases.host')">
            {{ resultHost }}
          </n-descriptions-item>
          <n-descriptions-item :label="t('databases.port')">{{ createResult.port }}</n-descriptions-item>
          <n-descriptions-item :label="t('databases.sslMode')">
            {{ sslModeLabel(createResult.ssl_mode) }}
          </n-descriptions-item>
          <n-descriptions-item :label="t('databases.permission')">
            {{ createForm.permission === 'owner'
              ? t('databases.permissionOwner')
              : createForm.permission === 'read_write'
                ? t('databases.permissionReadWrite')
                : t('databases.permissionReadOnly') }}
          </n-descriptions-item>
          <n-descriptions-item :label="t('databases.remoteAccess')">
            {{ createResult.remote_access ? t('databases.remoteEnabled') : t('databases.remoteDisabled') }}
          </n-descriptions-item>
        </n-descriptions>

        <n-alert
          v-if="createResult.remote_access"
          :type="createResult.firewall_warning ? 'warning' : 'success'"
          style="margin-top: 12px"
          :title="createResult.firewall_warning
            ? t('databases.remoteWarning', { warning: createResult.firewall_warning })
            : t('databases.remoteEnabled')"
        >
          <div v-if="!createResult.firewall_warning" class="mono">{{ createResult.remote_address }}</div>
        </n-alert>

        <n-alert
          v-if="createForm.account_mode === 'create'"
          type="warning"
          style="margin-top: 12px"
        >
          <template #header>{{ t('databases.savePasswordHint') }}</template>
          <div class="mono">{{ displayConnectionString }}</div>
          <div class="hint" style="margin-top: 6px">{{ t('databases.connectionHint') }}</div>
          <n-space style="margin-top: 8px">
            <n-button size="small" @click="copyUrl">
              {{ t('databases.copyConnectionString') }}
            </n-button>
            <n-button size="small" @click="copyPassword">
              {{ t('databases.copyPassword') }}
            </n-button>
            <n-button size="small" @click="showPassword = !showPassword">
              {{ t('databases.revealPassword') }}
            </n-button>
          </n-space>
        </n-alert>
        <n-alert v-else type="info" style="margin-top: 12px" :title="t('databases.connectionString')">
          <div class="mono">{{ displayConnectionString }}</div>
          <div class="hint" style="margin-top: 6px">{{ t('databases.existingAccountHint') }}</div>
          <n-button size="small" style="margin-top: 8px" @click="copyUrl">
            {{ t('databases.copyConnectionString') }}
          </n-button>
        </n-alert>
      </template>

      <template #footer>
        <n-space justify="end">
          <template v-if="!createResult">
            <n-button @click="closeCreate">{{ t('common.cancel') }}</n-button>
            <n-button v-if="currentStep > 1" @click="prevStep">{{ t('common.back') }}</n-button>
            <n-button v-if="currentStep < 2" type="primary" :disabled="!basicValid" @click="nextStep">
              {{ t('common.next') }}
            </n-button>
            <n-button
              v-else
              type="primary"
              :loading="creating"
              :disabled="!accountValid"
              @click="submitCreate"
            >
              {{ t('common.create') }}
            </n-button>
          </template>
          <template v-else>
            <n-button @click="configureBackup">{{ t('databases.configureBackup') }}</n-button>
            <n-button type="primary" @click="closeCreate">{{ t('common.confirm') }}</n-button>
          </template>
        </n-space>
      </template>
    </n-modal>

    <n-modal
      :show="deleting !== null"
      @update:show="(v) => { if (!v) deleting = null }"
      preset="card"
      :title="t('databases.delete')"
      style="width: 480px"
    >
      <template v-if="deleting">
        <n-alert type="error" :title="t('databases.deleteHint')" style="margin-bottom: 12px" />
        <div style="margin-bottom: 8px">
          {{ t('databases.deleteTypeName', { name: deleting.name }) }}
        </div>
        <n-input v-model:value="confirmName" :placeholder="deleting.name" />
      </template>
      <template #footer>
        <n-space justify="end">
          <n-button @click="deleting = null">{{ t('common.cancel') }}</n-button>
          <n-button type="error" :disabled="confirmName !== deleting?.name" @click="doDelete">
            {{ t('common.delete') }}
          </n-button>
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
.mono {
  font-family: monospace;
  word-break: break-all;
}
.hint {
  font-size: 12px;
  opacity: 0.7;
}
</style>

