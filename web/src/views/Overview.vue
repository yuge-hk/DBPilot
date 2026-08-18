<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NCard,
  NGrid,
  NGi,
  NStatistic,
  NTag,
  NButton,
  NEmpty,
} from 'naive-ui'
import { useRouter } from 'vue-router'
import { overviewApi, type InstanceSummary } from '@/api/overview'
import { useInstanceStore } from '@/stores/instance'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const router = useRouter()
const store = useInstanceStore()

const summaries = ref<InstanceSummary[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    summaries.value = await overviewApi.get()
  } catch (e) {
    if (e instanceof ApiRequestError) {
      /* ignore */
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await store.load()
  await load()
})

function backupStatusTag(s: InstanceSummary) {
  if (!s.last_backup_at) return h(NTag, { size: 'small' }, () => t('overview.noBackup'))
  const type = s.last_backup_status === 'success' ? 'success' : 'error'
  return h(NTag, { size: 'small', type }, () => s.last_backup_at.replace('T', ' ').slice(0, 16))
}
</script>

<template>
  <div>
    <n-empty v-if="!loading && summaries.length === 0" :description="t('overview.quickStartHint')">
      <template #extra>
        <n-button type="primary" @click="router.push('/instances')">
          {{ t('overview.addInstance') }}
        </n-button>
      </template>
    </n-empty>

    <div v-for="s in summaries" :key="s.id" class="instance-card">
      <n-card>
        <template #header>
          <div class="card-header">
            <span class="instance-name">
              {{ s.name }}
              <span class="dot" :style="{ background: s.connected ? '#18a058' : '#d03050' }" />
            </span>
            <span class="status-text">
              {{ s.connected ? t('overview.connected') : t('overview.disconnected') }}
            </span>
          </div>
        </template>
        <n-grid :cols="4" :x-gap="16">
          <n-gi>
            <n-statistic :label="t('overview.databaseCount')" :value="s.database_count" />
          </n-gi>
          <n-gi>
            <n-statistic :label="t('overview.accountCount')" :value="s.account_count" />
          </n-gi>
          <n-gi>
            <n-statistic :label="t('overview.backupProtected')" :value="`${s.protected_count} / ${s.database_count}`" />
          </n-gi>
          <n-gi>
            <n-statistic :label="t('overview.backupUnprotected')" :value="s.unprotected_count" />
          </n-gi>
        </n-grid>
        <div class="last-backup">
          {{ t('overview.lastBackup') }}：
          <component :is="backupStatusTag(s)" />
          <span v-if="s.recent_failed > 0" class="failed-count">
            {{ t('overview.failedBackups') }}: {{ s.recent_failed }}
          </span>
        </div>
      </n-card>
    </div>
  </div>
</template>

<style scoped>
.instance-card {
  margin-bottom: 16px;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.instance-name {
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.status-text {
  font-size: 13px;
  opacity: 0.7;
}
.last-backup {
  margin-top: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.failed-count {
  color: #d03050;
  font-size: 13px;
}
</style>
