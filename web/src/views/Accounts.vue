<script setup lang="ts">
import { h, onMounted, reactive, ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NModal,
  NInput,
  NInputNumber,
  NForm,
  NFormItem,
  NSwitch,
  NTag,
  NSpace,
  NDatePicker,
  NSelect,
  NRadioGroup,
  NRadio,
  NDivider,
  NCheckbox,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { accountsApi, type Account, type AccountPayload, type DatabasePrivilege } from '@/api/accounts'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'
import InstanceSelector from '@/components/InstanceSelector.vue'

const { t } = useI18n()
const message = useMessage()
const store = useInstanceStore()

const accounts = ref<Account[]>([])
const loading = ref(false)
const hideSystem = ref(true)

const filteredAccounts = computed(() =>
  hideSystem.value ? accounts.value.filter((a) => !a.system_role) : accounts.value,
)

async function load() {
  if (!store.selectedId) return
  loading.value = true
  try {
    accounts.value = await accountsApi.list(store.selectedId)
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
watch(() => store.selectedId, () => load())

// ---- privileges management ----
const showPriv = ref(false)
const privAccount = ref<Account | null>(null)
const privileges = ref<DatabasePrivilege[]>([])
const privForm = reactive({
  database: '',
  permission: 'read_write' as 'owner' | 'read_write' | 'read_only',
})
const granting = ref(false)

const dbOptions = computed(() => privileges.value.map((d) => ({ label: d.database, value: d.database })))

const levelLabel = (level: string) => {
  switch (level) {
    case 'owner':
      return t('databases.permissionOwner')
    case 'read_write':
      return t('databases.permissionReadWrite')
    case 'read_only':
      return t('databases.permissionReadOnly')
    default:
      return t('accounts.noPrivilege')
  }
}

const levelType = (level: string) => {
  switch (level) {
    case 'owner':
      return 'success'
    case 'read_write':
      return 'info'
    case 'read_only':
      return 'warning'
    default:
      return 'default'
  }
}

async function openPrivileges(row: Account) {
  privAccount.value = row
  privForm.database = ''
  privForm.permission = 'read_write'
  showPriv.value = true
  await loadPrivileges()
}

async function loadPrivileges() {
  if (!privAccount.value) return
  try {
    privileges.value = await accountsApi.listPrivileges(store.selectedId, privAccount.value.name)
  } catch {
    privileges.value = []
  }
}

async function grantPriv() {
  if (!privAccount.value || !privForm.database) {
    message.warning(t('accounts.selectDatabase'))
    return
  }
  granting.value = true
  try {
    await accountsApi.grantPrivileges(store.selectedId, privAccount.value.name, privForm.database, privForm.permission)
    message.success(t('common.success'))
    await loadPrivileges()
  } catch (e) {
    handleError(e)
  } finally {
    granting.value = false
  }
}

async function revokePriv(database: string) {
  if (!privAccount.value) return
  try {
    await accountsApi.revokePrivileges(store.selectedId, privAccount.value.name, database)
    message.success(t('common.success'))
    await loadPrivileges()
  } catch (e) {
    handleError(e)
  }
}

function isSystemAccount(row: Account) {
  return row.system_role || row.name === 'postgres'
}

// ---- create / edit modal ----
const showModal = ref(false)
const editing = ref<Account | null>(null)
const form = reactive({
  name: '',
  password: '',
  login: true,
  superuser: false,
  create_db: false,
  create_role: false,
  conn_limit: -1,
  valid_until: null as number | null,
})
const saving = ref(false)

function openCreate() {
  editing.value = null
  Object.assign(form, {
    name: '', password: '', login: true, superuser: false,
    create_db: false, create_role: false, conn_limit: -1, valid_until: null,
  })
  showModal.value = true
}

function openEdit(row: Account) {
  editing.value = row
  Object.assign(form, {
    name: row.name, password: '', login: row.can_login, superuser: row.superuser,
    create_db: row.create_db, create_role: row.create_role, conn_limit: row.conn_limit,
    valid_until: row.valid_until ? new Date(row.valid_until).getTime() : null,
  })
  showModal.value = true
}

function buildPayload(): AccountPayload {
  const payload: AccountPayload = {}
  if (!editing.value) {
    payload.name = form.name
    payload.password = form.password
  } else if (form.password) {
    payload.password = form.password
  }
  payload.login = form.login
  payload.superuser = form.superuser
  payload.create_db = form.create_db
  payload.create_role = form.create_role
  payload.conn_limit = form.conn_limit
  payload.valid_until = form.valid_until ? new Date(form.valid_until).toISOString() : ''
  return payload
}

async function save() {
  if (!store.selectedId) return
  if (!editing.value && !form.password) {
    message.warning(t('accounts.password'))
    return
  }
  saving.value = true
  try {
    const payload = buildPayload()
    if (editing.value) {
      await accountsApi.update(store.selectedId, editing.value.name, payload)
    } else {
      await accountsApi.create(store.selectedId, payload)
    }
    message.success(t('common.success'))
    showModal.value = false
    await load()
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function remove(row: Account) {
  if (!store.selectedId) return
  try {
    await accountsApi.remove(store.selectedId, row.name)
    message.success(t('common.success'))
    await load()
  } catch (e) {
    handleError(e)
  }
}

function connLimitLabel(v: number) {
  return v < 0 ? t('accounts.unlimited') : String(v)
}

function validUntilLabel(v: string) {
  return v ? v.replace('T', ' ').slice(0, 19) : t('accounts.never')
}

const columns: DataTableColumns<Account> = [
  {
    title: t('accounts.name'),
    key: 'name',
    render: (row) =>
      h(NSpace, { size: 6, align: 'center' }, () => [
        row.name,
        row.system_role ? h(NTag, { size: 'small', type: 'warning' }, () => t('accounts.systemRole')) : null,
      ]),
  },
  {
    title: t('accounts.canLogin'),
    key: 'can_login',
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.can_login ? 'success' : 'default' },
        () => (row.can_login ? t('accounts.loginEnabled') : t('accounts.loginDisabled')),
      ),
  },
  {
    title: t('accounts.superuser'),
    key: 'superuser',
    render: (row) => (row.superuser ? '✓' : ''),
  },
  {
    title: t('accounts.createDb'),
    key: 'create_db',
    render: (row) => (row.create_db ? '✓' : ''),
  },
  {
    title: t('accounts.createRole'),
    key: 'create_role',
    render: (row) => (row.create_role ? '✓' : ''),
  },
  { title: t('accounts.connLimit'), key: 'conn_limit', render: (row) => connLimitLabel(row.conn_limit) },
  { title: t('accounts.validUntil'), key: 'valid_until', render: (row) => validUntilLabel(row.valid_until) },
  { title: t('accounts.ownedDbs'), key: 'owned_dbs' },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 240,
    render: (row) =>
      h(NSpace, { size: 4 }, () => [
        h(NButton, { size: 'small', onClick: () => openEdit(row) }, () => t('common.edit')),
        h(
          NButton,
          { size: 'small', onClick: () => openPrivileges(row), disabled: isSystemAccount(row) },
          () => t('accounts.privileges'),
        ),
        row.system_role
          ? h(NButton, { size: 'small', type: 'error', tertiary: true, disabled: true }, () => t('common.delete'))
          : h(NButton, { size: 'small', type: 'error', tertiary: true, onClick: () => remove(row) }, () => t('common.delete')),
      ]),
  },
]
</script>

<template>
  <div>
    <div class="toolbar">
      <InstanceSelector />
      <n-button type="primary" @click="openCreate">{{ t('accounts.create') }}</n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
      <n-checkbox v-model:checked="hideSystem">{{ t('accounts.hideSystem') }}</n-checkbox>
    </div>

    <n-data-table
      :columns="columns"
      :data="filteredAccounts"
      :loading="loading"
      :empty-description="t('common.empty')"
    />

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editing ? t('accounts.edit') : t('accounts.create')"
      style="width: 520px"
    >
      <n-form label-placement="left" label-width="120">
        <n-form-item v-if="!editing" :label="t('accounts.name')" required>
          <n-input v-model:value="form.name" :placeholder="t('accounts.name')" />
        </n-form-item>
        <n-form-item :label="t('accounts.password')" :required="!editing">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="editing ? t('instances.passwordPlaceholder') : t('accounts.password')"
          />
        </n-form-item>
        <n-form-item :label="t('accounts.canLogin')">
          <n-switch v-model:value="form.login" />
        </n-form-item>
        <n-form-item :label="t('accounts.superuser')">
          <n-switch v-model:value="form.superuser" />
        </n-form-item>
        <n-form-item :label="t('accounts.createDb')">
          <n-switch v-model:value="form.create_db" />
        </n-form-item>
        <n-form-item :label="t('accounts.createRole')">
          <n-switch v-model:value="form.create_role" />
        </n-form-item>
        <n-form-item :label="t('accounts.connLimit')">
          <n-input-number v-model:value="form.conn_limit" :min="-1" />
          <span class="hint">{{ t('accounts.unlimited') }}: -1</span>
        </n-form-item>
        <n-form-item :label="t('accounts.validUntil')">
          <n-date-picker v-model:value="form.valid_until" type="datetime" clearable />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="save" :loading="saving">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal
      v-model:show="showPriv"
      preset="card"
      :title="`${t('accounts.managePrivileges')} - ${privAccount?.name ?? ''}`"
      style="width: 520px"
    >
      <n-form label-placement="left" label-width="100">
        <n-form-item :label="t('databases.name')">
          <n-select
            v-model:value="privForm.database"
            :options="dbOptions"
            :placeholder="t('accounts.selectDatabase')"
          />
        </n-form-item>
        <n-form-item :label="t('accounts.permission')">
          <n-radio-group v-model:value="privForm.permission">
            <n-radio value="owner">{{ t('databases.permissionOwner') }}</n-radio>
            <n-radio value="read_write">{{ t('databases.permissionReadWrite') }}</n-radio>
            <n-radio value="read_only">{{ t('databases.permissionReadOnly') }}</n-radio>
          </n-radio-group>
        </n-form-item>
      </n-form>
      <n-button type="primary" @click="grantPriv" :loading="granting" :disabled="!privForm.database">
        {{ t('accounts.grant') }}
      </n-button>

      <n-divider />

      <div class="priv-list-label">{{ t('accounts.privileges') }}</div>
      <n-data-table
        :columns="[
          { title: t('databases.name'), key: 'database' },
          {
            title: t('accounts.permission'),
            key: 'level',
            width: 140,
            render: (row: DatabasePrivilege) =>
              h(NTag, { size: 'small', type: levelType(row.level) }, () => levelLabel(row.level)),
          },
          {
            title: t('common.actions'),
            key: 'actions',
            width: 90,
            render: (row: DatabasePrivilege) =>
              row.level !== 'none'
                ? h(
                    NButton,
                    { size: 'tiny', type: 'error', tertiary: true, onClick: () => revokePriv(row.database) },
                    () => t('accounts.revoke'),
                  )
                : '',
          },
        ]"
        :data="privileges"
        size="small"
        :bordered="false"
      />
    </n-modal>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
  align-items: center;
}
.hint {
  margin-left: 8px;
  font-size: 12px;
  opacity: 0.6;
}
.priv-list-label {
  font-size: 13px;
  margin-bottom: 8px;
  opacity: 0.8;
}
.priv-row {
  padding: 4px 0;
  border-bottom: 1px solid rgba(128, 128, 128, 0.15);
}
</style>
