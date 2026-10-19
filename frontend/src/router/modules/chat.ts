const Layout = () => import("@/layout/index.vue");

export default {
  path: "/chat",
  name: "Chat",
  component: Layout,
  redirect: "/chat/index",
  meta: {
    icon: "ep/chat-dot-round",
    title: "AI 智能助手",
    rank: 1
  },
  children: [
    {
      path: "/chat/index",
      name: "AiChat",
      component: () => import("@/views/chat/index.vue"),
      meta: {
        title: "智能会话",
        icon: "ep/cpu"
      }
    }
  ]
} satisfies RouteConfigsTable;
