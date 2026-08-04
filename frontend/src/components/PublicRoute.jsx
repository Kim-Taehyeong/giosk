import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

// PublicRoute는 로그인된 사용자가 /login 이나 /signup 을 다시 보지 않도록 막는 가드.
const PublicRoute = ({ children }) => {
  const { user } = useAuth();
  if (user) return <Navigate to="/" replace />;
  return children;
};

export default PublicRoute;
