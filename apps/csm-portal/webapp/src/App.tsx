// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { type JSX, lazy } from "react";
import { Navigate, Outlet, Route, Routes, useLocation, useParams } from "react-router";
import AuthGuard from "@layouts/AuthGuard";
import { SectionIndexRedirect } from "@components/section-tabs/SectionTabs";
import { navNodeForPath } from "@config/csmNavItems";
import {
  featureStateForPath,
  firstEnabledDestination,
} from "@config/featureFlags";
import {
  POST_LOGIN_REDIRECT_KEY,
  PostLoginRedirectConsumer,
} from "@layouts/postLoginRedirect";
import ErrorLayout from "@layouts/ErrorLayout";
import CsmComingSoonPage from "@features/csm-coming-soon/pages/CsmComingSoonPage";
import Error401Page from "@components/error/Error401Page";
import Error403Page from "@components/error/Error403Page";
import Error404Page from "@components/error/Error404Page";
import { ErrorBannerProvider } from "@context/error-banner/ErrorBannerContext";
import { SuccessBannerProvider } from "@context/success-banner/SuccessBannerContext";
import { LoaderProvider } from "@context/linear-loader/LoaderContext";
import { ErrorPageProvider } from "@context/error-page/ErrorPageContext";

/*
 * Authenticated feature pages are lazily loaded so each lands in its own chunk
 * and is fetched only when its route is visited, instead of being bundled into
 * the initial entry chunk. They all render inside AppLayout's Outlet, which
 * owns the Suspense boundary that covers the load. Error pages and the shared
 * CsmComingSoonPage stay eager: they are tiny and act as immediate fallbacks.
 */
const CsmDashboardPage = lazy(
  () => import("@features/csm-dashboard/pages/CsmDashboardPage"),
);
const DashboardWidgetPreviewPage = lazy(
  () => import("@features/csm-dashboard/pages/DashboardWidgetPreviewPage"),
);
const CsmCasesPage = lazy(
  () => import("@features/csm-cases/pages/CsmCasesPage"),
);
const CsmCaseCreatePage = lazy(
  () => import("@features/csm-cases/pages/CsmCaseCreatePage"),
);
const CsmCaseDetailPage = lazy(
  () => import("@features/csm-cases/pages/CsmCaseDetailPage"),
);
const OperationsPage = lazy(
  () => import("@features/csm-operations/pages/OperationsPage"),
);
const CreateServiceRequestPage = lazy(
  () => import("@features/csm-operations/pages/CreateServiceRequestPage"),
);
const CsmChangeRequestDetailPage = lazy(
  () => import("@features/csm-operations/pages/CsmChangeRequestDetailPage"),
);
const CreateChangeRequestPage = lazy(
  () => import("@features/csm-operations/pages/CreateChangeRequestPage"),
);
const CsmIncidentDetailPage = lazy(
  () => import("@features/csm-operations/pages/CsmIncidentDetailPage"),
);
const CreateIncidentPage = lazy(
  () => import("@features/csm-operations/pages/CreateIncidentPage"),
);
const ProblemDetailPage = lazy(
  () => import("@features/csm-operations/pages/ProblemDetailPage"),
);
const CreateProblemPage = lazy(
  () => import("@features/csm-operations/pages/CreateProblemPage"),
);
const CsmAdminLayout = lazy(
  () => import("@features/csm-admin/pages/CsmAdminLayout"),
);
const CsmUsersPage = lazy(
  () => import("@features/csm-users/pages/CsmUsersPage"),
);
const UserProfilePage = lazy(
  () => import("@features/csm-users/pages/UserProfilePage"),
);
const CsmRolesPage = lazy(
  () => import("@features/csm-admin/pages/CsmRolesPage"),
);
const RoleMembersPage = lazy(
  () => import("@features/csm-admin/pages/RoleMembersPage"),
);
const CsmGroupsPage = lazy(
  () => import("@features/csm-admin/pages/CsmGroupsPage"),
);
const GroupMembersPage = lazy(
  () => import("@features/csm-admin/pages/GroupMembersPage"),
);
const CsmTeamsPage = lazy(
  () => import("@features/csm-admin/pages/CsmTeamsPage"),
);
const TeamMembersPage = lazy(
  () => import("@features/csm-admin/pages/TeamMembersPage"),
);
const CsmCustomersLayout = lazy(
  () => import("@features/csm-customers/pages/CsmCustomersLayout"),
);
const CsmAccountsPage = lazy(
  () => import("@features/csm-accounts/pages/CsmAccountsPage"),
);
const CsmAccountDetailPage = lazy(
  () => import("@features/csm-accounts/pages/CsmAccountDetailPage"),
);
const CsmProjectsPage = lazy(
  () => import("@features/csm-projects/pages/CsmProjectsPage"),
);
const CsmProjectDetailPage = lazy(
  () => import("@features/csm-projects/pages/CsmProjectDetailPage"),
);
const CsmUpdatesPage = lazy(
  () => import("@features/updates/pages/CsmUpdatesPage"),
);
const CsmSecurityCenterPage = lazy(
  () => import("@features/csm-security-center/pages/CsmSecurityCenterPage"),
);
const CreateSecurityReportPage = lazy(
  () => import("@features/csm-security-center/pages/CreateSecurityReportPage"),
);
const ProductVulnerabilityDetailPage = lazy(
  () => import("@features/csm-security-center/pages/ProductVulnerabilityDetailPage"),
);
const CsmEngagementsPage = lazy(
  () => import("@features/csm-engagements/pages/CsmEngagementsPage"),
);
const CsmEngagementCreatePage = lazy(
  () => import("@features/csm-engagements/pages/CsmEngagementCreatePage"),
);
const CsmTimeCardsPage = lazy(
  () => import("@features/csm-timecards/pages/CsmTimeCardsPage"),
);
const CsmAnnouncementsPage = lazy(
  () => import("@features/csm-announcements/pages/CsmAnnouncementsPage"),
);

