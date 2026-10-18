import { AdminApp } from "./admin/AdminApp";
import { AnnouncementDetailPage, AnnouncementsPage } from "./public/AnnouncementsPage";
import { IncidentPage, MaintenancePage } from "./public/DetailPages";
import { PublicLayout } from "./public/PublicLayout";
import { StatusPage } from "./public/StatusPage";
import { match, useLocation } from "./lib/router";

/**
 * Two areas share one bundle: the public status page and the admin area.
 * The public area never calls admin endpoints.
 */
export default function App() {
  const { pathname } = useLocation();
  if (pathname === "/admin" || pathname.startsWith("/admin/")) {
    return <AdminApp />;
  }
  let params: Record<string, string> | null;
  let content = <StatusPage />;
  if ((params = match("/incidents/:id", pathname))) {
    content = <IncidentPage id={params.id} />;
  } else if ((params = match("/maintenances/:id", pathname))) {
    content = <MaintenancePage id={params.id} />;
  } else if (match("/announcements", pathname)) {
    content = <AnnouncementsPage />;
  } else if ((params = match("/announcements/:id", pathname))) {
    content = <AnnouncementDetailPage key={params.id} id={params.id} />;
  }
  return <PublicLayout>{content}</PublicLayout>;
}
