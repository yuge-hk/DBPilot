<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NMenu,
  NIcon,
  NDropdown,
  NAvatar,
  NButton,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
  ServerOutline,
  GridOutline,
  PeopleOutline,
  CloudUploadOutline,
  TimeOutline,
  SettingsOutline,
  FolderOutline,
  DocumentTextOutline,
  AlbumsOutline,
  MoonOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { api } from '@/api/client'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()
const version = ref('')

onMounted(async () => {
  try {
    const health = await api.get<{ version: string }>('/api/v1/health')
    version.value = health.version
  } catch {
    // version is informational only
  }
})

const menuOptions: MenuOption[] = [
  { label: () => t('menu.overview'), key: 'overview', icon: renderIcon(GridOutline) },
  { label: () => t('menu.instances'), key: 'instances', icon: renderIcon(ServerOutline) },
  { label: () => t('menu.databases'), key: 'databases', icon: renderIcon(AlbumsOutline) },
  { label: () => t('menu.accounts'), key: 'accounts', icon: renderIcon(PeopleOutline) },
  { label: () => t('menu.backups'), key: 'backups', icon: renderIcon(CloudUploadOutline) },
  { label: () => t('menu.policies'), key: 'policies', icon: renderIcon(TimeOutline) },
  { label: () => t('menu.storages'), key: 'storages', icon: renderIcon(FolderOutline) },
  { label: () => t('menu.tasks'), key: 'tasks', icon: renderIcon(TimeOutline) },
  { label: () => t('menu.audit'), key: 'audit', icon: renderIcon(DocumentTextOutline) },
  { label: () => t('menu.settings'), key: 'settings', icon: renderIcon(SettingsOutline) },
]

function renderIcon(icon: any) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const activeKey = computed(() => (route.name as string) || 'overview')

const userOptions = [
  {
    label: () => t('auth.changePassword'),
    key: 'password',
  },
  {
    label: () => t('auth.logout'),
    key: 'logout',
  },
]

async function handleUserSelect(key: string) {
  if (key === 'logout') {
    await auth.logout()
    router.push('/login')
  } else if (key === 'password') {
    router.push('/settings')
  }
}

function onMenuSelect(key: string) {
  router.push({ name: key })
}
</script>

<template>
  <n-layout has-sider style="height: 100vh">
    <n-layout-sider bordered collapse-mode="width" :width="220" :collapsed-width="64">
      <div class="sider-inner">
        <div class="logo">
          <span class="logo-text">{{ t('app.name') }}</span>
        </div>
        <n-menu
          :value="activeKey"
          :options="menuOptions"
          @update:value="onMenuSelect"
        />
        <div class="sider-footer">
          <span v-if="version">v{{ version }}</span>
        </div>
      </div>
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered class="header">
        <div class="header-title">{{ t(route.meta.title as string) }}</div>
        <div class="header-actions">
          <n-button quaternary circle @click="theme.toggle">
            <template #icon>
              <n-icon><component :is="theme.dark ? SunnyOutline : MoonOutline" /></n-icon>
            </template>
          </n-button>
          <n-dropdown :options="userOptions" @select="handleUserSelect">
            <div class="user">
              <n-avatar round size="small">{{ auth.username.charAt(0).toUpperCase() }}</n-avatar>
              <span class="username">{{ auth.username }}</span>
            </div>
          </n-dropdown>
        </div>
      </n-layout-header>
      <n-layout-content class="content">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<style scoped>
.sider-inner {
  height: 100%;
  display: flex;
  flex-direction: column;
}
.logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 18px;
}
.sider-footer {
  margin-top: auto;
  padding: 12px 20px;
  font-size: 12px;
  opacity: 0.55;
  text-align: center;
}
.header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}
.header-title {
  font-size: 16px;
  font-weight: 600;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.user {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.username {
  font-size: 14px;
}
.content {
  padding: 20px;
  overflow: auto;
}
</style>
