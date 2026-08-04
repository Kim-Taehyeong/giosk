import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { isManager } from '../config/consoleRoles';

// 단일 콘솔 가드다. 플랫폼 관리자나 조직·그룹 관리자가 통과하며 레벨별 탭과 데이터는 내부에서 갈린다.
const AdminRoute = ({ children }) => {
  const { user } = useAuth();
  if (!user) return <Navigate to="/login" replace />;
  if (user.role !== 'admin' && !isManager(user)) return <Navigate to="/" replace />;
  return children;
};

export default AdminRoute;
