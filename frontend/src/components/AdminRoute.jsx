import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { isManager } from '../config/consoleRoles';

// 단일 콘솔 가드 — 플랫폼 관리자 또는 조직/그룹 관리자 통과(레벨별 탭·데이터는 내부에서 분기).
const AdminRoute = ({ children }) => {
  const { user } = useAuth();
  if (!user) return <Navigate to="/login" replace />;
  if (user.role !== 'admin' && !isManager(user)) return <Navigate to="/" replace />;
  return children;
};

export default AdminRoute;
