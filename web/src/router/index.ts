import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/Login.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      children: [
        {
          path: '',
          name: 'overview',
          component: () => import('@/views/Overview.vue'),
          meta: { title: 'overview.title' },
        },
        {
          path: 'instances',
          name: 'instances',
          component: () => import('@/views/Instances.vue'),
          meta: { title: 'menu.instances' },
        },
        {
          path: 'databases',
          name: 'databases',
          component: () => import('@/views/Databases.vue'),
          meta: { title: 'menu.databases' },
        },
        {
          path: 'accounts',
          name: 'accounts',
          component: () => import('@/views/Accounts.vue'),
          meta: { title: 'menu.accounts' },
        },
        {
          path: 'backups',
          name: 'backups',
          component: () => import('@/views/Backups.vue'),
          meta: { title: 'menu.backups' },
        },
        {
          path: 'policies',
          name: 'policies',
          component: () => import('@/views/Policies.vue'),
          meta: { title: 'menu.policies' },
        },
        {
          path: 'storages',
          name: 'storages',
          component: () => import('@/views/Storages.vue'),
          meta: { title: 'menu.storages' },
        },
        {
          path: 'tasks',
          name: 'tasks',
          component: () => import('@/views/Tasks.vue'),
          meta: { title: 'menu.tasks' },
        },
        {
          path: 'audit',
          name: 'audit',
          component: () => import('@/views/Audit.vue'),
          meta: { title: 'menu.audit' },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/Settings.vue'),
          meta: { title: 'menu.settings' },
        },
      ],
    },
  ],
})

export default router
