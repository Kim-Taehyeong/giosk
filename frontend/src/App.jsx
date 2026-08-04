import React, { Suspense, lazy, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './context/AuthContext';
import { useSystemConfig } from './context/SystemConfigContext';
import Login from './pages/Login';
import LocalSignup from './pages/LocalSignup';
import SignupPending from './pages/SignupPending';
import ProtectedRoute from './components/ProtectedRoute';
import PublicRoute from './components/PublicRoute';
import AdminRoute from './components/AdminRoute';
import ConsoleLayout from './layouts/ConsoleLayout';
import Spinner from './components/console/Spinner';

// 화면은 라우트 단위로 쪼개 필요할 때 내려받는다.
// 이 콘솔은 VPN·저대역폭에서 열리는 일이 많고, 일반 사용자에게 관리자 26화면까지
// 한 덩어리로 보내는 건 낭비다. 로그인 화면만 즉시 로드(첫 페인트)하고 나머지는 지연.
const Terminal = lazy(() => import('./pages/console/Terminal'));

// 사용자 콘솔 페이지
const UserDashboard = lazy(() => import('./pages/console/user/UserDashboard'));
const Guide = lazy(() => import('./pages/console/user/Guide'));
const GuideDetail = lazy(() => import('./pages/console/user/GuideDetail'));
const Sessions = lazy(() => import('./pages/console/user/Sessions'));
const UserSessionDetail = lazy(() => import('./pages/console/user/SessionDetail'));
const NewSession = lazy(() => import('./pages/console/user/NewSession'));
const Volumes = lazy(() => import('./pages/console/user/Volumes'));
const UserDatasets = lazy(() => import('./pages/console/user/Datasets'));
const Wallet = lazy(() => import('./pages/console/user/Wallet'));
const NotificationCenter = lazy(() => import('./pages/console/user/NotificationCenter'));
const Account = lazy(() => import('./pages/console/user/Account'));
const JoinGroup = lazy(() => import('./pages/console/user/JoinGroup'));

import { consolePathFor, consoleHomeFor } from './config/consoleRoles';

// 관리자 콘솔 페이지
const OpsDashboard = lazy(() => import('./pages/console/admin/OpsDashboard'));
const InfraDashboard = lazy(() => import('./pages/console/admin/InfraDashboard'));
const SessionMonitor = lazy(() => import('./pages/console/admin/SessionMonitor'));
const SessionDetailPage = lazy(() => import('./pages/console/admin/SessionDetailPage'));
const Nodes = lazy(() => import('./pages/console/admin/Nodes'));
const NodeDetailPage = lazy(() => import('./pages/console/admin/NodeDetailPage'));
const VolumesAdmin = lazy(() => import('./pages/console/admin/VolumesAdmin'));
const ManagerSettings = lazy(() => import('./pages/console/admin/ManagerSettings'));
const ImageDetail = lazy(() => import('./pages/console/admin/ImageDetail'));
const AdminUsers2 = lazy(() => import('./pages/console/admin/Users'));
const UserDetail = lazy(() => import('./pages/console/admin/UserDetail'));
const Groups = lazy(() => import('./pages/console/admin/Groups'));
const Orgs = lazy(() => import('./pages/console/admin/Orgs'));
const Resources = lazy(() => import('./pages/console/admin/Resources'));
const Policies = lazy(() => import('./pages/console/admin/Policies'));
const OrgDetail = lazy(() => import('./pages/console/admin/OrgDetail'));
const GroupDetail = lazy(() => import('./pages/console/admin/GroupDetail'));
const AdminDatasets = lazy(() => import('./pages/console/admin/Datasets'));
const DatasetDetail = lazy(() => import('./pages/console/admin/DatasetDetail'));
const Billing = lazy(() => import('./pages/console/admin/Billing'));
const Audit = lazy(() => import('./pages/console/admin/Audit'));
const Images = lazy(() => import('./pages/console/admin/Images'));
const AdminNotifications = lazy(() => import('./pages/console/admin/AdminNotifications'));
const Announcements = lazy(() => import('./pages/console/admin/Announcements'));
const Approvals = lazy(() => import('./pages/console/admin/Approvals'));
const Settings = lazy(() => import('./pages/console/admin/Settings'));

// 크레딧 전용 페이지 가드: Dynamic(선착순) 과금 모드에선 크레딧이 없으므로 콘솔 홈으로.
const CreditOnlyRoute = ({ children, to }) => {
  const { config } = useSystemConfig();
  if (config.billing.mode !== 'credit') return <Navigate to={to} replace />;
  return children;
};

// 데이터셋 기능 가드: 데이터셋 기능을 끄면 관련 페이지 접근 차단.
const DatasetRoute = ({ children, to }) => {
  const { config } = useSystemConfig();
  if (!config.features.datasets) return <Navigate to={to} replace />;
  return children;
};

// 그룹 가입 신청 가드: 전역 설정이 가입 신청을 받지 않으면 접근 차단.
const JoinGroupRoute = ({ children }) => {
  const { config } = useSystemConfig();
  if (!config.features.groupJoinRequest) return <Navigate to="/console/dashboard" replace />;
  return children;
};

// 승인 페이지 가드: 가입/충전 어느 것도 요청 불가면 접근 차단.
const ApprovalsRoute = ({ children }) => {
  const { config } = useSystemConfig();
  const active = config.features.signupRequest || (config.billing.mode === 'credit' && config.features.creditRequest);
  if (!active) return <Navigate to="/console/admin/dashboard/ops" replace />;
  return children;
};

const userRoutes = {
  dashboard: <UserDashboard />,
  guide: <Guide />,
  'guide/:id': <GuideDetail />,
  sessions: <Sessions />,
  'sessions/new': <NewSession />,
  'sessions/:id': <UserSessionDetail />,
  volumes: <Volumes />,
  datasets: <DatasetRoute to="/console/dashboard"><UserDatasets /></DatasetRoute>,
  wallet: <CreditOnlyRoute to="/console/dashboard"><Wallet /></CreditOnlyRoute>,
  'join-group': <JoinGroupRoute><JoinGroup /></JoinGroupRoute>,
  notifications: <NotificationCenter />,
  account: <Account />,
};

// 관리 콘솔의 overview 탭은 두 레벨이 공유하므로 레벨에 맞는 대시보드로 분기한다.
const adminRoutes = {
  'dashboard/ops': <OpsDashboard />,
  'dashboard/infra': <InfraDashboard />,
  sessions: <SessionMonitor />,
  'sessions/:id': <SessionDetailPage />,
  nodes: <Nodes />,
  'nodes/:name': <NodeDetailPage />,
  volumes: <VolumesAdmin />,
  'manage-settings': <ManagerSettings />,
  approvals: <ApprovalsRoute><Approvals /></ApprovalsRoute>,
  users: <AdminUsers2 />,
  'users/:id': <UserDetail />,
  groups: <Groups />,
  'groups/:id': <GroupDetail />,
  orgs: <Orgs />,
  'orgs/:id': <OrgDetail />,
  resources: <Resources />,
  policies: <Policies />,
  datasets: <DatasetRoute to="/console/admin/dashboard/ops"><AdminDatasets /></DatasetRoute>,
  'datasets/:id': <DatasetRoute to="/console/admin/dashboard/ops"><DatasetDetail /></DatasetRoute>,
  images: <Images />,
  'images/:id': <ImageDetail />,
  announcements: <Announcements />,
  billing: <CreditOnlyRoute to="/console/admin/dashboard/ops"><Billing /></CreditOnlyRoute>,
  audit: <Audit />,
  notifications: <AdminNotifications />,
  settings: <Settings />,
};

// 로그인 후 진입점: 역할/레벨에 따라 알맞은 콘솔로.
//  platform 은 /console/admin, org 와 group 은 /console/manage, 그 외는 /console 로 간다
//  (설치 모드가 Helm values 로 고정이라 첫 실행 위저드가 없다)
const ConsoleHome = () => {
  const { user } = useAuth();
  return <Navigate to={consolePathFor(user)} replace />;
};

// 관리자 콘솔 인덱스(단일 콘솔)의 레벨별 첫 화면이다. 플랫폼은 대시보드, 조직과 그룹 관리자는 자기 상세로 간다.
const AdminHome = () => {
  const { user } = useAuth();
  return <Navigate to={consoleHomeFor(user)} replace />;
};

const App = () => {
  const { config } = useSystemConfig();
  // 브랜드명에 따라 브라우저 탭 제목도 갱신.
  useEffect(() => {
    document.title = `${config.branding?.name?.trim() || 'Giosk'} Console`;
  }, [config.branding?.name]);

  return (
  <BrowserRouter>
    {/* 라우트 청크를 받아오는 동안의 대기 표시. 콘솔 내부 전환은 ConsoleLayout 안쪽에서
        셸을 유지한 채 본문만 기다리므로, 여기 폴백은 로그인→콘솔 같은 첫 진입에만 보인다. */}
    <Suspense fallback={<div style={{ display: 'grid', placeItems: 'center', minHeight: '100vh' }}><Spinner /></div>}>
    <Routes>
      <Route
        path="/login"
        element={<PublicRoute><Login /></PublicRoute>}
      />
      <Route
        path="/signup-local"
        element={<PublicRoute><LocalSignup /></PublicRoute>}
      />
      <Route
        path="/signup-pending"
        element={<PublicRoute><SignupPending /></PublicRoute>}
      />
      {/* 새 콘솔이 기본 — 기존 대시보드/관리자콘솔은 콘솔로 리다이렉트 */}
      <Route
        path="/"
        element={<ProtectedRoute><ConsoleHome /></ProtectedRoute>}
      />
      <Route path="/admin" element={<Navigate to="/console/admin" replace />} />

      {/* 사용자 콘솔 (좌측 사이드바 대시보드) */}
      <Route
        path="/console"
        element={<ProtectedRoute><ConsoleLayout variant="user" /></ProtectedRoute>}
      >
        <Route index element={<Navigate to="dashboard" replace />} />
        {Object.entries(userRoutes).map(([p, el]) => (
          <Route key={p} path={p} element={el} />
        ))}
      </Route>

      {/* 구 관리 콘솔(/console/manage)은 단일 콘솔로 통합됨 → 관리자 콘솔로 리다이렉트(북마크 호환). */}
      <Route path="/console/manage/*" element={<Navigate to="/console/admin" replace />} />

      {/* 관리자 콘솔 (단일 — platform/org/group 공용) */}
      <Route
        path="/console/admin"
        element={<AdminRoute><ConsoleLayout variant="admin" /></AdminRoute>}
      >
        <Route index element={<AdminHome />} />
        {Object.entries(adminRoutes).map(([p, el]) => (
          <Route key={p} path={p} element={el} />
        ))}
      </Route>

      {/* 웹터미널 단독 전체화면(새 창) — 콘솔 레이아웃 없이 */}
      <Route path="/terminal/:id" element={<ProtectedRoute><Terminal /></ProtectedRoute>} />

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
    </Suspense>
  </BrowserRouter>
  );
};

export default App;
