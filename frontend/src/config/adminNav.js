import {
  LayoutDashboard, Activity, MonitorPlay, Server,
  Users, FolderKanban, Building2, Boxes, Receipt, ScrollText, BellRing, Container, BadgeCheck, SlidersHorizontal, Database, Megaphone, ShieldCheck, HardDrive,
} from 'lucide-react';

// 관리자 콘솔 사이드바 네비게이션이다(단일 콘솔이라 platform, org, group 이 함께 쓴다).
// label 은 i18n 키(consoleAdmin.nav.*). path 는 /console/admin 하위 상대경로.
// levels: 이 탭을 볼 수 있는 레벨. 생략 시 platform 전용(인프라·시스템 기본 보호).
//   platform=최고관리자, org=조직관리자, group=팀장.
const ALL = ['platform', 'org', 'group'];
export const adminNavGroups = [
  {
    title: 'groupOps',
    items: [
      { key: 'opsDashboard', icon: Activity, path: 'dashboard/ops', levels: ALL }, // 운영(사용과 거버넌스). 자기 범위만 본다
      { key: 'infraDashboard', icon: LayoutDashboard, path: 'dashboard/infra' }, // 인프라(클러스터 하드웨어) = platform 전용
      { key: 'sessions', icon: MonitorPlay, path: 'sessions', levels: ALL },
      { key: 'nodes', icon: Server, path: 'nodes' }, // 물리 노드 인프라 = platform 전용
    ],
  },
  {
    title: 'groupGov',
    items: [
      { key: 'approvals', icon: BadgeCheck, path: 'approvals', levels: ALL },
      { key: 'users', icon: Users, path: 'users', levels: ALL },
      { key: 'groups', icon: FolderKanban, path: 'groups', levels: ALL }, // 팀장은 자기 팀만 본다(백엔드 group 스코프가 자기 팀만 반환한다). 조직관리자가 자기 조직만 보는 것과 같은 관점이다
      { key: 'orgs', icon: Building2, path: 'orgs', levels: ['platform', 'org'] }, // 조직 관리는 최고/조직관리자만. 팀장은 조직 안 봄.
      { key: 'policies', icon: ShieldCheck, path: 'policies', levels: ALL }, // 자기 범위 정책(읽기), 편집은 platform

      { key: 'resources', icon: Boxes, path: 'resources' },
      { key: 'volumes', icon: HardDrive, path: 'volumes', levels: ALL }, // 매니저는 자기 범위 볼륨 열람, 임포트는 platform
      { key: 'datasets', icon: Database, path: 'datasets' },
    ],
  },
  {
    title: 'groupSystem',
    items: [
      { key: 'images', icon: Container, path: 'images' },
      { key: 'announcements', icon: Megaphone, path: 'announcements', levels: ALL }, // 자기 범위 타겟 공지
      { key: 'billing', icon: Receipt, path: 'billing', levels: ALL }, // 자기 범위 소비 showback
      { key: 'audit', icon: ScrollText, path: 'audit', levels: ALL }, // 자기 범위 감사 로그
      { key: 'notifications', icon: BellRing, path: 'notifications' },
      { key: 'settings', icon: SlidersHorizontal, path: 'settings' },
      { key: 'manageSettings', icon: SlidersHorizontal, path: 'manage-settings', levels: ['org', 'group'] }, // 매니저 설정(가입 수락 등)
    ],
  },
];