/**
 * Landing for `/`. Defers to AuthGuard's post-login deep-link restore when a
 * redirect is pending (rendering nothing so it doesn't race the restore);
 * otherwise sends the user to the default `/dashboard` landing. A pure read of
 * sessionStorage — AuthGuard owns clearing the key.
 */
function RootLanding(): JSX.Element | null {
  const pending = sessionStorage.getItem(POST_LOGIN_REDIRECT_KEY);
  return pending ? null : <Navigate to="/dashboard" replace />;
}

/**
 * Layout guard honouring the `CSM_PORTAL_FEATURE_OVERRIDES` runtime config, so
 * a direct, pinned or shared link can't reach a page the deployment restricts.
 *
 * A `wip` page renders the shared "coming soon" message in place — the URL
 * survives and the wording matches the nav's "work in progress" tooltip. The
 * exception is a page that already renders a more specific unavailable message
 * of its own (`rendersOwnWipPage`), which is let through rather than downgraded
 * to the generic one.
 *
 * A `hidden` page has no nav entry at all, so there is nothing to stay
 * consistent with and the link is bounced to the first destination this
 * deployment does offer. That target is never assumed to exist: a config that
 * hides everything, or that hides the target itself, falls through to `/404`,
 * which sits outside this guard and so cannot bounce back here.
 *
 * The path resolves to the most specific nav node, so restricting a section
 * does not restrict a finished tab inside it (and vice versa).
 */
