import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

// PublicRoute — 로그인된 사용자가 /login·/signup을 다시 보지 않도록 가드.
const PublicRoute = ({ children }) => {
  const { user } = useAuth();
  if (user) return <Navigate to="/" replace />;
  return children;
};

export default PublicRoute;
