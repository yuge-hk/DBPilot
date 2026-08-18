<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue'
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
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { accountsApi, type Account, type AccountPayload } from '@/api/accounts'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'
import InstanceSelector from '@/components/InstanceSelector.vue'

const { t } = useI18n()
const message = useMessage()
const store = useInstanceStore()

const accounts = ref<Account[]>([])
const loading = ref(false)

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
    width: 170,
    render: (row) =>
      h(NSpace, { size: 4 }, () => [
        h(NButton, { size: 'small', onClick: () => openEdit(row) }, () => t('common.edit')),
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
    </div>

    <n-data-table
      :columns="columns"
      :data="accounts"
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
</style>
