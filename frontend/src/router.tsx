import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'

import App from '@/App'

const rootRoute = createRootRoute({
  component: App,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/dashboard' })
  },
})

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/dashboard',
})

const logsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/logs',
})

const agentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/agents',
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  beforeLoad: () => {
    throw redirect({ to: '/settings/setting' })
  },
})

const settingsSettingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/setting',
})

const settingsSoulRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/soul',
})

const settingsTelegramRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/telegram',
})

const settingsBehaviorRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/behavior',
})

const settingsVariablesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/variables',
})

const settingsUserRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/user',
})

const settingsPersonaGroupsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/persona-groups',
})

const settingsPersonaUsersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/persona-users',
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  dashboardRoute,
  logsRoute,
  agentsRoute,
  settingsRoute,
  settingsSettingRoute,
  settingsSoulRoute,
  settingsTelegramRoute,
  settingsBehaviorRoute,
  settingsVariablesRoute,
  settingsUserRoute,
  settingsPersonaGroupsRoute,
  settingsPersonaUsersRoute,
])

export const router = createRouter({ routeTree })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
