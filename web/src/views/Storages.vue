<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NModal,
  NInput,
  NForm,
  NFormItem,
  NSpace,
  NTag,
  NRadioGroup,
  NRadio,
  NSwitch,
  NInputNumber,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { storagesApi, type StorageTarget } from '@/api/storages'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()

const targets = ref<StorageTarget[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    targets.value = await storagesApi.list()
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

const showModal = ref(false)
const editingId = ref<number | null>(null)
const form = reactive({
  name: '',
  type: 'local' as 'local' | 'webdav',
  path: '',
  endpoint: '',
  username: '',
  password: '',
  base_path: '',
  tls_verify: true,
  timeout: 30,
})
const saving = ref(false)
const testing = ref(false)

function openCreate() {
  editingId.value = null
  Object.assign(form, {
    name: '', type: 'local', path: '', endpoint: '', username: '',
    password: '', base_path: '', tls_verify: true, timeout: 30,
  })
  showModal.value = true
}

function openEdit(row: StorageTarget) {
  editingId.value = row.id
  const cfg = row.config || {}
  Object.assign(form, {
    name: row.name,
    type: row.type,
    path: cfg.path || '',
    endpoint: cfg.endpoint || '',
    username: cfg.username || '',
    password: '',
    base_path: cfg.base_path || '',
    tls_verify: cfg.tls_verify !== false,
    timeout: cfg.timeout || 30,
  })
  showModal.value = true
}

function buildPayload() {
  const config =
    form.type === 'local'
      ? { path: form.path }
      : {
          endpoint: form.endpoint,
          username: form.username,
          base_path: form.base_path,
          tls_verify: form.tls_verify,
          timeout: form.timeout,
        }
  return {
    name: form.name,
    type: form.type,
    config,
    password: form.password || undefined,
  }
}

async function save() {
  saving.value = true
  try {
    const payload = buildPayload()
    if (editingId.value) await storagesApi.update(editingId.value, payload)
    else await storagesApi.create(payload)
    message.success(t('common.success'))
    showModal.value = false
    await load()
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function testStorage() {
  testing.value = true
  try {
    const res = await storagesApi.test(buildPayload())
    if (res.ok) message.success(t('storages.testSuccess'))
    else message.error(`${t('storages.testFailed')}: ${res.error || ''}`)
  } catch (e) {
    handleError(e)
  } finally {
    testing.value = false
  }
}

async function remove(row: StorageTarget) {
  try {
    await storagesApi.remove(row.id)
    message.success(t('common.success'))
    await load()
  } catch (e) {
    handleError(e)
  }
}

const columns: DataTableColumns<StorageTarget> = [
  { title: t('storages.name'), key: 'name' },
  {
    title: t('storages.type'),
    key: 'type',
    render: (row) =>
      h(
        NTag,
        { size: 'small' },
        () => (row.type === 'local' ? t('storages.typeLocal') : t('storages.typeWebdav')),
      ),
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 160,
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
      <n-button type="primary" @click="openCreate">{{ t('storages.add') }}</n-button>
      <n-button quaternary @click="load">{{ t('common.refresh') }}</n-button>
    </div>

    <n-data-table
      :columns="columns"
      :data="targets"
      :loading="loading"
      :empty-description="t('common.empty')"
    />

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingId ? t('storages.edit') : t('storages.add')"
      style="width: 520px"
    >
      <n-form label-placement="left" label-width="120">
        <n-form-item :label="t('storages.name')" required>
          <n-input v-model:value="form.name" />
        </n-form-item>
        <n-form-item :label="t('storages.type')">
          <n-radio-group v-model:value="form.type">
            <n-radio value="local">{{ t('storages.typeLocal') }}</n-radio>
            <n-radio value="webdav">{{ t('storages.typeWebdav') }}</n-radio>
          </n-radio-group>
        </n-form-item>

        <template v-if="form.type === 'local'">
          <n-form-item :label="t('storages.path')" required>
            <n-input v-model:value="form.path" placeholder="/var/backups/dbpilot" />
          </n-form-item>
        </template>

        <template v-else>
          <n-form-item :label="t('storages.endpoint')" required>
            <n-input v-model:value="form.endpoint" placeholder="https://example.com/dav/" />
          </n-form-item>
          <n-form-item :label="t('storages.username')">
            <n-input v-model:value="form.username" />
          </n-form-item>
          <n-form-item :label="t('storages.password')">
            <n-input v-model:value="form.password" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item :label="t('storages.basePath')">
            <n-input v-model:value="form.base_path" placeholder="/dbpilot/" />
          </n-form-item>
          <n-form-item :label="t('storages.tlsVerify')">
            <n-switch v-model:value="form.tls_verify" />
          </n-form-item>
          <n-form-item :label="t('storages.timeout')">
            <n-input-number v-model:value="form.timeout" :min="1" :max="600" />
          </n-form-item>
        </template>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="testStorage" :loading="testing">{{ t('storages.test') }}</n-button>
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
</style>
