import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // 국기 SVG(flag-icons)는 138개를 URL 로만 참조한다. 기본 인라인 한도(4KB) 아래라 그냥 두면
    // 전부 data URI 로 진입 청크에 들어가 200KB 넘게 불어난다 — 정작 화면엔 한 번에 한두 개만 뜬다.
    // 파일로 내보내 실제 표시되는 국기만 내려받게 한다.
    assetsInlineLimit: (filePath) => (filePath.includes('flag-icons') ? false : undefined),
  },
})
