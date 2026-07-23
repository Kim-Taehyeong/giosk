import { apiGet } from '../client';

// 사용자/관리자 대시보드 — 실 백엔드(DB 집계 + Prometheus/DCGM 지표).
// Prometheus 미연동 시 GPU 지표는 0/빈 추이로 안전하게 내려온다.
export const getUserDashboard = () => apiGet('/dashboard');
// 인프라 대시보드(클러스터 하드웨어) — platform 전용. Topbar 배지와 공용.
export const getAdminDashboard = () => apiGet('/admin/dashboard');
export const getInfraDashboard = () => apiGet('/admin/dashboard');
// 운영 대시보드(사용·거버넌스) — 전 관리 레벨, 백엔드가 호출자 스코프로 필터.
export const getOpsDashboard = () => apiGet('/console/dashboard/ops');
