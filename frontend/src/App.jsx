import React, { useEffect } from 'react';
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
import Terminal from './pages/console/Terminal';

// 사용자 콘솔 페이지
import UserDashboard from './pages/console/user/UserDashboard';
import Guide from './pages/console/user/Guide';
import GuideDetail from './pages/console/user/GuideDetail';
import Sessions from './pages/console/user/Sessions';
import UserSessionDetail from './pages/console/user/SessionDetail';
import NewSession from './pages/console/user/NewSession';
import Volumes from './pages/console/user/Volumes';
import UserDatasets from './pages/console/user/Datasets';
import Wallet from './pages/console/user/Wallet';
import NotificationCenter from './pages/console/user/NotificationCenter';
import Account from './pages/console/user/Account';
import JoinGroup from './pages/console/user/JoinGroup';

import { consolePathFor, consoleHomeFor } from './config/consoleRoles';

// 관리자 콘솔 페이지
import OpsDashboard from './pages/console/admin/OpsDashboard';
import InfraDashboard from './pages/console/admin/InfraDashboard';
import SessionMonitor from './pages/console/admin/SessionMonitor';
import SessionDetailPage from './pages/console/admin/SessionDetailPage';
import Nodes from './pages/console/admin/Nodes';
import NodeDetailPage from './pages/console/admin/NodeDetailPage';
import VolumesAdmin from './pages/console/admin/VolumesAdmin';
import ManagerSettings from './pages/console/admin/ManagerSettings';
import ImageDetail from './pages/console/admin/ImageDetail';
import AdminUsers2 from './pages/console/admin/Users';
import UserDetail from './pages/console/admin/UserDetail';
import Groups from './pages/console/admin/Groups';
import Orgs from './pages/console/admin/Orgs';
import Resources from './pages/console/admin/Resources';
import Policies from './pages/console/admin/Policies';
import OrgDetail from './pages/console/admin/OrgDetail';
import GroupDetail from './pages/console/admin/GroupDetail';
import AdminDatasets from './pages/console/admin/Datasets';
import DatasetDetail from './pages/console/admin/DatasetDetail';
import Billing from './pages/console/admin/Billing';
import Audit from './pages/console/admin/Audit';
import Images from './pages/console/admin/Images';
import AdminNotifications from './pages/console/admin/AdminNotifications';
import Announcements from './pages/console/admin/Announcements';
import Approvals from './pages/console/admin/Approvals';
import Settings from './pages/console/admin/Settings';

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

// 관리 콘솔의 'overview' 탭은 두 레벨이 공유 — 레벨에 맞는 대시보드로 분기.
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
//  platform→/console/admin, org·group→/console/manage, 그 외→/console
//  (설치 모드는 Helm values 로 고정 → 첫 실행 위저드 없음)
const ConsoleHome = () => {
  const { user } = useAuth();
  return <Navigate to={consolePathFor(user)} replace />;
};

// 관리자 콘솔 인덱스(단일 콘솔) — 레벨별 첫 화면. 플랫폼=대시보드, 조직/그룹 관리자=자기 상세.
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
  </BrowserRouter>
  );
};

export default App;
