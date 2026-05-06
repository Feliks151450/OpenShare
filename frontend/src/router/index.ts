import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

import AdminLayout from "@/layouts/AdminLayout.vue";
import AdminAdminsView from "@/views/admin/AdminAdminsView.vue";
import PublicLayout from "@/layouts/PublicLayout.vue";
import AdminAuditView from "@/views/admin/AdminAuditView.vue";
import AdminAccountSettingsView from "@/views/admin/AdminAccountSettingsView.vue";
import AdminDashboard from "@/views/admin/AdminDashboard.vue";
import AdminOperationLogsView from "@/views/admin/AdminOperationLogsView.vue";
import AdminFileTagsView from "@/views/admin/AdminFileTagsView.vue";
import AdminAnnouncementsView from "@/views/admin/AdminAnnouncementsView.vue";
import PublicFileDetailView from "@/views/public/PublicFileDetailView.vue";
import HomeView from "@/views/public/Home.vue";
import UploadView from "@/views/public/UploadView.vue";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    component: PublicLayout,
    children: [
      {
        path: "",
        name: "public-home",
        component: HomeView,
      },
      {
        path: "upload",
        name: "public-upload",
        component: UploadView,
      },
      {
        path: "files/:fileID",
        name: "public-file-detail",
        component: PublicFileDetailView,
      },
    ],
  },
  {
    path: "/admin",
    component: AdminLayout,
    children: [
      {
        path: "",
        name: "admin-dashboard",
        component: AdminDashboard,
      },
      {
        path: "admins",
        redirect: "/admin/permissions",
      },
      {
        path: "permissions",
        name: "admin-permissions",
        component: AdminAdminsView,
      },
      {
        path: "audit",
        name: "admin-audit",
        component: AdminAuditView,
      },
      {
        path: "logs",
        name: "admin-logs",
        component: AdminOperationLogsView,
      },
      {
        path: "file-tags",
        name: "admin-file-tags",
        component: AdminFileTagsView,
      },
      {
        path: "announcements",
        name: "admin-announcements",
        component: AdminAnnouncementsView,
      },
      {
        path: "account",
        name: "admin-account",
        component: AdminAccountSettingsView,
      },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 };
  },
});

export default router;
