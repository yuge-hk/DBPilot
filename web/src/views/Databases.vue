<script setup lang="ts">
import { h, onMounted, ref, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
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
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { databasesApi, type Database, type CreateDatabaseResult } from '@/api/databases'
import { accountsApi, type Account } from '@/api/accounts'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'
import InstanceSelector from '@/components/InstanceSelector.vue'

const { t } = useI18n()
const message = useMessage()
const store = useInstanceStore()

const databases = ref<Database[]>([])
const loading = ref(false)

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
  if (e instanceof ApiRequestError) message.error(t(`errors.${e.code}`))
  else message.error(t('common.failed'))
}

onMounted(() => load())

// ---- create database (one-click) ----
const showCreate = ref(false)
const creating = ref(false)
const accounts = ref<Account[]>([])
const createForm = reactive({
  database: '',
  account_mode: 'create' as 'create' | 'existing',
  account: '',
  password: '',
  permission: 'owner' as 'owner' | 'read_write' | 'read_only',
})
const createResult = ref<CreateDatabaseResult | null>(null)
const showPassword = ref(false)

const accountOptions = computed(() =>
  accounts.value
    .filter((a) => a.can_login && !a.system_role)
    .map((a) => ({ label: a.name, value: a.name })),
)

async function openCreate() {
  createForm.database = ''
  createForm.account_mode = 'create'
  createForm.account = ''
  createForm.password = ''
  createForm.permission = 'owner'
  createResult.value = null
  showPassword.value = false
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
    })
    createResult.value = res
  } catch (e) {
    handleError(e)
  } finally {
    creating.value = false
  }
}

function buildUrl(): string {
  if (!createResult.value) return ''
  const { account, host, port, database } = createResult.value
  const pw = createForm.password
  return `postgresql://${account}:${pw ? '********' : ''}@${host}:${port}/${database}`
}

function buildUrlRevealed(): string {
  if (!createResult.value) return ''
  const { account, host, port, database } = createResult.value
  return `postgresql://${account}:${createForm.password}@${host}:${port}/${database}`
}

async function copyUrl(revealed: boolean) {
  try {
    await navigator.clipboard.writeText(revealed ? buildUrlRevealed() : buildUrl())
    message.success(t('common.copied'))
  } catch {
    message.error(t('common.failed'))
  }
}

function closeCreate() {
  showCreate.value = false
  createResult.value = null
  load()
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
  { title: t('databases.name'), key: 'name' },
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
      <n-button type="primary" @click="openCreate">{{ t('databases.create') }}</n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="databases"
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
      <template v-if="!createResult">
        <n-form label-placement="left" label-width="120">
          <n-form-item :label="t('databases.name')" required>
            <n-input v-model:value="createForm.database" placeholder="musiccopier" />
          </n-form-item>
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
              placeholder="musiccopier"
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
            />
          </n-form-item>
          <n-form-item :label="t('databases.permission')" required>
            <n-radio-group v-model:value="createForm.permission">
              <n-radio value="owner">{{ t('databases.permissionOwner') }}</n-radio>
              <n-radio value="read_write">{{ t('databases.permissionReadWrite') }}</n-radio>
              <n-radio value="read_only">{{ t('databases.permissionReadOnly') }}</n-radio>
            </n-radio-group>
          </n-form-item>
        </n-form>
      </template>

      <template v-else>
        <n-alert type="success" :title="t('databases.createSuccess')" style="margin-bottom: 16px" />
        <n-descriptions bordered :column="2">
          <n-descriptions-item :label="t('databases.database')">
            {{ createResult.database }}
          </n-descriptions-item>
          <n-descriptions-item :label="t('databases.account')">
            {{ createResult.account }}
          </n-descriptions-item>
          <n-descriptions-item label="Host">{{ createResult.host }}</n-descriptions-item>
          <n-descriptions-item label="Port">{{ createResult.port }}</n-descriptions-item>
        </n-descriptions>

        <n-alert
          v-if="createForm.account_mode === 'create'"
          type="warning"
          style="margin-top: 12px"
        >
          <template #header>{{ t('databases.savePasswordHint') }}</template>
          <div class="mono">{{ showPassword ? buildUrlRevealed() : buildUrl() }}</div>
          <n-space style="margin-top: 8px">
            <n-button size="small" @click="copyUrl(showPassword)">
              {{ t('databases.copyConnectionString') }}
            </n-button>
            <n-button size="small" @click="showPassword = !showPassword">
              {{ t('databases.revealPassword') }}
            </n-button>
          </n-space>
        </n-alert>
        <n-alert v-else type="info" style="margin-top: 12px" :title="t('databases.connectionString')">
          <div class="mono">
            postgresql://{{ createResult.account }}:******@{{ createResult.host }}:{{ createResult.port }}/{{ createResult.database }}
          </div>
        </n-alert>
      </template>

      <template #footer>
        <n-space justify="end">
          <template v-if="!createResult">
            <n-button @click="closeCreate">{{ t('common.cancel') }}</n-button>
            <n-button type="primary" @click="submitCreate" :loading="creating">
              {{ t('common.create') }}
            </n-button>
          </template>
          <template v-else>
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
</style>