function FeatureRouteGuard(): JSX.Element {
  const { pathname } = useLocation();
  const node = navNodeForPath(pathname);
  const state = featureStateForPath(pathname);

  if (state === "hidden") {
    const fallback = firstEnabledDestination();
    const samePath = fallback !== undefined && fallback.split(/[?#]/)[0] === pathname;
    return <Navigate to={!fallback || samePath ? "/404" : fallback} replace />;
  }

  if (state === "wip" && !node?.rendersOwnWipPage) {
    const label = node?.label ?? "This section";
    return (
      <CsmComingSoonPage
        title={label}
        description={`${label} is still a work in progress and isn't available yet.`}
      />
    );
  }

  return <Outlet />;
}

/**
 * Redirects a legacy detail path (`/accounts/:id`, `/projects/:id`) to its new
 * home under `/customers`, preserving the id. Exists only so old pinned/deep
 * links survive the Accounts+Projects → Customers menu merge.
 */
function LegacyDetailRedirect({ to }: { to: string }): JSX.Element {
  const { id } = useParams();
  // Preserve any query/hash so legacy deep links (e.g. /accounts/:id?tab=…#…)
  // keep their context through the compatibility redirect.
  const { search, hash } = useLocation();
  const target = id ? `${to}/${id}` : to;
  return <Navigate to={`${target}${search}${hash}`} replace />;
}

export default function App(): JSX.Element {
  return (
    <LoaderProvider>
      <ErrorBannerProvider>
        <SuccessBannerProvider>
          <ErrorPageProvider>
            <PostLoginRedirectConsumer />
            <Routes>
              <Route
                path="/401"
                element={
                  <ErrorLayout>
                    <Error401Page />
                  </ErrorLayout>
                }
              />
              <Route
                path="/403"
                element={
                  <ErrorLayout>
                    <Error403Page />
                  </ErrorLayout>
                }
              />
              <Route
                path="/404"
                element={
                  <ErrorLayout>
                    <Error404Page />
                  </ErrorLayout>
                }
              />

              <Route element={<AuthGuard />}>
                <Route element={<FeatureRouteGuard />}>
                  <Route path="/" element={<RootLanding />} />

                  {/* Customers — Accounts + Projects under one tabbed section.
                      BFF-backed pages (entity-service search + by-id endpoints).
                      Detail pages render full-width (outside the tab layout). */}
                  <Route path="customers" element={<CsmCustomersLayout />}>
                    <Route
                      index
                      element={<SectionIndexRedirect sectionId="customers" />}
                    />
                    <Route path="accounts" element={<CsmAccountsPage />} />
                    <Route path="projects" element={<CsmProjectsPage />} />
                  </Route>
                  <Route
                    path="customers/accounts/:id"
                    element={<CsmAccountDetailPage />}
                  />
                  <Route
                    path="customers/projects/:id"
                    element={<CsmProjectDetailPage />}
                  />

                  {/* Legacy paths kept alive so pinned/deep links don't 404. */}
                  <Route
                    path="accounts"
                    element={<Navigate to="/customers/accounts" replace />}
                  />
                  <Route
                    path="accounts/:id"
                    element={<LegacyDetailRedirect to="/customers/accounts" />}
                  />
                  <Route
                    path="projects"
                    element={<Navigate to="/customers/projects" replace />}
                  />
                  <Route
                    path="projects/:id"
                    element={<LegacyDetailRedirect to="/customers/projects" />}
                  />

                  {/* Administration — Users/Roles/Groups/Teams are real,
                      Permissions is still WIP. */}
                  <Route path="admin" element={<CsmAdminLayout />}>
                    <Route
                      index
                      element={<SectionIndexRedirect sectionId="admin" />}
                    />
                    <Route path="users" element={<CsmUsersPage />} />
                    <Route path="roles" element={<CsmRolesPage />} />
                    <Route path="groups" element={<CsmGroupsPage />} />
                    <Route path="teams" element={<CsmTeamsPage />} />
                    <Route
                      path="permissions"
                      element={
                        <CsmComingSoonPage
                          title="Permissions"
                          description="Fine-grained permission catalog and assignment view."
                          blockedOn="csm-portal/backend permissions endpoints"
                        />
                      }
                    />
                  </Route>

                  {/* Role/group/team member lists, one level below the
                      directory pages above. Not admin-permission-gated:
                      standing project rule is to show the action and let the
                      backend reject it, never gate in the frontend. */}
                  <Route path="admin/roles/:id" element={<RoleMembersPage />} />
                  <Route path="admin/groups/:id" element={<GroupMembersPage />} />
                  <Route path="admin/teams/:id" element={<TeamMembersPage />} />

                  {/* Person profile — reachable by clicking any user reference
                      (case creator, assignee, watchers, comment authors,
                      attachment uploaders). Keyed on the user id (not the
                      email): most `UserReference` sites don't resolve an id
                      themselves, so `UserRefLink` only ever links once one is
                      known or resolved (see useResolvedUserId). Not
                      admin-gated: any signed-in CS engineer can look up any
                      user. */}
                  <Route
                    path="people/:id"
                    element={<UserProfilePage />}
                  />

                  <Route path="dashboard" element={<CsmDashboardPage />} />
                  {/* Dashboard widget "View more" preview — :previewSlug is
                      one of WIDGET_RESOURCE_CONFIG's own previewSlug values
                      (e.g. "cases"), resolved back to a resourceType by
                      resourceTypeForPreviewSlug. Distinct from the resource's
                      own real list route (e.g. /cases). */}
                  <Route
                    path="dashboard/:previewSlug"
                    element={<DashboardWidgetPreviewPage />}
                  />
                  <Route path="cases" element={<CsmCasesPage />} />
                  <Route path="cases/new" element={<CsmCaseCreatePage />} />
                  <Route path="cases/:caseId" element={<CsmCaseDetailPage />} />

                  <Route path="operations" element={<OperationsPage />} />
                  <Route
                    path="operations/service-requests/new"
                    element={<CreateServiceRequestPage />}
                  />
                  <Route
                    path="operations/service-requests/:caseId"
                    element={<CsmCaseDetailPage />}
                  />
                  <Route
                    path="operations/change-requests/new"
                    element={<CreateChangeRequestPage />}
                  />
                  <Route
                    path="operations/change-requests/:id"
                    element={<CsmChangeRequestDetailPage />}
                  />
                  <Route
                    path="operations/incidents/new"
                    element={<CreateIncidentPage />}
                  />
                  <Route
                    path="operations/incidents/:id"
                    element={<CsmIncidentDetailPage />}
                  />
                  <Route
                    path="operations/problems/new"
                    element={<CreateProblemPage />}
                  />
                  <Route
                    path="operations/problems/:id"
                    element={<ProblemDetailPage />}
                  />

                  <Route path="engagements" element={<CsmEngagementsPage />} />
                  <Route path="engagements/new" element={<CsmEngagementCreatePage />} />
                  <Route path="engagements/:caseId" element={<CsmCaseDetailPage />} />
                  <Route path="updates" element={<CsmUpdatesPage />} />
                  <Route path="security-center" element={<CsmSecurityCenterPage />} />
                  <Route
                    path="security-center/reports/new"
                    element={<CreateSecurityReportPage />}
                  />
                  <Route
                    path="security-center/vulnerabilities/:id"
                    element={<ProductVulnerabilityDetailPage />}
                  />
                  <Route
                    path="security-center/security-reports/:caseId"
                    element={<CsmCaseDetailPage />}
                  />
                  <Route path="time-cards" element={<CsmTimeCardsPage />} />
                  <Route path="announcements" element={<CsmAnnouncementsPage />} />
                  <Route
                    path="announcements/:caseId"
                    element={<CsmCaseDetailPage />}
                  />
                </Route>
              </Route>

              <Route
                path="*"
                element={
                  <ErrorLayout>
                    <Error404Page />
                  </ErrorLayout>
                }
              />
            </Routes>
          </ErrorPageProvider>
        </SuccessBannerProvider>
      </ErrorBannerProvider>
    </LoaderProvider>
  );
}
