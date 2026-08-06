import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{js,jsx}'],
    extends: [
      js.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        ecmaVersion: 'latest',
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
      },
    },
    rules: {
      // 대문자로 시작하는 이름은 JSX 안에서만 쓰이는 컴포넌트 참조다. eslint-plugin-react 를
      // 넣지 않아 jsx-uses-vars 가 없고, 그래서 <Icon /> 처럼 JSX 에서만 쓰면 미사용으로 잡힌다.
      // 변수(varsIgnorePattern)와 구조분해 인자(argsIgnorePattern) 모두 같은 규칙을 적용한다.
      'no-unused-vars': ['error', { varsIgnorePattern: '^[A-Z_]', argsIgnorePattern: '^[A-Z_]' }],
      // Fast Refresh 편의 규칙이다. 이 저장소는 Provider 와 그 useX 훅을 한 파일에 둔다.
      // 읽기 쉬운 배치라 유지하고, HMR 경고는 경고로만 남긴다.
      'react-refresh/only-export-components': 'warn',

      // ── React Compiler 계열 규칙 (react-hooks v6 신규) ─────────────────
      // 이 코드베이스보다 나중에 나온 규칙들이라 기존 패턴이 전부 걸린다. 지금 18곳이다.
      // 가리려는 게 아니라 순서를 정한 것이다. 전부 실제로 고쳐야 하고, 고칠 때마다
      // 렌더 동작이 바뀔 수 있어 화면별로 확인이 필요하다. 그래서 경고로 두고 줄여 나간다.
      // (진행 상황은 docs/ko/roadmap.md 에 적어 둔다. 다 없애면 다시 error 로 올린다.)
      //
      // rules-of-hooks 는 여기 넣지 않는다. 훅 순서가 깨지는 것은 예전부터 버그였고
      // 지금도 error 로 남긴다.
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/refs': 'warn',
      'react-hooks/purity': 'warn',
      'react-hooks/immutability': 'warn',
      'react-hooks/static-components': 'warn',
      'react-hooks/globals': 'warn',
    },
  },
])
