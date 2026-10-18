import type { RouteRecordRaw } from 'vue-router'

export { ChatImageNavIcon } from './navIcon'

export const chatImageRoutes: RouteRecordRaw[] = [
  {
    path: '/chat-image',
    name: 'ChatImage',
    component: () => import('./ChatImageView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: '对话画图'
    }
  }
]
